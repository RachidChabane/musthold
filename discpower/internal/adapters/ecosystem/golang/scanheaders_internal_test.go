package golang

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/vacuity"
)

const (
	dispatchFile        = "dispatch.go"
	dispatchProfileFile = tallyModule + "/" + dispatchFile
	verifiesHeaders     = " [verifies SP-2101~4]"
)

// textSource is the source files of a module, by path relative to its root.
type textSource map[string]string

func (s textSource) line(file string, line int) (string, bool) {
	content, found := s[file]
	lines := strings.Split(content, lineSeparator)
	if !found || line < 1 || line > len(lines) {
		return "", false
	}
	return lines[line-1], true
}

func (s textSource) clauseHeaders(file string) []clauseHeader {
	content, found := s[file]
	if !found {
		return nil
	}
	return clauseHeadersOf([]byte(content))
}

// dispatchSource is the source of dispatch.go that the profiles below count.
const dispatchSource = `package tally

func Grade(score int) string {
	switch {
	case score < 0:
		return "invalid"
	case score < 50:
		return "low"
	default:
		return "high"
	}
}

func Carry(step int) int {
	total := 0
	switch step {
	case 1:
		total++
		fallthrough
	case 2:
		total += 10
	case 3:
	case 4,
		5:
		total = 7
	}
	return total
}

func Shape(value any) string {
	switch value.(type) {
	case int:
		return "int"
	case string:
		return "string"
	}
	return "other"
}

func Await(ready, done chan int) int {
	select {
	case <-ready:
		return 1
	case <-done:
		return 2
	}
}

func Band(x int) int {
	if x > 5 {
		return 1
	} else if x < -5 {
		return 2
	}
	return 0
}

func Probe(f func() bool) int {
	switch {
	case func() bool { return f() }():
		return 1
	}
	return 0
}

func Drain(items []int) int {
	count := 0
retry:
	for range items {
		count++
		if count > 3 {
			goto retry
		}
	}
	return count
}
`

var dispatchFiles = textSource{dispatchFile: dispatchSource}

// The blocks of dispatch.go, by the position a cover profile names them with.
const (
	gradeEntry      = "3.30,4.9"
	negativeBody    = "5.17,6.19"
	lowBody         = "7.18,8.15"
	highBody        = "9.10,10.16"
	carryEntry      = "14.26,16.14"
	stepOneBody     = "17.9,19.14"
	stepTwoBody     = "20.9,21.14"
	stepThreeBody   = "22.9,22.9"
	stepFourBody    = "24.5,25.12"
	carryReturn     = "27.2,27.14"
	shapeEntry      = "30.30,31.22"
	intBody         = "32.11,33.15"
	stringBody      = "34.14,35.18"
	shapeFallback   = "37.2,37.16"
	awaitEntry      = "40.38,41.9"
	readyBody       = "42.15,43.11"
	doneBody        = "44.14,45.11"
	bandEntry       = "49.22,50.11"
	bandFirstBranch = "50.11,52.3"
	bandCondition   = "52.8,52.19"
	bandLastBranch  = "52.19,54.3"
	bandFallback    = "55.2,55.10"
	probeEntry      = "58.31,59.9"
	closureBody     = "60.19,60.33"
	probeBody       = "60.36,61.11"
	probeFallback   = "63.2,63.10"
	drainEntry      = "66.29,69.18"
	drainLoopBody   = "69.18,71.16"
	drainRetry      = "71.16,72.14"
	drainReturn     = "75.2,75.14"
)

// The lines of dispatch.go: clause headers, and the lines around an else-if and
// a label.
var (
	negativeHeader  = dispatchLine(5)
	lowHeader       = dispatchLine(7)
	defaultHeader   = dispatchLine(9)
	stepOneHeader   = dispatchLine(17)
	stepTwoHeader   = dispatchLine(20)
	stepThreeHeader = dispatchLine(22)
	stepFourFirst   = dispatchLine(23)
	stepFourLast    = dispatchLine(24)
	intHeader       = dispatchLine(32)
	stringHeader    = dispatchLine(34)
	readyHeader     = dispatchLine(42)
	elseIfLine      = dispatchLine(52)
	closingLine     = dispatchLine(54)
	closureHeader   = dispatchLine(60)
	labelLine       = dispatchLine(68)
)

func dispatchLine(number int) vacuity.Line { return vacuity.Line{File: dispatchFile, Line: number} }

// dispatchBlocks lists every block of dispatch.go.
var dispatchBlocks = []string{
	gradeEntry, negativeBody, lowBody, highBody,
	carryEntry, stepOneBody, stepTwoBody, stepThreeBody, stepFourBody, carryReturn,
	shapeEntry, intBody, stringBody, shapeFallback,
	awaitEntry, readyBody, doneBody,
	bandEntry, bandFirstBranch, bandCondition, bandLastBranch, bandFallback,
	probeEntry, closureBody, probeBody, probeFallback,
	drainEntry, drainLoopBody, drainRetry, drainReturn,
}

