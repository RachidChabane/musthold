// Package parcel is test support: the honest host of the parcel project, which reads
// the prompt an executor sees and answers the way a careful model would.
package parcel

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/RachidChabane/spec-proof/format/hostproto"
)

// Oracle plays an honest host: it reads the prompt an executor sees and answers
// the way a careful model would for the parcel project.
type Oracle struct{}

var (
	unitHeader       = regexp.MustCompile(`^(\S+) :: (\S+)\n`)
	changedPattern   = regexp.MustCompile(`(?s)--- (\S+) :: (\S+)\nbefore:\n(.*?)\nafter:\n(.*?)(?:\n--- |\n\nObserved|\n\nAnswer|\n\nFor each|$)`)
	observedPattern  = regexp.MustCompile(`- (\w+)\(([^)]*)\): original (\S+), mutant (.+)`)
	requirementBlock = regexp.MustCompile(`(?s)Requirement (\d+):\n(.*?)\n\n`)
)

// Answer implements hostprototest.AnswerSource.
func (Oracle) Answer(_ string, view hostproto.ExecutorView) (string, error) {
	switch {
	case len(view.Closed) > 0:
		return ScreenAnswer(view.User), nil
	case strings.Contains(string(view.ResponseSchema), `"witnesses"`):
		return WitnessAnswer(view.User), nil
	case strings.Contains(string(view.ResponseSchema), `"verdicts"`):
		return ScopeAnswer(view.User), nil
	case strings.Contains(string(view.ResponseSchema), `"mutants"`):
		return generationAnswer(view.User), nil
	case strings.Contains(string(view.ResponseSchema), `"equivalent"`):
		return `{"equivalent":"EQUIVALENT","confidence":0.95}`, nil
	default:
		return judgeAnswer(view.User), nil
	}
}

// Source file suffixes of the parcel projects.
const (
	pythonSuffix     = ".py"
	goSuffix         = ".go"
	typescriptSuffix = ".ts"
	goPackage        = "parcel."
)

// mutation is one scripted edit: the text to replace in a unit and what replaces
// it. A suffix limits the edit to the files of one language; none applies to every language.
type mutation struct{ from, to, suffix string }

func generationAnswer(user string) string {
	statement := user[:strings.Index(user, "Code under test")]
	var scripted map[string][]mutation
	switch {
	case strings.Contains(statement, "shipping fee"):
		scripted = map[string][]mutation{
			"S2": {{">=", ">", ""}},
			"S1": {{"return 0", "return SHIPPING_FEE", pythonSuffix}, {"return 0", "return FlatFee", goSuffix}, {"return 0", "return SHIPPING_FEE", typescriptSuffix}},
		}
	case strings.Contains(statement, "unit price times"):
		scripted = map[string][]mutation{"S1": {
			{"unit_price * quantity", "unit_price + quantity", pythonSuffix},
			{"unitPrice * quantity", "unitPrice + quantity", goSuffix},
			{"unitPrice * quantity", "unitPrice + quantity", typescriptSuffix},
		}}
	default:
		scripted = map[string][]mutation{"S1": {
			{"MAX_PERCENT - clamp_percent(percent)", "MAX_PERCENT + clamp_percent(percent)", pythonSuffix},
			{"MaxPercent - ClampPercent(percent)", "MaxPercent + ClampPercent(percent)", goSuffix},
			{"MAX_PERCENT - clampPercent(percent)", "MAX_PERCENT + clampPercent(percent)", typescriptSuffix},
		}}
	}
	units := sliceUnits(user)
	var mutants []map[string]any
	for _, slot := range []string{"S1", "S2", "S3"} {
		for _, change := range scripted[slot] {
			for qualname, unit := range units {
				if !strings.Contains(unit.source, change.from) || !strings.HasSuffix(unit.file, change.suffix) {
					continue
				}
				mutants = append(mutants, map[string]any{
					"slot": slot, "mechanism": "replace " + change.from + " by " + change.to,
					"edits": []map[string]string{{"file": unit.file, "qualname": qualname, "new_source": strings.Replace(unit.source, change.from, change.to, 1)}},
				})
			}
		}
	}
	encoded, _ := json.Marshal(map[string]any{"mutants": mutants})
	return string(encoded)
}

