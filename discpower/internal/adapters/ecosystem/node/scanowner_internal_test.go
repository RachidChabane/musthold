package node

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/vacuity"
)

const (
	verifiesOwnership = " [verifies SP-2293~1]"
	verifiesFallbacks = " [verifies SP-2294~1]"
	verifiesSegments  = " [verifies SP-2295~1]"
	calcFile          = "src/calc.js"
	scriptID          = "7"
	lazyScriptID      = "8"
	lazyFile          = "src/lazy.js"
	// takeStart and takeEnd are the takes of the start and the end of a
	// process.
	takeStart = "start"
	takeEnd   = "end"
)

// calcSource is a module a recording runs, as its own generated code.
const calcSource = `function add(a, b) {
  return a + b;
}
function pick(flag) {
  return flag ? add(1, 1) : add(2, 2);
}
function tick(state) {
  state.n += 1;
}
`

const lazySource = "function lazy() {\n  return 1;\n}\n"

// span is the range [start, end) of the first occurrence of a text in the
// calc module, counted the given number of times.
func span(t *testing.T, source, text string, count int) [3]int {
	t.Helper()
	start := strings.Index(source, text)
	if start < 0 {
		t.Fatalf("%q is not in the source", text)
	}
	return [3]int{start, start + len(text), count}
}

// functionsOf are the functions of a take: each its own range first.
func functionsOf(ranges ...[][3]int) [][][3]int { return ranges }

// calcFunctions are the ranges of the calc module's functions.
type calcRanges struct {
	t *testing.T
}

func (c calcRanges) add(count int) [][3]int {
	return [][3]int{span(c.t, calcSource, "function add(a, b) {\n  return a + b;\n}", count)}
}

// pick runs pick count times, with the branch that calls add(1, 1) taken
// then times and the other one the rest.
func (c calcRanges) pick(count, then int) [][3]int {
	ranges := [][3]int{span(c.t, calcSource, "function pick(flag) {\n  return flag ? add(1, 1) : add(2, 2);\n}", count)}
	if then < count {
		ranges = append(ranges, span(c.t, calcSource, "add(1, 1)", then))
	}
	if then > 0 {
		ranges = append(ranges, span(c.t, calcSource, "add(2, 2)", count-then))
	}
	return ranges
}

func (c calcRanges) tick(count int) [][3]int {
	return [][3]int{span(c.t, calcSource, "function tick(state) {\n  state.n += 1;\n}", count)}
}

func (c calcRanges) module(count int) [][3]int {
	return [][3]int{{0, len(calcSource), count}}
}

// identityFiles is a workspace whose files are the modules the records name.
func identityFiles(t *testing.T, files map[string]string) projectFiles {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		target := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return projectFiles{roots: []string{dir}, read: func(file string) ([]byte, bool) {
		content, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(file)))
		return content, err == nil
	}}
}

// capable shows every precondition of the rule.
func capable() capabilities {
	return capabilities{Inspector: true, SynchronousTake: true, AsyncHooks: true, Evaluation: true, FakeTimers: true, Maps: true, Bounds: true, Retention: true}
}

// recordingOf is one process recording a test alone over the calc module.
func recordingOf(takes ...takeRecord) processRecord {
	return processRecord{
		recording: true, capabilities: capable(),
		scripts: map[string]scriptRecord{
			scriptID:     {file: calcFile, source: calcSource},
			lazyScriptID: {file: lazyFile, source: lazySource},
		},
		takes: takes,
	}
}

func take(at string, counts map[string][][][3]int) takeRecord {
	return takeRecord{at: at, counts: counts, bounded: at == takeClose}
}

// threeTimes is the same recording three times over.
func threeTimes(process processRecord) []processRecord {
	return []processRecord{process, process, process}
}