// dispatchProfile is the profile of dispatch.go in which each block counted the
// given number of times, zero for a block not listed.
func dispatchProfile(counts map[string]int64) blockCounts {
	lines := make([]string, 0, len(dispatchBlocks))
	for _, block := range dispatchBlocks {
		lines = append(lines, fmt.Sprintf("%s:%s 1 %d", dispatchProfileFile, block, counts[block]))
	}
	return profileOf(lines...)
}

// dispatchCoverage is what a leaf that counted the blocks leaves against a
// start-up that counted the baseline's.
func dispatchCoverage(leaf, startUp map[string]int64) vacuity.Coverage {
	run := baselineRun{counts: countsRun(dispatchProfile(startUp)), usable: true}
	chain := baselineChain{baselineOf(slices.Repeat([]baselineRun{run}, baselineRunsPerPath))}
	return leafCoverage(dispatchProfile(leaf), chain, dispatchFiles, tallyModule)
}

func ownsLine(coverage vacuity.Coverage, line vacuity.Line) bool {
	return slices.Contains(coverage.Owned, line)
}

func TestClauseHeaderOwnership(t *testing.T) {
	t.Run("a header whose body the test owns is owned"+verifiesHeaders, func(t *testing.T) {
		coverage := dispatchCoverage(map[string]int64{lowBody: 1}, nil)
		if !ownsLine(coverage, lowHeader) || !slices.Contains(coverage.Lines, lowHeader) {
			t.Fatalf("coverage = %+v", coverage)
		}
		if ownsLine(coverage, negativeHeader) || ownsLine(coverage, defaultHeader) {
			t.Fatalf("owned = %v", coverage.Owned)
		}
	})
	t.Run("a header whose body ran only at start-up is not owned"+verifiesHeaders, func(t *testing.T) {
		ran := map[string]int64{lowBody: 1}
		coverage := dispatchCoverage(ran, ran)
		if ownsLine(coverage, lowHeader) || !slices.Contains(coverage.Lines, lowHeader) {
			t.Fatalf("coverage = %+v", coverage)
		}
	})
	t.Run("a header whose body ran more often than at start-up is owned"+verifiesHeaders, func(t *testing.T) {
		coverage := dispatchCoverage(map[string]int64{lowBody: 3}, map[string]int64{lowBody: 1})
		if !ownsLine(coverage, lowHeader) {
			t.Fatalf("owned = %v", coverage.Owned)
		}
	})
	t.Run("the header a fallthrough enters is not owned, and the header of the clause that falls is"+verifiesHeaders, func(t *testing.T) {
		coverage := dispatchCoverage(map[string]int64{carryEntry: 1, stepOneBody: 1, stepTwoBody: 1, carryReturn: 1}, nil)
		if !ownsLine(coverage, stepOneHeader) || ownsLine(coverage, stepTwoHeader) {
			t.Fatalf("owned = %v", coverage.Owned)
		}
		if !slices.Contains(coverage.Lines, stepTwoHeader) || !ownsLine(coverage, dispatchLine(21)) {
			t.Fatalf("coverage = %+v", coverage)
		}
	})
	t.Run("an empty clause is owned through its zero-width block"+verifiesHeaders, func(t *testing.T) {
		coverage := dispatchCoverage(map[string]int64{stepThreeBody: 1}, nil)
		if !ownsLine(coverage, stepThreeHeader) {
			t.Fatalf("coverage = %+v", coverage)
		}
	})
	t.Run("a type-switch header and a comm-clause header are owned"+verifiesHeaders, func(t *testing.T) {
		coverage := dispatchCoverage(map[string]int64{intBody: 1, readyBody: 1}, nil)
		if !ownsLine(coverage, intHeader) || !ownsLine(coverage, readyHeader) || ownsLine(coverage, stringHeader) {
			t.Fatalf("owned = %v", coverage.Owned)
		}
	})
	t.Run("neither line of a header written on two lines is owned"+verifiesHeaders, func(t *testing.T) {
		coverage := dispatchCoverage(map[string]int64{stepFourBody: 1}, nil)
		if ownsLine(coverage, stepFourFirst) || ownsLine(coverage, stepFourLast) || !ownsLine(coverage, dispatchLine(25)) {
			t.Fatalf("owned = %v", coverage.Owned)
		}
	})
	t.Run("a header that holds an unowned closure is not owned"+verifiesHeaders, func(t *testing.T) {
		ranAtStartUp := map[string]int64{closureBody: 1}
		coverage := dispatchCoverage(map[string]int64{closureBody: 1, probeBody: 1}, ranAtStartUp)
		if ownsLine(coverage, closureHeader) || !ownsLine(coverage, dispatchLine(61)) {
			t.Fatalf("owned = %v", coverage.Owned)
		}
	})
	t.Run("an else-if line is not owned when only the first branch ran"+verifiesHeaders, func(t *testing.T) {
		coverage := dispatchCoverage(map[string]int64{bandEntry: 1, bandFirstBranch: 1}, nil)
		if ownsLine(coverage, elseIfLine) || ownsLine(coverage, closingLine) {
			t.Fatalf("owned = %v", coverage.Owned)
		}
	})
	t.Run("an else line, a label line and a closing brace line are not owned through a clause header"+verifiesHeaders, func(t *testing.T) {
		everything := map[string]int64{}
		for _, block := range dispatchBlocks {
			everything[block] = 1
		}
		headerLines := ownedClauseHeaders(dispatchProfile(everything), baselineChain{stableBaseline()}, dispatchFiles, tallyModule)
		for _, line := range []vacuity.Line{labelLine, closingLine, dispatchLine(53), dispatchLine(51)} {
			if slices.Contains(headerLines, line) {
				t.Fatalf("line %v owned through a clause header in %v", line, headerLines)
			}
		}
	})
	t.Run("source that cannot be read gives no clause header"+verifiesHeaders, func(t *testing.T) {
		profile := dispatchProfile(map[string]int64{lowBody: 1})
		if got := ownedClauseHeaders(profile, baselineChain{stableBaseline()}, textSource{}, tallyModule); got != nil {
			t.Fatalf("owned = %v", got)
		}
	})
	t.Run("source that does not parse gives no clause header"+verifiesHeaders, func(t *testing.T) {
		broken := textSource{dispatchFile: strings.Replace(dispatchSource, "func Grade(score int) string {", "func Grade(score int) string", 1)}
		profile := dispatchProfile(map[string]int64{lowBody: 1})
		if got := ownedClauseHeaders(profile, baselineChain{stableBaseline()}, broken, tallyModule); got != nil {
			t.Fatalf("owned = %v", got)
		}
	})
	t.Run("a leaf with a missing baseline owns no clause header"+verifiesHeaders, func(t *testing.T) {
		profile := dispatchProfile(map[string]int64{lowBody: 1})
		if got := ownedClauseHeaders(profile, nil, dispatchFiles, tallyModule); got != nil {
			t.Fatalf("owned = %v", got)
		}
	})
	t.Run("a header whose body an unsteady baseline counted unevenly is not owned"+verifiesHeaders, func(t *testing.T) {
		unsteady := baselineOf([]baselineRun{
			{counts: countsRun(dispatchProfile(map[string]int64{lowBody: 1})), usable: true},
			{counts: countsRun(dispatchProfile(map[string]int64{lowBody: 2})), usable: true},
			{counts: countsRun(dispatchProfile(nil)), usable: true},
		})
		profile := dispatchProfile(map[string]int64{lowBody: 9})
		if got := ownedClauseHeaders(profile, baselineChain{unsteady}, dispatchFiles, tallyModule); got != nil {
			t.Fatalf("owned = %v", got)
		}
	})
}

