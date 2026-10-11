package cli_test

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/RachidChabane/spec-proof/format/formattest/parcel"
	"github.com/RachidChabane/spec-proof/format/hostproto"
)

// javaOracle plays an honest host for the Java parcel project: it reads the
// prompt an executor sees and answers the way a careful model would.
type javaOracle struct{}

const (
	javaSuffix        = ".java"
	javaOwnerPrefix   = "Rates."
	generationCode    = "Code under test:\n"
	generationClosing = "\n\nWrite up to"
)

var (
	javaUnitHeader       = regexp.MustCompile(`^(\S+) :: (\S+)\n`)
	javaChangedPattern   = regexp.MustCompile(`(?s)--- (\S+) :: (\S+)\nbefore:\n(.*?)\nafter:\n(.*?)(?:\n--- |\n\nObserved|\n\nAnswer|\n\nFor each|$)`)
	javaObservedPattern  = regexp.MustCompile(`- (\w+)\(([^)]*)\): original (\S+), mutant (.+)`)
	javaRequirementBlock = regexp.MustCompile(`(?s)Requirement (\d+):\n(.*?)\n\n`)
)

// javaSubjects names the units each statement is about, by the phrase that
// identifies the statement.
var javaSubjects = map[string][]string{
	"shipping fee":  {"shippingFee", "FREE_SHIPPING_FROM", "FLAT_FEE"},
	"unit price":    {"lineTotal"},
	"discount perc": {"discounted", "clampPercent", "MAX_PERCENT"},
}

// javaMutation is one scripted edit: the text to replace in a unit and what
// replaces it.
type javaMutation struct{ from, to string }

// Answer implements hostprototest.AnswerSource.
func (javaOracle) Answer(_ string, view hostproto.ExecutorView) (string, error) {
	switch {
	case len(view.Closed) > 0:
		return javaScreenAnswer(view.User), nil
	case strings.Contains(string(view.ResponseSchema), `"witnesses"`):
		return parcel.WitnessAnswer(view.User), nil
	case strings.Contains(string(view.ResponseSchema), `"verdicts"`):
		return parcel.ScopeAnswer(view.User), nil
	case strings.Contains(string(view.ResponseSchema), `"mutants"`):
		return javaGenerationAnswer(view.User), nil
	case strings.Contains(string(view.ResponseSchema), `"equivalent"`):
		return `{"equivalent":"EQUIVALENT","confidence":0.95}`, nil
	default:
		return javaJudgeAnswer(view.User), nil
	}
}

func javaGenerationAnswer(user string) string {
	statement := user[:strings.Index(user, "Code under test")]
	var scripted map[string][]javaMutation
	switch {
	case strings.Contains(statement, "shipping fee"):
		scripted = map[string][]javaMutation{
			"S2": {{">= 0", "> 0"}},
			"S1": {{"return BigDecimal.ZERO;", "return FLAT_FEE;"}},
		}
	case strings.Contains(statement, "unit price times"):
		scripted = map[string][]javaMutation{"S1": {{"unitPrice.multiply(", "unitPrice.add("}}}
	default:
		scripted = map[string][]javaMutation{"S1": {{"MAX_PERCENT - clampPercent(percent)", "MAX_PERCENT + clampPercent(percent)"}}}
	}
	units := javaSliceUnits(user)
	var mutants []map[string]any
	for _, slot := range []string{"S1", "S2", "S3"} {
		for _, change := range scripted[slot] {
			for qualname, unit := range units {
				if !strings.Contains(unit.source, change.from) || !strings.HasSuffix(unit.file, javaSuffix) {
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

type javaSliceUnit struct{ file, source string }

// javaSliceUnits reads the units of the code slice a generation prompt shows.
func javaSliceUnits(user string) map[string]javaSliceUnit {
	start := strings.Index(user, generationCode) + len(generationCode)
	end := strings.Index(user, generationClosing)
	units := map[string]javaSliceUnit{}
	for _, block := range strings.Split(user[start:end], "--- ")[1:] {
		header := javaUnitHeader.FindStringSubmatch(block)
		units[header[2]] = javaSliceUnit{file: header[1], source: strings.TrimRight(block[len(header[0]):], "\n")}
	}
	return units
}

// javaViolates reports whether the change touches what the statement is about.
func javaViolates(statement string, changedUnits []string) bool {
	for phrase, names := range javaSubjects {
		if !strings.Contains(statement, phrase) {
			continue
		}
		for _, unit := range changedUnits {
			unit = strings.TrimPrefix(unit, javaOwnerPrefix)
			for _, name := range names {
				if unit == name {
					return true
				}
			}
		}
	}
	return false
}

func javaChangedUnits(user string) []string {
	var units []string
	for _, match := range javaChangedPattern.FindAllStringSubmatch(user, -1) {
		units = append(units, match[2])
	}
	return units
}

func javaScreenAnswer(user string) string {
	units := javaChangedUnits(user[strings.Index(user, "Changed units"):])
	var answers []map[string]any
	for _, match := range javaRequirementBlock.FindAllStringSubmatch(user, -1) {
		choice := "CONSISTENT"
		if javaViolates(match[2], units) {
			choice = "VIOLATES"
		}
		answers = append(answers, map[string]any{"id": "q" + match[1], "choice": choice})
	}
	encoded, _ := json.Marshal(map[string]any{"answers": answers})
	return string(encoded)
}

func javaJudgeAnswer(user string) string {
	statement := user[:strings.Index(user, "Operations you may use")]
	if !javaViolates(statement, javaChangedUnits(user)) {
		return `{"validity":"NOT_VIOLATING","confidence":0.9}`
	}
	witness := javaFirstWitness(user)
	if witness == nil {
		return `{"validity":"VALID","confidence":0.9}`
	}
	encoded, _ := json.Marshal(map[string]any{"validity": "VALID", "confidence": 0.9, "witness": witness})
	return string(encoded)
}

// javaFirstWitness turns the first observed difference into a witness whose
// expected result is what the original returned.
func javaFirstWitness(user string) map[string]any {
	match := javaObservedPattern.FindStringSubmatch(user)
	if match == nil {
		return nil
	}
	args := map[string]any{}
	for _, pair := range strings.Split(match[2], ", ") {
		name, value, _ := strings.Cut(pair, "=")
		args[name] = javaLiteral(value)
	}
	expected := javaLiteral(match[3])
	expected["kind"] = "returns"
	return map[string]any{"operation": match[1], "args": args, "expected": expected}
}

func javaLiteral(text string) map[string]any {
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