type sliceUnit struct{ file, source string }

// sliceUnits reads the units of the code slice a generation prompt shows.
func sliceUnits(user string) map[string]sliceUnit {
	start := strings.Index(user, "Code under test:\n") + len("Code under test:\n")
	end := strings.Index(user, "\n\nWrite up to")
	units := map[string]sliceUnit{}
	for _, block := range strings.Split(user[start:end], "--- ")[1:] {
		header := unitHeader.FindStringSubmatch(block)
		units[header[2]] = sliceUnit{file: header[1], source: strings.TrimRight(block[len(header[0]):], "\n")}
	}
	return units
}

// violates reports whether the change touches what the statement is about.
func violates(statement string, changedUnits []string) bool {
	subjects := map[string][]string{
		"shipping fee":  {"shipping_fee", "FREE_SHIPPING_FROM", "SHIPPING_FEE", "ShippingFee", "FreeShippingFrom", "FlatFee", "shippingFee"},
		"unit price":    {"line_total", "LineTotal", "lineTotal"},
		"discount perc": {"discounted", "clamp_percent", "MAX_PERCENT", "Discounted", "ClampPercent", "MaxPercent", "clampPercent"},
	}
	for phrase, names := range subjects {
		if !strings.Contains(statement, phrase) {
			continue
		}
		for _, unit := range changedUnits {
			unit = strings.TrimPrefix(unit, goPackage)
			for _, name := range names {
				if unit == name {
					return true
				}
			}
		}
	}
	return false
}

func changedUnits(user string) []string {
	var units []string
	for _, match := range changedPattern.FindAllStringSubmatch(user, -1) {
		units = append(units, match[2])
	}
	return units
}

// ScreenAnswer answers a closed screening request the way an honest host does.
func ScreenAnswer(user string) string {
	units := changedUnits(user[strings.Index(user, "Changed units"):])
	var answers []map[string]any
	for _, match := range requirementBlock.FindAllStringSubmatch(user, -1) {
		choice := "CONSISTENT"
		if violates(match[2], units) {
			choice = "VIOLATES"
		}
		answers = append(answers, map[string]any{"id": "q" + match[1], "choice": choice})
	}
	encoded, _ := json.Marshal(map[string]any{"answers": answers})
	return string(encoded)
}

func judgeAnswer(user string) string {
	end := strings.Index(user, "Operations you may use")
	if end < 0 {
		end = strings.Index(user, "Changed units")
	}
	statement := user[:max(end, 0)]
	if !violates(statement, changedUnits(user)) {
		return `{"validity":"NOT_VIOLATING","confidence":0.9}`
	}
	witness := firstWitness(user)
	if witness == nil {
		return `{"validity":"VALID","confidence":0.9}`
	}
	encoded, _ := json.Marshal(map[string]any{"validity": "VALID", "confidence": 0.9, "witness": witness})
	return string(encoded)
}

// firstWitness turns the first observed difference into a witness whose expected
// result is what the original returned.
func firstWitness(user string) map[string]any {
	match := observedPattern.FindStringSubmatch(user)
	if match == nil {
		return nil
	}
	args := map[string]any{}
	for _, pair := range strings.Split(match[2], ", ") {
		name, value, _ := strings.Cut(pair, "=")
		args[name] = literal(value)
	}
	expected := literal(match[3])
	expected["kind"] = "returns"
	return map[string]any{"operation": match[1], "args": args, "expected": expected}
}

func literal(text string) map[string]any {
	switch {
	case strings.Contains(text, "."):
		return map[string]any{"type": "decimal", "value": text}
	case text == "true" || text == "false":
		return map[string]any{"type": "boolean", "value": text}
	case strings.HasPrefix(text, `"`):
		return map[string]any{"type": "string", "value": strings.Trim(text, `"`)}
	}
	return map[string]any{"type": "integer", "value": text}
}
