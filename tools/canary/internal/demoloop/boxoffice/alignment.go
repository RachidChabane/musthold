package boxoffice

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/RachidChabane/spec-proof/format/formattest/parcel"
)

// Readings of an alignment host.
const (
	noneChoice      = "NONE"
	fullCoverage    = "FULL"
	partialCoverage = "PARTIAL"
)

var (
	constantLine  = regexp.MustCompile(`(?m)^(\w+) = (-?\d+)\s*$`)
	assertionLine = regexp.MustCompile(`assert ticket_price\(age=(\w+)\) == (\w+)`)
)

// Alignment is the honest alignment host of the project: it reads the ages a
// masked test asserts prices for, names the criterion that fixes most of them,
// and judges the test to cover it in full when it reaches every bound of the
// criterion, and a strict part of it otherwise.
func Alignment() parcel.AlignmentOracle {
	return parcel.AlignmentOracle{Reading: honestReading}
}

func honestReading(unit string, tokens map[string]string) (string, string) {
	constants := map[string]string{}
	for _, match := range constantLine.FindAllStringSubmatch(unit, -1) {
		constants[match[1]] = match[2]
	}
	counts := map[string]int{}
	asserted := map[int]bool{}
	for _, match := range assertionLine.FindAllStringSubmatch(unit, -1) {
		age, err := strconv.Atoi(resolve(match[1], constants))
		if err != nil {
			continue
		}
		if owner, known := criterionOfAge(age); known {
			counts[owner.phrase]++
			asserted[age] = true
		}
	}
	best, bestCount := criterion{}, 0
	for _, candidate := range criteria {
		if counts[candidate.phrase] > bestCount {
			best, bestCount = candidate, counts[candidate.phrase]
		}
	}
	if bestCount == 0 {
		return noneChoice, noneChoice
	}
	token := tokenOf(tokens, best.phrase)
	if token == noneChoice {
		return noneChoice, noneChoice
	}
	for _, bound := range best.bounds {
		if !asserted[bound] {
			return token, partialCoverage
		}
	}
	return token, fullCoverage
}

func resolve(word string, constants map[string]string) string {
	if value, found := constants[word]; found {
		return value
	}
	return word
}

func tokenOf(tokens map[string]string, phrase string) string {
	for token, statement := range tokens {
		if strings.Contains(statement, phrase) {
			return token
		}
	}
	return noneChoice
}
