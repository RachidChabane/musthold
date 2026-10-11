package format

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Lengths of a commit id: SHA-1 and SHA-256 object names, in lowercase hex.
const (
	sha1CommitLength   = 40
	sha256CommitLength = Sha256HexLength
	uint64Bits         = 64
	firstLine          = 1
)

// IsCommitID reports whether s is a lowercase hex object name of SHA-1 or
// SHA-256 length.
func IsCommitID(s string) bool {
	if len(s) != sha1CommitLength && len(s) != sha256CommitLength {
		return false
	}
	return !strings.ContainsFunc(s, func(r rune) bool {
		return (r < '0' || r > '9') && (r < 'a' || r > 'f')
	})
}

// IsScanSeed reports whether s writes an unsigned 64-bit seed in decimal without
// a sign or a leading zero.
func IsScanSeed(s string) bool {
	if _, err := strconv.ParseUint(s, decimalBase, uint64Bits); err != nil {
		return false
	}
	return s == "0" || !strings.HasPrefix(s, "0")
}

func checkScanSubject(sink *collector, subject []Subject) {
	if !checkSingleSubject(sink, subject) {
		return
	}
	if !IsCommitID(subject[0].Name) {
		sink.add(CodeSubject, "subject[0].name", "%q is not a commit id", subject[0].Name)
	}
}

// scanIndex holds what the cross-references of a scan point at.
type scanIndex struct {
	mutants   map[string]ScanMutant
	promises  map[string]bool
	witnesses map[string]ScanWitness
}

func indexScan(p Scan) scanIndex {
	index := scanIndex{mutants: map[string]ScanMutant{}, promises: map[string]bool{}, witnesses: map[string]ScanWitness{}}
	for _, mutant := range p.Mutants {
		index.mutants[mutant.ID] = mutant
	}
	for _, promise := range p.Promises {
		index.promises[promise.ID] = true
	}
	for _, witness := range p.Witnesses {
		index.witnesses[witness.ID] = witness
	}
	return index
}

func (p Scan) check(sink *collector) {
	const path = "predicate"
	if len(p.Requirement) > 0 {
		sink.add(CodeRequirement, path+".requirement", "a scan proves no requirement")
	}
	checkNotEmpty(sink, path+".module.name", p.Module.Name)
	checkNotEmpty(sink, path+".module.version", p.Module.Version)
	checkMember(sink, path+".producedBy", p.ProducedBy, producers)
	checkScanMode(sink, path, p.Mode, p.Base)
	checkNotEmpty(sink, path+".runner", p.Runner)
	if !IsScanSeed(p.Seed) {
		sink.add(CodeUnknownValue, path+".seed", "%q is not a decimal unsigned 64-bit seed", p.Seed)
	}
	index := indexScan(p)
	checkScanPromises(sink, path, p, index)
	checkScanMutants(sink, path+".mutants", p.Mutants, index)
	checkScanTests(sink, path+".tests", p.Tests, index)
	checkScanCounts(sink, path+".counts", p.Counts, p.Tests)
}

func checkScanMode(sink *collector, path string, mode ScanMode, base string) {
	checkMember(sink, path+".mode", mode, scanModes)
	switch {
	case mode == ScanModeChange && !IsCommitID(base):
		sink.add(CodeMissingField, path+".base", "a change scan names its base commit, not %q", base)
	case mode == ScanModeRepository && base != "":
		sink.add(CodeUnknownValue, path+".base", "a repository scan has no base commit")
	}
}

func checkScanCounts(sink *collector, path string, counts ScanCounts, tests []ScanTest) {
	parts := []int{counts.VerifiesNothing, counts.Caught, counts.Unproven, counts.NotSampled, counts.NotGreen}
	total := 0
	for _, part := range append(parts, counts.Tests) {
		if part < 0 {
			sink.add(CodeScanCounts, path, "a count is negative")
			return
		}
	}
	for _, part := range parts {
		total += part
	}
	if total != counts.Tests {
		sink.add(CodeScanCounts, path+".tests", "the classes add up to %d, not %d", total, counts.Tests)
	}
	for _, class := range scanVerdicts {
		listed := 0
		for _, test := range tests {
			if test.Class == class {
				listed++
			}
		}
		if listed != counts.Of(class) {
			sink.add(CodeScanCounts, path, "%d tests are listed as %s, the count says %d", listed, class, counts.Of(class))
		}
	}
}

func checkScanMutants(sink *collector, path string, mutants []ScanMutant, index scanIndex) {
	checkAscending(sink, path, mutants, func(a, b ScanMutant) int { return strings.Compare(a.ID, b.ID) })
	for position, mutant := range mutants {
		checkScanMutant(sink, fmt.Sprintf("%s[%d]", path, position), mutant, index)
	}
}

