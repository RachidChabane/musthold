// Package boxoffice is test support: the honest host of the demonstration's box
// office project. It reads the prompt an executor sees and answers the way a
// careful model would, judging every change against the spec's own prices, so
// that the project's gate can be proved before and after a change without any
// model call.
package boxoffice

import (
	"encoding/json"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/RachidChabane/spec-proof/format/formattest/parcel"
	"github.com/RachidChabane/spec-proof/format/formattest/prompts"
	"github.com/RachidChabane/spec-proof/format/hostproto"
)

// oldestAge bounds the ages the honest host reasons about.
const oldestAge = 130

// criterion is one requirement of the spec: the words its statement uses, the
// ages it covers and the price it fixes for them.
type criterion struct {
	phrase   string
	from, to int
	price    int
	// bounds are the ages a test must reach to check the whole criterion.
	bounds []int
}

var criteria = []criterion{
	{phrase: "12 to 64", from: 12, to: 64, price: 12, bounds: []int{12, 64}},
	{phrase: "65 or older", from: 65, to: oldestAge, price: 8, bounds: []int{65}},
	{phrase: "3 to 11", from: 3, to: 11, price: 6, bounds: []int{3, 11}},
	{phrase: "younger than 3", from: 0, to: 2, price: 0, bounds: []int{2}},
}

// criterionOf names the criterion a statement is about.
func criterionOf(statement string) (criterion, bool) {
	for _, candidate := range criteria {
		if strings.Contains(statement, candidate.phrase) {
			return candidate, true
		}
	}
	return criterion{}, false
}

// covers reports whether the criterion fixes the price of an age.
func (c criterion) covers(age int) bool { return age >= c.from && age <= c.to }

// criterionOfAge names the criterion that fixes the price of an age.
func criterionOfAge(age int) (criterion, bool) {
	for _, candidate := range criteria {
		if candidate.covers(age) {
			return candidate, true
		}
	}
	return criterion{}, false
}

// mutation is one scripted edit of a unit, by the criterion it targets.
type mutation struct{ slot, from, to string }

var mutations = map[string][]mutation{
	"12 to 64": {
		{"S1", "if age >= ADULT_FROM_AGE:", "if age > ADULT_FROM_AGE:"},
		{"S2", "return ADULT_PRICE", "return SENIOR_PRICE"},
	},
	"65 or older": {
		{"S1", "if age >= SENIOR_FROM_AGE:", "if age > SENIOR_FROM_AGE:"},
		{"S2", "return SENIOR_PRICE", "return CHILD_PRICE"},
	},
	"3 to 11": {
		{"S1", "if age >= CHILD_FROM_AGE:", "if age > CHILD_FROM_AGE:"},
		{"S2", "return CHILD_PRICE", "return SENIOR_PRICE"},
	},
	"younger than 3": {
		{"S1", "return FREE", "return CHILD_PRICE"},
	},
}

// Oracle plays an honest host for the discriminating-power module of the box
// office project.
type Oracle struct{}

var observedPattern = regexp.MustCompile(`(?m)^- (\w+)\(age=(-?[0-9.]+)\): original (\S+), mutant (\S+)$`)

// Answer implements hostprototest.AnswerSource.
func (Oracle) Answer(_ string, view hostproto.ExecutorView) (string, error) {
	schema := string(view.ResponseSchema)
	switch {
	case len(view.Closed) > 0:
		return screenAnswer(view.User), nil
	case strings.Contains(schema, `"witnesses"`):
		return witnessAnswer(view.User), nil
	case strings.Contains(schema, `"verdicts"`):
		return parcel.ScopeAnswer(view.User), nil
	case strings.Contains(schema, `"mutants"`):
		return generationAnswer(view.User), nil
	case strings.Contains(schema, `"equivalent"`):
		return `{"equivalent":"EQUIVALENT","confidence":0.95}`, nil
	default:
		return judgeAnswer(view.User), nil
	}
}

func generationAnswer(user string) string {
	statement := user[:max(strings.Index(user, "Code under test"), 0)]
	target, _ := criterionOf(statement)
	units := prompts.Units(user)
	mutants := []map[string]any{}
	for _, change := range mutations[target.phrase] {
		for _, qualname := range slices.Sorted(maps.Keys(units)) {
			unit := units[qualname]
			if !strings.Contains(unit.Source, change.from) {
				continue
			}
			mutants = append(mutants, map[string]any{
				"slot": change.slot, "mechanism": "replace " + change.from + " by " + change.to,
				"edits": []map[string]string{{"file": unit.File, "qualname": qualname, "new_source": strings.Replace(unit.Source, change.from, change.to, 1)}},
			})
		}
	}
	encoded, _ := json.Marshal(map[string]any{"mutants": mutants})
	return string(encoded)
}

// observation is one input on which the original and the mutant differ.
type observation struct {
	operation          string
	age                int
	original, mutation string
}

// observations reads the differences a prompt shows, keeping the inputs the
// spec prices: whole ages of a person.
func observations(user string) []observation {
	var found []observation
	for _, match := range observedPattern.FindAllStringSubmatch(user, -1) {
		age, err := strconv.Atoi(match[2])
		if err != nil || age < 0 || age > oldestAge {
			continue
		}
		found = append(found, observation{operation: match[1], age: age, original: match[3], mutation: match[4]})
	}
	return found
}

// breaking returns the first observed input on which the change breaks the
// criterion: the original charged the criterion's price and the mutant does not.
func breaking(target criterion, user string) (observation, bool) {
	price := strconv.Itoa(target.price)
	for _, seen := range observations(user) {
		if target.covers(seen.age) && seen.original == price && seen.mutation != price {
			return seen, true
		}
	}
	return observation{}, false
}

// screenAnswer answers a closed screening request the way an honest host does.
func screenAnswer(user string) string {
	var answers []map[string]any
	for _, requirement := range prompts.Requirements(user) {
		choice := "CONSISTENT"
		if target, known := criterionOf(requirement.Statement); known {
			if _, broken := breaking(target, user); broken {
				choice = "VIOLATES"
			}
		}
		answers = append(answers, map[string]any{"id": "q" + requirement.Number, "choice": choice})
	}
	encoded, _ := json.Marshal(map[string]any{"answers": answers})
	return string(encoded)
}

func judgeAnswer(user string) string {
	end := strings.Index(user, "Operations you may use")
	if end < 0 {
		end = strings.Index(user, "Changed units")
	}
	target, known := criterionOf(user[:max(end, 0)])
	if !known {
		return `{"validity":"NOT_VIOLATING","confidence":0.9}`
	}
	seen, broken := breaking(target, user)
	if !broken {
		return `{"validity":"NOT_VIOLATING","confidence":0.9}`
	}
	witness := map[string]any{
		"operation": seen.operation,
		"args":      map[string]any{"age": map[string]any{"type": "integer", "value": strconv.Itoa(seen.age)}},
		"expected":  map[string]any{"kind": "returns", "type": "integer", "value": strconv.Itoa(target.price)},
	}
	encoded, _ := json.Marshal(map[string]any{"validity": "VALID", "confidence": 0.9, "witness": witness})
	return string(encoded)
}
