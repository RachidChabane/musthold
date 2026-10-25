package node

import (
	"cmp"
	"maps"
	"slices"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/vacuity"
)

// RecordingsPerTest is how many processes record a test alone: a range is
// owned only when every one of them gave it the same count in the window.
const RecordingsPerTest = vacuity.RunsPerOutcome

// interval is what the counts of one take cover: the work between the
// previous take and this one.
type interval int

const (
	// outside is work outside the window: the process's start-up, its setup
	// files, module evaluation, describe bodies and the hooks no test owns.
	outside interval = iota
	// inWindow is the test's own work, from before its first beforeEach hook
	// to the settling of its function.
	inWindow
	// inSlice is a callback of foreign origin that ran inside the window.
	inSlice
)

// unitCounts are the counts of one code unit of a module: over the whole
// process, inside the window, and inside the slices cut out of it.
type unitCounts struct {
	total, window, foreign int
}

// unitKey names a code unit of a module the same way in every process: its
// file and its offset in the generated code.
type unitKey struct {
	file   string
	offset int
}

// processLines is what one process recorded of the lines of the project: the
// counts of the code units of each line, the test whose window it opened, and
// whether that window may own lines.
type processLines struct {
	counts map[vacuity.Line]map[unitKey]unitCounts
	// segments are, for a recording of several test files, the lines each
	// file's segment ran; the start-up's lines belong to every file.
	segments map[string]map[vacuity.Line]bool
	startUp  map[vacuity.Line]bool
	// owning is whether the window may own lines: the process shows every
	// precondition it needs, its window holds the test function, no module
	// was evaluated in it, no fake timer it cannot see was pending, and no
	// callback of foreign origin ran in it that could not be cut out.
	owning bool
	// callbacks is whether a callback of foreign origin ran in the window
	// while the async hooks could not cut it out.
	callbacks bool
	// readAgainstHollow is whether the window is read against hollow runs of
	// the test: the async hooks were blind to the realm's promises.
	readAgainstHollow bool
}

// ran is every line the process ran.
func (p processLines) ran() map[vacuity.Line]bool {
	lines := map[vacuity.Line]bool{}
	for line, units := range p.counts {
		for _, unit := range units {
			if unit.total > 0 {
				lines[line] = true
				break
			}
		}
	}
	return lines
}

// analyzeProcess reads the takes of a process: each take's counts cover the
// work since the previous take, whose kind the takes around it mark, or, for
// a process whose takes count from its start, the difference with the
// previous take. A window owns nothing when a take shows a module evaluated in
// it: the top-level function of a module of the project counting, or a module
// first counting there.
func analyzeProcess(process processRecord, shapes *geometries) processLines {
	result := processLines{counts: map[vacuity.Line]map[unitKey]unitCounts{}, segments: map[string]map[vacuity.Line]bool{}, startUp: map[vacuity.Line]bool{}}
	capable := process.capabilities.Inspector && process.capabilities.Retention && process.capabilities.Evaluation
	state := outside
	windowSeen, closed, tainted := false, false, false
	var marks takeRecord
	currentFile := ""
	seen := map[string]bool{}
	previous := map[string][]int32{}
	previousTop := map[string]int{}
	for _, take := range process.takes {
		kind := state
		for id, functions := range take.counts {
			script, known := process.scripts[id]
			if !known {
				continue
			}
			shape := shapes.of(script)
			painted, top := paint(functions, len(shape.lineOf)), topCount(functions, script.prefix, len(shape.lineOf))
			if process.cumulative {
				painted, previous[id] = minus(painted, previous[id]), painted
				top, previousTop[id] = top-previousTop[id], top
			}
			if kind != outside && (!seen[id] || top > 0) {
				tainted = true
			}
			seen[id] = true
			result.add(script.file, shape, painted, kind, currentFile)
		}
		switch take.at {
		case takeOpen:
			state, windowSeen = inWindow, true
		case takeSliceStart:
			if state == inWindow {
				state = inSlice
			}
		case takeSliceEnd:
			if state == inSlice {
				state = inWindow
			}
		case takeClose:
			state, closed, marks = outside, true, take
		case takeFile:
			currentFile = take.file
		}
	}
	result.callbacks = marks.callbacks
	result.readAgainstHollow = !process.capabilities.AsyncHooks
	result.owning = capable && windowSeen && closed && !tainted && marks.bounded && !marks.fakeUnseen &&
		(process.capabilities.AsyncHooks || !marks.callbacks || result.readAgainstHollow)
	return result
}

// topCount is the count of a module's own functions, which count when the
// module is evaluated: those that start within the runner's wrapper and end
// at the end of the module's code.
func topCount(functions [][][3]int, prefix, length int) int {
	count := 0
	for _, function := range functions {
		if len(function) > 0 && function[0][0] <= prefix && function[0][1] >= length-len(wrapperSuffix) {
			count += function[0][2]
		}
	}
	return count
}

// paint gives each offset of a module the count of the innermost range that
// holds it, painting from the widest range to the narrowest; an offset no
// range holds did not run.
func paint(functions [][][3]int, length int) []int32 {
	var all [][3]int
	for _, function := range functions {
		all = append(all, function...)
	}
	slices.SortStableFunc(all, func(a, b [3]int) int { return cmp.Compare(b[1]-b[0], a[1]-a[0]) })
	painted := make([]int32, length)
	for _, item := range all {
		for offset := max(item[0], 0); offset < min(item[1], length); offset++ {
			painted[offset] = int32(item[2])
		}
	}
	return painted
}