func checkScanMutant(sink *collector, path string, mutant ScanMutant, index scanIndex) {
	checkDigest(sink, path+".id", mutant.ID)
	checkMember(sink, path+".origin", mutant.Origin, scanOrigins)
	checkScanEdits(sink, path+".edits", mutant.Edits)
	if digest, err := ScanEditsDigest(mutant.Edits); err == nil && IsSHA256Hex(mutant.ID) && digest != mutant.ID {
		sink.add(CodeMutantID, path+".id", "the edits digest to %s", digest)
	}
	checkScanLines(sink, path+".lines", mutant.Lines)
	if mutant.Sentinel && (mutant.Origin != ScanOriginOperator || mutant.Proven()) {
		sink.add(CodeSentinelNotEquivalent, path+".routes", "a sentinel is an operator mutant that keeps behaviour by construction")
	}
	checkAscending(sink, path+".routes", mutant.Routes, CompareScanRoutes)
	for position, route := range mutant.Routes {
		checkScanRoute(sink, fmt.Sprintf("%s.routes[%d]", path, position), route, index)
	}
}

func checkScanEdits(sink *collector, path string, edits []ScanEdit) {
	checkNotEmptyList(sink, path, edits)
	checkAscending(sink, path, edits, compareScanEdits)
	for position, edit := range edits {
		editPath := fmt.Sprintf("%s[%d]", path, position)
		checkScanPath(sink, editPath+".file", edit.File)
		checkNotEmpty(sink, editPath+".qualname", edit.Qualname)
		checkMember(sink, editPath+".target", edit.Target, scanEditTargets)
	}
}

func checkScanLines(sink *collector, path string, lines []ScanLine) {
	checkNotEmptyList(sink, path, lines)
	checkAscending(sink, path, lines, CompareScanLines)
	for position, line := range lines {
		linePath := fmt.Sprintf("%s[%d]", path, position)
		checkScanPath(sink, linePath+".file", line.File)
		if line.Line < firstLine {
			sink.add(CodeUnknownValue, linePath+".line", "line %d is before the first line", line.Line)
		}
	}
}

// CompareScanLines orders lines by file, then by line number.
func CompareScanLines(a, b ScanLine) int {
	return cmp.Or(strings.Compare(a.File, b.File), cmp.Compare(a.Line, b.Line))
}

func checkScanPath(sink *collector, path, value string) {
	if !IsRelativePOSIXPath(value) {
		sink.add(CodeUnknownValue, path, "%q is not a relative POSIX path", value)
	}
}

// CompareScanRoutes orders the routes of a mutant: by kind, then by every field
// of their evidence, so that only identical routes compare equal.
func CompareScanRoutes(a, b ScanRoute) int {
	return cmp.Or(
		strings.Compare(string(a.Kind), string(b.Kind)),
		strings.Compare(a.Test, b.Test),
		strings.Compare(a.Witness, b.Witness),
		strings.Compare(a.Input, b.Input),
		strings.Compare(string(a.Channel), string(b.Channel)),
		strings.Compare(a.Reference, b.Reference),
		strings.Compare(a.Mutant, b.Mutant),
	)
}

func checkScanRoute(sink *collector, path string, route ScanRoute, index scanIndex) {
	checkMember(sink, path+".kind", route.Kind, scanRouteKinds)
	killer := route.Test != ""
	differential := route.Input != "" || route.Channel != "" || route.Reference != "" || route.Mutant != ""
	witnessed := route.Witness != ""
	switch route.Kind {
	case ScanRouteKilledByTest:
		if !killer || differential || witnessed {
			sink.add(CodeScanRoute, path, "a killed_by_test route names its killing test and nothing else")
		}
	case ScanRouteDifferential:
		if killer || witnessed || route.Input == "" || route.Reference == route.Mutant {
			sink.add(CodeScanRoute, path, "an f3_difference route names its input and two different results, and nothing else")
		}
		checkMember(sink, path+".channel", route.Channel, scanChannels)
	case ScanRouteWitness:
		if !witnessed || killer || differential {
			sink.add(CodeScanRoute, path, "a witness_failure route names its witness and nothing else")
		}
		if witness, known := index.witnesses[route.Witness]; witnessed && (!known || !witness.Admitted) {
			sink.add(CodeScanReference, path+".witness", "%q is not an admitted witness of the scan", route.Witness)
		}
	}
}

func checkScanTests(sink *collector, path string, tests []ScanTest, index scanIndex) {
	checkAscending(sink, path, tests, func(a, b ScanTest) int { return strings.Compare(a.ID, b.ID) })
	for position, test := range tests {
		checkScanTest(sink, fmt.Sprintf("%s[%d]", path, position), test, index)
	}
}

