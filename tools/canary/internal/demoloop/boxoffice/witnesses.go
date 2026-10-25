package boxoffice

import (
	"encoding/json"
	"fmt"

	"github.com/RachidChabane/spec-proof/format/formattest/prompts"
)

const (
	targetHeader   = "Requirement to check:"
	witnessImports = "from boxoffice.prices import ticket_price\nimport random"
	exampleKind    = "example"
	propertyKind   = "property"
	propertySeed   = 7
	propertyDraws  = 200
)

// witness is one test an honest writer derives from the spec alone.
type witness struct {
	Name   string `json:"name"`
	Clause string `json:"clause"`
	Kind   string `json:"kind"`
	Code   string `json:"code"`
}

// witnessesOf writes the witnesses of a criterion: an example at each bound, one
// inside, and a property over every age it covers.
func witnessesOf(target criterion) []witness {
	clause := "WHEN the buyer is " + target.phrase
	ages := []int{target.from, target.to, (target.from + target.to) / 2}
	if target.to == oldestAge {
		ages = []int{target.from, target.from + 1, target.from + 25}
	}
	var written []witness
	for _, age := range ages {
		name := fmt.Sprintf("price_at_age_%d", age)
		written = append(written, witness{Name: name, Clause: clause, Kind: exampleKind,
			Code: fmt.Sprintf("def %s():\n    assert ticket_price(age=%d) == %d", name, age, target.price)})
	}
	property := fmt.Sprintf("every_age_from_%d_to_%d", target.from, target.to)
	written = append(written, witness{Name: property, Clause: clause, Kind: propertyKind,
		Code: fmt.Sprintf("def %s():\n    draws = random.Random(%d)\n    for _ in range(%d):\n        age = draws.randint(%d, %d)\n        assert ticket_price(age=age) == %d",
			property, propertySeed, propertyDraws, target.from, target.to, target.price)})
	return written
}

// witnessAnswer answers a witness request the way an honest writer does.
func witnessAnswer(user string) string {
	target, _ := criterionOf(prompts.SectionAfter(user, targetHeader))
	encoded, _ := json.Marshal(map[string]any{"imports": witnessImports, "witnesses": witnessesOf(target)})
	return string(encoded)
}