// minus is the counts of a take less those of the previous one.
func minus(painted, previous []int32) []int32 {
	delta := slices.Clone(painted)
	for offset := range min(len(delta), len(previous)) {
		delta[offset] -= previous[offset]
	}
	return delta
}

// add adds the counts of one take of a module to the lines its code is on.
func (p *processLines) add(file string, shape geometry, painted []int32, kind interval, segment string) {
	for offset, count := range painted {
		if count <= 0 {
			continue
		}
		line, holds := shape.at(offset)
		if !holds {
			continue
		}
		units := p.counts[line]
		if units == nil {
			units = map[unitKey]unitCounts{}
			p.counts[line] = units
		}
		key := unitKey{file: file, offset: offset}
		unit := units[key]
		unit.total += int(count)
		switch kind {
		case inWindow:
			unit.window += int(count)
		case inSlice:
			unit.foreign += int(count)
		}
		units[key] = unit
		p.markSegment(line, segment)
	}
}

func (p *processLines) markSegment(line vacuity.Line, file string) {
	if file == "" {
		p.startUp[line] = true
		return
	}
	if p.segments[file] == nil {
		p.segments[file] = map[vacuity.Line]bool{}
	}
	p.segments[file][line] = true
}

// segmentOf is what a test file's segment of a recording of several files
// ran, the start-up included.
func (p processLines) segmentOf(file string) map[vacuity.Line]bool {
	lines := maps.Clone(p.startUp)
	maps.Copy(lines, p.segments[file])
	return lines
}

// windowOf gives, for each code unit of the line that the process ran, its
// count inside the window; false when the line is not the window's own: some
// unit of it that ran never counted in the window, or counted inside a slice
// of foreign origin.
func (p processLines) windowOf(line vacuity.Line) (map[unitKey]int, bool) {
	units := p.counts[line]
	window := make(map[unitKey]int, len(units))
	for key, unit := range units {
		if unit.total == 0 {
			continue
		}
		if unit.window == 0 || unit.foreign > 0 {
			return nil, false
		}
		window[key] = unit.window
	}
	return window, len(window) > 0
}

// ownedLines applies the rule to the recordings of one test alone: a line is
// owned when, in every recording, each code unit on it that ran counted in
// the window and outside any slice of foreign origin, gave the same count in
// every recording, and, for windows read against hollow runs, counted more
// often than in the hollow runs, which agree with each other. A recording
// that may not own lines leaves the test owning none.
func ownedLines(leaves []processLines, hollows []processLines) []vacuity.Line {
	if len(leaves) != RecordingsPerTest {
		return nil
	}
	againstHollow := false
	for _, leaf := range leaves {
		if !leaf.owning {
			return nil
		}
		againstHollow = againstHollow || leaf.readAgainstHollow
	}
	var baseline map[vacuity.Line]map[unitKey]int
	if againstHollow {
		var stable bool
		if baseline, stable = hollowBaseline(hollows); !stable {
			// Failing a hollow baseline, a window in which any callback of
			// foreign origin ran owns nothing.
			for _, leaf := range leaves {
				if leaf.callbacks {
					return nil
				}
			}
			againstHollow = false
		}
	}
	var owned []vacuity.Line
	for line := range leaves[0].counts {
		if ownedLine(line, leaves, baseline, againstHollow) {
			owned = append(owned, line)
		}
	}
	slices.SortFunc(owned, compareLines)
	return owned
}

func ownedLine(line vacuity.Line, leaves []processLines, baseline map[vacuity.Line]map[unitKey]int, againstHollow bool) bool {
	first, own := leaves[0].windowOf(line)
	if !own {
		return false
	}
	for _, leaf := range leaves[1:] {
		window, own := leaf.windowOf(line)
		if !own || !maps.Equal(window, first) {
			return false
		}
	}
	if !againstHollow {
		return true
	}
	for key, count := range first {
		if count <= baseline[line][key] {
			return false
		}
	}
	return true
}

// hollowBaseline is the window counts of the hollow runs of a test, by line
// and code unit, when every hollow run recorded its window and they agree.
func hollowBaseline(hollows []processLines) (map[vacuity.Line]map[unitKey]int, bool) {
	if len(hollows) != RecordingsPerTest {
		return nil, false
	}
	var first map[vacuity.Line]map[unitKey]int
	for index, hollow := range hollows {
		if !hollow.owning {
			return nil, false
		}
		counts := map[vacuity.Line]map[unitKey]int{}
		for line, units := range hollow.counts {
			for key, unit := range units {
				if unit.window == 0 {
					continue
				}
				if counts[line] == nil {
					counts[line] = map[unitKey]int{}
				}
				counts[line][key] = unit.window
			}
		}
		if index == 0 {
			first = counts
			continue
		}
		if !maps.EqualFunc(first, counts, func(a, b map[unitKey]int) bool { return maps.Equal(a, b) }) {
			return nil, false
		}
	}
	return first, true
}

func compareLines(a, b vacuity.Line) int {
	if byFile := cmp.Compare(a.File, b.File); byFile != 0 {
		return byFile
	}
	return cmp.Compare(a.Line, b.Line)
}

// sortedLines lists a set of lines by file, then line.
func sortedLines(set map[vacuity.Line]bool, keep func(vacuity.Line) bool) []vacuity.Line {
	lines := make([]vacuity.Line, 0, len(set))
	for line := range set {
		if keep(line) {
			lines = append(lines, line)
		}
	}
	slices.SortFunc(lines, compareLines)
	return lines
}