func checkScanTest(sink *collector, path string, test ScanTest, index scanIndex) {
	checkNotEmpty(sink, path+".id", test.ID)
	checkMember(sink, path+".class", test.Class, scanVerdicts)
	switch {
	case test.Class == ScanUnproven:
		checkMember(sink, path+".reason", test.Reason, scanReasons)
	case test.Reason != "":
		sink.add(CodeScanClass, path+".reason", "only an unproven test has a reason")
	}
	checkAscending(sink, path+".changes", test.Changes, func(a, b ScanChange) int { return strings.Compare(a.Mutant, b.Mutant) })
	killed, survivedAll := false, len(test.Changes) > 0
	for position, change := range test.Changes {
		changePath := fmt.Sprintf("%s.changes[%d]", path, position)
		checkMember(sink, changePath+".outcome", change.Outcome, scanOutcomes)
		if mutant, known := index.mutants[change.Mutant]; !known || !mutant.Proven() || mutant.Sentinel {
			sink.add(CodeScanReference, changePath+".mutant", "%q is not a proven mutant of the scan", change.Mutant)
		}
		killed = killed || change.Outcome == ScanKilled
		survivedAll = survivedAll && change.Outcome == ScanSurvived
	}
	if test.Class == ScanVerifiesNothing && !survivedAll {
		sink.add(CodeScanClass, path+".changes", "a test that verifies nothing passes every proven change it ran, and ran some")
	}
	if test.Class == ScanCaught && !killed {
		sink.add(CodeScanClass, path+".changes", "a test that caught a change killed a proven mutant")
	}
	checkAscending(sink, path+".promises", test.Promises, strings.Compare)
	for position, promise := range test.Promises {
		if !index.promises[promise] {
			sink.add(CodeScanReference, fmt.Sprintf("%s.promises[%d]", path, position), "%q is not a promise of the scan", promise)
		}
	}
}

func checkScanPromises(sink *collector, path string, p Scan, index scanIndex) {
	if !p.PromisesChecked && (len(p.Promises) > 0 || len(p.Witnesses) > 0 || len(p.BrokenPromises) > 0 || len(p.Questions) > 0) {
		sink.add(CodeScanReference, path+".promisesChecked", "a scan that checked no promise holds no promise, witness, broken promise or question")
	}
	checkAscending(sink, path+".promises", p.Promises, func(a, b ScanPromise) int { return strings.Compare(a.ID, b.ID) })
	for position, promise := range p.Promises {
		promisePath := fmt.Sprintf("%s.promises[%d]", path, position)
		checkNotEmpty(sink, promisePath+".id", promise.ID)
		checkNotEmpty(sink, promisePath+".quote", promise.Quote)
		checkNotEmpty(sink, promisePath+".source.name", promise.Source.Name)
		checkDigest(sink, promisePath+".source.digest.sha256", promise.Source.Digest.SHA256)
	}
	checkAscending(sink, path+".witnesses", p.Witnesses, func(a, b ScanWitness) int { return strings.Compare(a.ID, b.ID) })
	for position, witness := range p.Witnesses {
		checkScanWitness(sink, fmt.Sprintf("%s.witnesses[%d]", path, position), witness, index)
	}
	checkAscending(sink, path+".brokenPromises", p.BrokenPromises, func(a, b ScanBrokenPromise) int {
		return cmp.Or(strings.Compare(a.Promise, b.Promise), strings.Compare(a.Mutant, b.Mutant), strings.Compare(a.Witness, b.Witness))
	})
	for position, broken := range p.BrokenPromises {
		checkBrokenPromise(sink, fmt.Sprintf("%s.brokenPromises[%d]", path, position), broken, index)
	}
	checkAscending(sink, path+".questions", p.Questions, func(a, b ScanQuestion) int {
		return cmp.Or(strings.Compare(a.Promise, b.Promise), strings.Compare(a.Witness, b.Witness))
	})
	for position, question := range p.Questions {
		witness, known := index.witnesses[question.Witness]
		if !known || witness.Promise != question.Promise || witness.Admitted {
			sink.add(CodeScanReference, fmt.Sprintf("%s.questions[%d]", path, position), "a question names a witness of its promise that the reference fails")
		}
	}
}

func checkScanWitness(sink *collector, path string, witness ScanWitness, index scanIndex) {
	checkDigest(sink, path+".id", witness.ID)
	checkNotEmpty(sink, path+".name", witness.Name)
	checkNotEmpty(sink, path+".code", witness.Code)
	if !index.promises[witness.Promise] {
		sink.add(CodeScanReference, path+".promise", "%q is not a promise of the scan", witness.Promise)
	}
	if witness.BlockBearing && !witness.Admitted {
		sink.add(CodeScanClass, path+".blockBearing", "a block-bearing witness is admitted")
	}
}

// checkBrokenPromise requires a broken promise to rest on a mutant that fails,
// on a witness_failure route, a block-bearing witness of that promise.
func checkBrokenPromise(sink *collector, path string, broken ScanBrokenPromise, index scanIndex) {
	witness, known := index.witnesses[broken.Witness]
	if !known || !witness.BlockBearing || witness.Promise != broken.Promise {
		sink.add(CodeScanReference, path+".witness", "%q is not a block-bearing witness of %q", broken.Witness, broken.Promise)
	}
	mutant, known := index.mutants[broken.Mutant]
	failsWitness := known && slices.ContainsFunc(mutant.Routes, func(route ScanRoute) bool {
		return route.Kind == ScanRouteWitness && route.Witness == broken.Witness
	})
	if !failsWitness || mutant.Sentinel {
		sink.add(CodeScanReference, path+".mutant", "%q has no witness_failure route on %q", broken.Mutant, broken.Witness)
	}
}
