package proofs

import (
	"slices"
	"strings"

	"github.com/RachidChabane/spec-proof/format"
)

// The scanned repository of the scan fixture.
const (
	ScanCommit      = "3f2a9c1e5b7d4a6f8e0c2b4d6f8a0c2e4b6d8f0a"
	ScanBase        = "9b1d3f5a7c9e1b3d5f7a9c1e3b5d7f9a1c3e5b7d"
	ScanRunner      = "pytest"
	ScanSeed        = "7"
	ScanModule      = "spec-proof-dp"
	ScanCodeFile    = "src/invoicing/parse.py"
	ScanReadme      = "README.md"
	ScanVacuousTest = "tests/test_parse.py::test_parse_ok"
	ScanCaughtTest  = "tests/test_split.py::test_trailing_comma"
	ScanThinTest    = "tests/test_parse.py::test_parse_single"
	ScanPromiseID   = "P1"
	scanQualname    = "split_fields"
	scanPromise     = "Empty fields are kept: `a,` splits into `a` and an empty field."
	scanReadmeText  = "# invoicing\n\nParse invoice lines.\n\n" + scanPromise + "\n"
	scanTotalTests  = 412
	scanNotGreen    = 3
	scanFamilyCmp   = "relational"
	scanFamilyConst = "constant"
)

// scanSource is the reference definition every scan fixture mutant replaces.
const scanSource = `def split_fields(line):
    fields = []
    start = 0
    for index, char in enumerate(line):
        if char == ",":
            fields.append(line[start:index])
            start = index + 1
    if start <= len(line):
        fields.append(line[start:])
    return fields`

// scanOperatorMutant is one operator mutant of the fixture: the reference with
// one fragment replaced, on one line.
type scanOperatorMutant struct {
	from, to string
	line     int
	family   string
	route    format.ScanRoute
	sentinel bool
}

func scanOperatorMutants() []scanOperatorMutant {
	killed := format.ScanRoute{Kind: format.ScanRouteKilledByTest, Test: ScanCaughtTest}
	differs := func(input, reference, mutant string) format.ScanRoute {
		return format.ScanRoute{Kind: format.ScanRouteDifferential, Operation: scanQualname, Input: input, Channel: format.ScanChannelReturnValue, Reference: reference, Mutant: mutant}
	}
	return []scanOperatorMutant{
		{from: "start <= len(line)", to: "start < len(line)", line: 41, family: scanFamilyCmp, route: killed},
		{from: `char == ","`, to: `char != ","`, line: 38, family: scanFamilyCmp, route: differs(`"a,b"`, `["a", "b"]`, `["", "", ""]`)},
		{from: "start = index + 1", to: "start = index + 2", line: 40, family: scanFamilyConst, route: differs(`"a,b"`, `["a", "b"]`, `["a", ""]`)},
		{from: "start = 0", to: "start = 1", line: 36, family: scanFamilyConst, route: differs(`"ab,c"`, `["ab", "c"]`, `["b", "c"]`)},
		{from: "line[start:index]", to: "line[start:index - 1]", line: 39, family: scanFamilyConst, route: differs(`"ab,c"`, `["ab", "c"]`, `["a", "c"]`)},
		{from: "return fields", to: "return list(fields)", line: 43, family: "sentinel", sentinel: true},
	}
}

// scanModelSource is the definition of the fixture's model mutant: it drops a
// trailing empty field, which the README promises to keep.
const scanModelSource = `def split_fields(line):
    fields = line.split(",")
    if fields and fields[-1] == "":
        fields.pop()
    return fields`

// ScanWitnessImports are the lines the fixture's witness file starts with.
const ScanWitnessImports = "from invoicing.parse import split_fields"

// ScanWitnessCode and ScanQuestionCode are the two witnesses of the fixture's
// promise: the first passes the reference, the second does not.
const (
	ScanWitnessCode  = "def test_keeps_trailing_empty_field():\n    assert split_fields(\"a,\") == [\"a\", \"\"]"
	ScanQuestionCode = "def test_keeps_leading_empty_field():\n    assert split_fields(\",a\") == [\"\", \"a\", \"\"]"
)

func scanEdits(source string) []format.ScanEdit {
	return []format.ScanEdit{{File: ScanCodeFile, Qualname: scanQualname, NewSource: source}}
}

func scanMutant(origin format.ScanOrigin, family string, source string, line int) format.ScanMutant {
	edits := scanEdits(source)
	id, err := format.ScanEditsDigest(edits)
	if err != nil {
		panic(err)
	}
	return format.ScanMutant{ID: id, Origin: origin, Family: family, Edits: edits, Lines: []format.ScanLine{{File: ScanCodeFile, Line: line}}}
}

// ScanWitnessID is the id of a fixture witness: the digest of its code.
func ScanWitnessID(code string) string { return format.SHA256Hex([]byte(code)) }

// ScanReadmeDigest is the digest of the fixture's README, the source of its
// promise.
func ScanReadmeDigest() string { return format.SHA256Hex([]byte(ScanReadmeText())) }

