package golang

import (
	"slices"
	"strings"
	"unicode"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/vacuity"
)

// baselineRunsPerPath is how many times each baseline of a coverage binary
// runs: a block is stable only when every run gives it the same count.
const baselineRunsPerPath = vacuity.RunsPerOutcome

// runCounts maps the position of each block a run counted at least once to its
// count; a block it does not list counted zero.
type runCounts map[string]int64

// countsRun keeps the blocks of a profile that ran.
func countsRun(profile blockCounts) runCounts {
	counts := runCounts{}
	for position, entry := range profile {
		if entry.count > 0 {
			counts[position] = entry.count
		}
	}
	return counts
}

// baselineRun is what one run of a baseline left: the blocks it ran, and
// whether it is usable, which it is not when it left no profile, exited
// non-zero or timed out.
type baselineRun struct {
	counts runCounts
	usable bool
}

// baseline is what the runs of a coverage binary that start no test, or that
// start an ancestor of a leaf and none of its subtests, count: the count of
// each block every run gave the same count, zero for a block none ran, and the
// blocks they did not agree on. A missing baseline had a run that is not
// usable.
type baseline struct {
	counts   runCounts
	unstable map[string]bool
	missing  bool
}

// baselineOf reads the runs of one baseline.
func baselineOf(runs []baselineRun) baseline {
	if len(runs) == 0 || slices.ContainsFunc(runs, func(run baselineRun) bool { return !run.usable }) {
		return baseline{missing: true}
	}
	result := baseline{counts: runCounts{}, unstable: map[string]bool{}}
	for _, run := range runs {
		for position := range run.counts {
			if _, read := result.counts[position]; read || result.unstable[position] {
				continue
			}
			count := runs[0].counts[position]
			if slices.ContainsFunc(runs, func(other baselineRun) bool { return other.counts[position] != count }) {
				result.unstable[position] = true
				continue
			}
			result.counts[position] = count
		}
	}
	return result
}

// baselineChain is the baselines of a leaf, from the start-up of its binary
// down to its immediate baseline: the run of its parent with no subtest, or
// the start-up for a top-level test.
type baselineChain []baseline

// missing reports whether a baseline of the chain is missing: the leaf then
// owns nothing.
func (c baselineChain) missing() bool {
	return len(c) == 0 || slices.ContainsFunc(c, func(b baseline) bool { return b.missing })
}

// owns reports whether a leaf that ran the block the given number of times
// owns it: the block is stable in every baseline of the chain and the leaf ran
// it more often than its immediate baseline did.
func (c baselineChain) owns(position string, count int64) bool {
	if c.missing() || slices.ContainsFunc(c, func(b baseline) bool { return b.unstable[position] }) {
		return false
	}
	return count > c[len(c)-1].counts[position]
}

// lineHolders records, for one line, whether the leaf ran a block holding code
// on it that it owns, and one that it does not own.
type lineHolders struct {
	owned, unowned bool
}

// leafCoverage reads a leaf's profile against its baseline chain: the lines of
// every block it ran, and among them the lines it owns. A line is owned when the
// leaf owns a block holding code on it and owns every block holding code on it
// that it ran, or when it is a clause header that the leaf matched (see
// ownedClauseHeaders); a leaf with a missing baseline owns no line. A block of a file
// outside the module is left out.
func leafCoverage(profile blockCounts, chain baselineChain, source moduleSource, module string) vacuity.Coverage {
	var ran []vacuity.Line
	holders := map[vacuity.Line]lineHolders{}
	for position, entry := range profile {
		if entry.count == 0 {
			continue
		}
		lines := entry.block.lines(module)
		ran = append(ran, lines...)
		if chain.missing() {
			continue
		}
		owned := chain.owns(position, entry.count)
		for _, line := range lines {
			text, readable := source.line(line.File, line.Line)
			if readable && !entry.block.holdsCode(line.Line, text) {
				continue
			}
			holder := holders[line]
			holder.owned = holder.owned || owned
			holder.unowned = holder.unowned || !owned
			holders[line] = holder
		}
	}
	var ownedLines []vacuity.Line
	for line, holder := range holders {
		if holder.owned && !holder.unowned {
			ownedLines = append(ownedLines, line)
		}
	}
	ownedLines = append(ownedLines, ownedClauseHeaders(profile, chain, source, module)...)
	coverage := vacuity.Coverage{Lines: sortedLines(ran)}
	if ownedLines != nil {
		coverage.Owned = sortedLines(ownedLines)
	}
	return coverage
}

// holdsCode reports whether the block's span on the line, read from the line's
// text, has a character other than whitespace or a brace. Columns count bytes
// from one, and a block ends before its end column.
func (b profileBlock) holdsCode(line int, text string) bool {
	from, to := 1, len(text)+1
	if line == b.first {
		from = b.firstColumn
	}
	if line == b.last {
		to = b.lastColumn
	}
	from, to = min(max(from, 1), len(text)+1), min(max(to, 1), len(text)+1)
	if from >= to {
		return false
	}
	return strings.ContainsFunc(text[from-1:to-1], isCode)
}

// isCode reports whether a character of a block's span is code: anything but
// whitespace and the braces that open and close a block.
func isCode(r rune) bool {
	return !unicode.IsSpace(r) && r != openingBrace && r != closingBrace
}

// The braces of a Go block.
const (
	openingBrace = '{'
	closingBrace = '}'
)

// positionedBlock is a block of a profile with the position that names it.
type positionedBlock struct {
	position string
	countedBlock
}

// blocksByFile groups the blocks of a profile, whatever their count, by their
// file relative to the module; a block of a file outside it is left out.
func (c blockCounts) blocksByFile(module string) map[string][]positionedBlock {
	byFile := map[string][]positionedBlock{}
	for position, entry := range c {
		if relative, inside := entry.block.relativeFile(module); inside {
			byFile[relative] = append(byFile[relative], positionedBlock{position: position, countedBlock: entry})
		}
	}
	return byFile
}

// ownedClauseHeaders gives the lines of the clause headers a leaf owns: a
// header written on one line, entered by no fallthrough, that no block of the
// profile holds code on, is owned when the leaf owns the block that starts one
// column after its colon, which only a match of the clause enters.
func ownedClauseHeaders(profile blockCounts, chain baselineChain, source moduleSource, module string) []vacuity.Line {
	if chain.missing() {
		return nil
	}
	var owned []vacuity.Line
	for file, blocks := range profile.blocksByFile(module) {
		for _, header := range source.clauseHeaders(file) {
			if holdsCodeOnLine(blocks, file, header.line, source) {
				continue
			}
			if matched, found := blockStartingAt(blocks, header.line, header.colonColumn+1); found && chain.owns(matched.position, matched.count) {
				owned = append(owned, vacuity.Line{File: file, Line: header.line})
			}
		}
	}
	return owned
}

// holdsCodeOnLine reports whether a block holds code on the line of a file.
func holdsCodeOnLine(blocks []positionedBlock, file string, line int, source moduleSource) bool {
	text, readable := source.line(file, line)
	if !readable {
		return true
	}
	return slices.ContainsFunc(blocks, func(entry positionedBlock) bool {
		return entry.block.first <= line && line <= entry.block.last && entry.block.holdsCode(line, text)
	})
}

// blockStartingAt finds the block that starts at the column of a line.
func blockStartingAt(blocks []positionedBlock, line, column int) (positionedBlock, bool) {
	index := slices.IndexFunc(blocks, func(entry positionedBlock) bool {
		return entry.block.first == line && entry.block.firstColumn == column
	})
	if index < 0 {
		return positionedBlock{}, false
	}
	return blocks[index], true
}