func TestClauseHeadersOfSource(t *testing.T) {
	t.Run("the headers are the clauses written whole on one line that no fallthrough enters"+verifiesHeaders, func(t *testing.T) {
		got := clauseHeadersOf([]byte(dispatchSource))
		want := []clauseHeader{
			{line: 5, colonColumn: 16}, {line: 7, colonColumn: 17}, {line: 9, colonColumn: 9},
			{line: 17, colonColumn: 8}, {line: 22, colonColumn: 8},
			{line: 32, colonColumn: 10}, {line: 34, colonColumn: 13},
			{line: 42, colonColumn: 14}, {line: 44, colonColumn: 13},
			{line: 60, colonColumn: 35},
		}
		if !slices.Equal(got, want) {
			t.Fatalf("headers = %v", got)
		}
	})
	t.Run("a fallthrough followed by an empty statement still enters the next header"+verifiesHeaders, func(t *testing.T) {
		source := "package p\n\nfunc F(n int) int {\n\tswitch n {\n\tcase 1:\n\t\tfallthrough;\n\tcase 2:\n\t\treturn 2\n\t}\n\treturn 0\n}\n"
		got := clauseHeadersOf([]byte(source))
		if want := []clauseHeader{{line: 5, colonColumn: 8}}; !slices.Equal(got, want) {
			t.Fatalf("headers = %v", got)
		}
	})
}

func TestOwnedLinesAreRunLines(t *testing.T) {
	t.Run("the owned lines of a leaf are a subset of the lines it ran"+verifiesHeaders, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			leaf, startUp := map[string]int64{}, map[string]int64{}
			for _, block := range dispatchBlocks {
				leaf[block] = rapid.Int64Range(0, 3).Draw(t, "leaf "+block)
				startUp[block] = rapid.Int64Range(0, 3).Draw(t, "start-up "+block)
			}
			coverage := dispatchCoverage(leaf, startUp)
			for _, owned := range coverage.Owned {
				if !slices.Contains(coverage.Lines, owned) {
					t.Fatalf("owned %v is not among the lines %v", owned, coverage.Lines)
				}
			}
		})
	})
}