// ScanReadmeText is the README the fixture's promise is quoted from.
func ScanReadmeText() string { return scanReadmeText }

// Scan builds a valid scan statement of a change: one test verifies nothing, one
// caught a change, one is unproven, and the host checked one promise, which a
// model mutant breaks. Each mutation edits it in turn.
func Scan(mutations ...func(*format.ScanStatement)) format.ScanStatement {
	statement := format.NewScanStatement(ScanCommit, Digest("tree/"+ScanCommit), scanPredicate())
	for _, mutate := range mutations {
		mutate(&statement)
	}
	return statement
}

func scanPredicate() format.Scan {
	witness := ScanWitnessID(ScanWitnessCode)
	question := ScanWitnessID(ScanQuestionCode)
	var mutants []format.ScanMutant
	var proven []string
	for _, operator := range scanOperatorMutants() {
		mutant := scanMutant(format.ScanOriginOperator, operator.family, strings.Replace(scanSource, operator.from, operator.to, 1), operator.line)
		mutant.Sentinel = operator.sentinel
		if !operator.sentinel {
			mutant.Routes = []format.ScanRoute{operator.route}
			proven = append(proven, mutant.ID)
		}
		mutants = append(mutants, mutant)
	}
	model := scanMutant(format.ScanOriginModel, "", scanModelSource, 37)
	model.Lines = []format.ScanLine{{File: ScanCodeFile, Line: 37}, {File: ScanCodeFile, Line: 38}, {File: ScanCodeFile, Line: 39}}
	model.Routes = []format.ScanRoute{{Kind: format.ScanRouteWitness, Witness: witness}}
	mutants = append(mutants, model)
	slices.SortFunc(mutants, func(a, b format.ScanMutant) int { return strings.Compare(a.ID, b.ID) })
	return format.Scan{
		Module:          format.Module{Name: ScanModule, Version: moduleVersion},
		ProducedBy:      format.ProducedByLocal,
		Mode:            format.ScanModeChange,
		Base:            ScanBase,
		Runner:          ScanRunner,
		Seed:            ScanSeed,
		PromisesChecked: true,
		Counts: format.ScanCounts{
			Tests: scanTotalTests, VerifiesNothing: 1, Caught: 1, Unproven: 1,
			NotSampled: scanTotalTests - scanNotGreen - 3, NotGreen: scanNotGreen,
		},
		Mutants: mutants,
		Tests:   scanTests(proven, model.ID),
		Promises: []format.ScanPromise{{
			ID: ScanPromiseID, Quote: scanPromise,
			Source: format.ScanSource{Name: ScanReadme, Digest: format.Digest{SHA256: ScanReadmeDigest()}},
		}},
		Witnesses: sortedScanWitnesses([]format.ScanWitness{
			{ID: witness, Promise: ScanPromiseID, Name: "test_keeps_trailing_empty_field", Imports: ScanWitnessImports, Code: ScanWitnessCode, Admitted: true, BlockBearing: true},
			{ID: question, Promise: ScanPromiseID, Name: "test_keeps_leading_empty_field", Imports: ScanWitnessImports, Code: ScanQuestionCode},
		}),
		BrokenPromises: []format.ScanBrokenPromise{{Promise: ScanPromiseID, Mutant: model.ID, Witness: witness}},
		Questions:      []format.ScanQuestion{{Promise: ScanPromiseID, Witness: question}},
	}
}

func scanTests(proven []string, model string) []format.ScanTest {
	changes := func(outcome func(mutant string) format.ScanOutcome, mutants ...string) []format.ScanChange {
		out := make([]format.ScanChange, len(mutants))
		for index, mutant := range mutants {
			out[index] = format.ScanChange{Mutant: mutant, Outcome: outcome(mutant)}
		}
		slices.SortFunc(out, func(a, b format.ScanChange) int { return strings.Compare(a.Mutant, b.Mutant) })
		return out
	}
	survives := func(string) format.ScanOutcome { return format.ScanSurvived }
	killsFirst := func(mutant string) format.ScanOutcome {
		if mutant == proven[0] {
			return format.ScanKilled
		}
		return format.ScanSurvived
	}
	tests := []format.ScanTest{
		{ID: ScanVacuousTest, Class: format.ScanVerifiesNothing, Changes: changes(survives, append(slices.Clone(proven), model)...), Promises: []string{ScanPromiseID}},
		{ID: ScanCaughtTest, Class: format.ScanCaught, Changes: changes(killsFirst, proven...)},
		{ID: ScanThinTest, Class: format.ScanUnproven, Reason: format.ScanTooFewChanges, Changes: changes(survives, proven[1]), NotStarted: []string{proven[0]}},
	}
	slices.SortFunc(tests, func(a, b format.ScanTest) int { return strings.Compare(a.ID, b.ID) })
	return tests
}

func sortedScanWitnesses(witnesses []format.ScanWitness) []format.ScanWitness {
	slices.SortFunc(witnesses, func(a, b format.ScanWitness) int { return strings.Compare(a.ID, b.ID) })
	return witnesses
}