func ownedIn(t *testing.T, processes []processRecord, hollows []processRecord) []vacuity.Line {
	t.Helper()
	shapes := newGeometries(identityFiles(t, map[string]string{calcFile: calcSource, lazyFile: lazySource}))
	var leaves, baseline []processLines
	for _, process := range processes {
		leaves = append(leaves, analyzeProcess(process, shapes))
	}
	for _, process := range hollows {
		baseline = append(baseline, analyzeProcess(process, shapes))
	}
	return ownedLines(leaves, baseline)
}

func lines(numbers ...int) []vacuity.Line {
	var result []vacuity.Line
	for _, number := range numbers {
		result = append(result, vacuity.Line{File: calcFile, Line: number})
	}
	return result
}

// plainRecording runs the module at start-up, add twice in the window and pick
// with its other branch in afterEach.
func plainRecording(c calcRanges) processRecord {
	return recordingOf(
		take(takeStart, map[string][][][3]int{scriptID: functionsOf(c.module(1))}),
		take(takeOpen, nil),
		take(takeClose, map[string][][][3]int{scriptID: functionsOf(c.add(2))}),
		take(takeEnd, map[string][][][3]int{scriptID: functionsOf(c.pick(1, 0), c.add(1))}),
	)
}

func TestOwnershipOfRecordedTakes(t *testing.T) {
	c := calcRanges{t: t}
	t.Run("a line every code unit of which ran in the window of each of the three recordings, with the same counts, is owned"+verifiesOwnership, func(t *testing.T) {
		owned := ownedIn(t, threeTimes(plainRecording(c)), nil)
		if !slices.Equal(owned, lines(1, 2)) {
			t.Fatalf("owned %v", owned)
		}
	})
	t.Run("a line with a code unit that ran only in afterEach is not owned, though another unit of it ran in the window"+verifiesOwnership, func(t *testing.T) {
		recording := recordingOf(
			take(takeStart, map[string][][][3]int{scriptID: functionsOf(c.module(1))}),
			take(takeOpen, nil),
			take(takeClose, map[string][][][3]int{scriptID: functionsOf(c.pick(1, 1), c.add(1))}),
			take(takeEnd, map[string][][][3]int{scriptID: functionsOf(c.pick(1, 0), c.add(1))}),
		)
		owned := ownedIn(t, threeTimes(recording), nil)
		if slices.Contains(owned, lines(5)[0]) || !slices.Contains(owned, lines(4)[0]) {
			t.Fatalf("owned %v", owned)
		}
	})
	t.Run("a callback of foreign origin cut out of the window owns nothing, the window's own work around it still does"+verifiesOwnership, func(t *testing.T) {
		recording := recordingOf(
			take(takeStart, map[string][][][3]int{scriptID: functionsOf(c.module(1))}),
			take(takeOpen, nil),
			take(takeSliceStart, map[string][][][3]int{scriptID: functionsOf(c.add(1))}),
			take(takeSliceEnd, map[string][][][3]int{scriptID: functionsOf(c.tick(1))}),
			take(takeClose, map[string][][][3]int{scriptID: functionsOf(c.add(1))}),
		)
		owned := ownedIn(t, threeTimes(recording), nil)
		if !slices.Equal(owned, lines(1, 2)) {
			t.Fatalf("owned %v", owned)
		}
	})
	t.Run("a line that also ran inside a slice of foreign origin is not owned"+verifiesOwnership, func(t *testing.T) {
		recording := recordingOf(
			take(takeStart, map[string][][][3]int{scriptID: functionsOf(c.module(1))}),
			take(takeOpen, nil),
			take(takeSliceStart, map[string][][][3]int{scriptID: functionsOf(c.add(1))}),
			take(takeSliceEnd, map[string][][][3]int{scriptID: functionsOf(c.add(1))}),
			take(takeClose, nil),
		)
		if owned := ownedIn(t, threeTimes(recording), nil); len(owned) != 0 {
			t.Fatalf("owned %v", owned)
		}
	})
	t.Run("a window in which a module is evaluated owns nothing"+verifiesOwnership, func(t *testing.T) {
		recording := recordingOf(
			take(takeStart, map[string][][][3]int{scriptID: functionsOf(c.module(1))}),
			take(takeOpen, nil),
			take(takeClose, map[string][][][3]int{scriptID: functionsOf(c.module(1), c.add(1))}),
		)
		if owned := ownedIn(t, threeTimes(recording), nil); len(owned) != 0 {
			t.Fatalf("owned %v", owned)
		}
	})
	t.Run("a window in which a module first counts owns nothing"+verifiesOwnership, func(t *testing.T) {
		recording := recordingOf(
			take(takeStart, map[string][][][3]int{scriptID: functionsOf(c.module(1))}),
			take(takeOpen, nil),
			take(takeClose, map[string][][][3]int{scriptID: functionsOf(c.add(1)), lazyScriptID: functionsOf([][3]int{{9, 28, 1}})}),
		)
		if owned := ownedIn(t, threeTimes(recording), nil); len(owned) != 0 {
			t.Fatalf("owned %v", owned)
		}
	})
	t.Run("a line whose window counts differ between recordings is not owned"+verifiesOwnership, func(t *testing.T) {
		other := recordingOf(
			take(takeStart, map[string][][][3]int{scriptID: functionsOf(c.module(1))}),
			take(takeOpen, nil),
			take(takeClose, map[string][][][3]int{scriptID: functionsOf(c.add(3))}),
		)
		if owned := ownedIn(t, []processRecord{plainRecording(c), plainRecording(c), other}, nil); len(owned) != 0 {
			t.Fatalf("owned %v", owned)
		}
	})
	t.Run("fewer than three recordings own nothing"+verifiesOwnership, func(t *testing.T) {
		if owned := ownedIn(t, threeTimes(plainRecording(c))[:2], nil); len(owned) != 0 {
			t.Fatalf("owned %v", owned)
		}
	})
	t.Run("a window that does not hold the settling of the test function owns nothing"+verifiesOwnership, func(t *testing.T) {
		recording := plainRecording(c)
		recording.takes[2].bounded = false
		if owned := ownedIn(t, threeTimes(recording), nil); len(owned) != 0 {
			t.Fatalf("owned %v", owned)
		}
	})
	t.Run("takes that count from the start of the process own what takes that reset own"+verifiesSegments, func(t *testing.T) {
		recording := recordingOf(
			take(takeStart, map[string][][][3]int{scriptID: functionsOf(c.module(1), c.add(0), c.pick(0, 0), c.tick(0))}),
			take(takeOpen, map[string][][][3]int{scriptID: functionsOf(c.module(1), c.add(0), c.pick(0, 0), c.tick(0))}),
			take(takeClose, map[string][][][3]int{scriptID: functionsOf(c.module(1), c.add(2), c.pick(0, 0), c.tick(0))}),
			take(takeEnd, map[string][][][3]int{scriptID: functionsOf(c.module(1), c.add(3), c.pick(1, 0), c.tick(0))}),
		)
		recording.cumulative = true
		owned := ownedIn(t, threeTimes(recording), nil)
		if !slices.Equal(owned, lines(1, 2)) {
			t.Fatalf("owned %v", owned)
		}
	})
}

func TestOwnershipFallbacks(t *testing.T) {
	c := calcRanges{t: t}
	for _, missing := range []struct {
		name   string
		remove func(*capabilities)
	}{
		{"no inspector session", func(c *capabilities) { c.Inspector = false }},
		{"function counts that may vanish between takes", func(c *capabilities) { c.Retention = false }},
		{"a take that may not show a module evaluated in the window", func(c *capabilities) { c.Evaluation = false }},
	} {
		t.Run("a process with "+missing.name+" owns nothing"+verifiesFallbacks, func(t *testing.T) {
			recording := plainRecording(c)
			missing.remove(&recording.capabilities)
			if owned := ownedIn(t, threeTimes(recording), nil); len(owned) != 0 {
				t.Fatalf("owned %v", owned)
			}
		})
	}
	t.Run("a window whose realm faked timers it could not enumerate owns nothing"+verifiesFallbacks, func(t *testing.T) {
		recording := plainRecording(c)
		recording.takes[2].fakeUnseen = true
		if owned := ownedIn(t, threeTimes(recording), nil); len(owned) != 0 {
			t.Fatalf("owned %v", owned)
		}
	})
	t.Run("where takes are not synchronous, a window in which a callback of foreign origin ran owns nothing"+verifiesFallbacks, func(t *testing.T) {
		recording := plainRecording(c)
		recording.capabilities.SynchronousTake, recording.capabilities.AsyncHooks = false, false
		recording.takes[2].callbacks = true
		if owned := ownedIn(t, threeTimes(recording), nil); len(owned) != 0 {
			t.Fatalf("owned %v", owned)
		}
		recording.takes[2].callbacks = false
		if owned := ownedIn(t, threeTimes(recording), nil); !slices.Equal(owned, lines(1, 2)) {
			t.Fatalf("owned %v", owned)
		}
	})
	blind := func(addInWindow int) processRecord {
		recording := recordingOf(
			take(takeStart, map[string][][][3]int{scriptID: functionsOf(c.module(1))}),
			take(takeOpen, nil),
			take(takeClose, map[string][][][3]int{scriptID: functionsOf(c.add(addInWindow))}),
		)
		recording.capabilities.AsyncHooks = false
		return recording
	}
	t.Run("where the async hooks are blind, a line is owned when it ran more often in the window than in the hollow runs"+verifiesFallbacks, func(t *testing.T) {
		if owned := ownedIn(t, threeTimes(blind(2)), threeTimes(blind(1))); !slices.Equal(owned, lines(1, 2)) {
			t.Fatalf("owned %v", owned)
		}
		if owned := ownedIn(t, threeTimes(blind(2)), threeTimes(blind(2))); len(owned) != 0 {
			t.Fatalf("owned %v against hollow runs that ran it as often", owned)
		}
	})
	t.Run("hollow runs that disagree leave the rule to the windows' callbacks: one with a callback owns nothing"+verifiesFallbacks, func(t *testing.T) {
		disagreeing := []processRecord{blind(1), blind(1), blind(2)}
		if owned := ownedIn(t, threeTimes(blind(2)), disagreeing); !slices.Equal(owned, lines(1, 2)) {
			t.Fatalf("owned %v", owned)
		}
		withCallback := blind(2)
		withCallback.takes[2].callbacks = true
		if owned := ownedIn(t, threeTimes(withCallback), disagreeing); len(owned) != 0 {
			t.Fatalf("owned %v", owned)
		}
	})
}

func TestSegmentsOfARecordingOfSeveralFiles(t *testing.T) {
	c := calcRanges{t: t}
	recording := recordingOf(
		take(takeStart, nil),
		takeRecord{at: takeFile, file: "test/a.test.js", counts: map[string][][][3]int{scriptID: functionsOf(c.tick(1))}},
		takeRecord{at: takeFile, file: "test/b.test.js", counts: map[string][][][3]int{scriptID: functionsOf(c.add(1))}},
		take(takeEnd, map[string][][][3]int{scriptID: functionsOf(c.pick(1, 0))}),
	)
	shapes := newGeometries(identityFiles(t, map[string]string{calcFile: calcSource}))
	view := analyzeProcess(recording, shapes)
	t.Run("a test file's segment runs from its start to the next file's, the start-up included"+verifiesSegments, func(t *testing.T) {
		all := func(vacuity.Line) bool { return true }
		a, b := sortedLines(view.segmentOf("test/a.test.js"), all), sortedLines(view.segmentOf("test/b.test.js"), all)
		if !slices.Equal(a, lines(1, 2, 7, 8)) || !slices.Equal(b, lines(4, 5, 7, 8)) {
			t.Fatalf("a %v, b %v", a, b)
		}
	})
}
