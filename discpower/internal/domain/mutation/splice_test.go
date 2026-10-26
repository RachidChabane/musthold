package mutation_test

import (
	"errors"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation"
)

const (
	sampleFile   = "pkg/calc.py"
	sampleSource = "def add(a, b):\n    return a + b\n\ndef sub(a, b):\n    return a - b\n\ndef neg(a):\n    return -a\n"
	defPrefix    = "def "
	defNameEnd   = "("
	blockBreak   = "\n\n"
)

// lineLocator is a fake locator: a definition starts at a line beginning with
// "def " and runs to the end of its blank-line separated block.
type lineLocator struct{ failOn string }

func (l lineLocator) Locate(_ string, source []byte) ([]ecosystem.Definition, error) {
	text := string(source)
	if l.failOn != "" && strings.Contains(text, l.failOn) {
		return nil, errors.New("syntax error")
	}
	var definitions []ecosystem.Definition
	offset := 0
	for _, block := range strings.SplitAfter(text, blockBreak) {
		trimmed := strings.TrimRight(block, "\n")
		if strings.HasPrefix(trimmed, defPrefix) {
			name := trimmed[len(defPrefix):strings.Index(trimmed, defNameEnd)]
			definitions = append(definitions, ecosystem.Definition{
				Qualname: name, Kind: ecosystem.KindFunction,
				Span:      ecosystem.Span{Start: offset, End: offset + len(trimmed)},
				StartLine: strings.Count(text[:offset], "\n") + 1,
				EndLine:   strings.Count(text[:offset+len(trimmed)], "\n") + 1,
			})
		}
		offset += len(block)
	}
	return definitions, nil
}

// lineAssignments is a fake assignment locator: an assignment is a line that
// starts with an upper-case name followed by " = ".
type lineAssignments struct{}

func (lineAssignments) LocateAssignments(_ string, source []byte) ([]ecosystem.Definition, error) {
	var found []ecosystem.Definition
	offset := 0
	for _, line := range strings.SplitAfter(string(source), "\n") {
		trimmed := strings.TrimRight(line, "\n")
		if name, _, ok := strings.Cut(trimmed, " = "); ok && name == strings.ToUpper(name) && !strings.Contains(name, " ") {
			found = append(found, ecosystem.Definition{
				Qualname: name, Kind: ecosystem.KindAssignment,
				Span: ecosystem.Span{Start: offset, End: offset + len(trimmed)},
			})
		}
		offset += len(line)
	}
	return found, nil
}

// outOfBounds reports a definition whose span exceeds the source.
type outOfBounds struct{}

func (outOfBounds) Locate(_ string, source []byte) ([]ecosystem.Definition, error) {
	return []ecosystem.Definition{{Qualname: "f", Kind: ecosystem.KindFunction, Span: ecosystem.Span{Start: 0, End: len(source) + 1}}}, nil
}

const assignmentSource = "RATE = 1\nGRACE = 5\n\ndef cost(a):\n    return a * RATE\n"

func TestSpliceTargetKinds(t *testing.T) {
	splicer := mutation.Splicer{Locator: lineLocator{failOn: "BROKEN"}, Assignments: lineAssignments{}}

	t.Run("a module assignment is replaced and every other unit is kept [verifies SP-84~1]", func(t *testing.T) {
		edit := mutation.Edit{File: sampleFile, Qualname: "GRACE", Target: mutation.TargetModuleAssignment, NewSource: "GRACE = 6"}
		got, err := splicer.Splice(sampleFile, []byte(assignmentSource), edit)
		if err != nil || string(got) != strings.Replace(assignmentSource, "GRACE = 5", "GRACE = 6", 1) {
			t.Fatalf("got %q, %v", got, err)
		}
	})

	t.Run("a module assignment and a definition with one name are distinct units [verifies SP-84~1]", func(t *testing.T) {
		source := "cost = 1\n\ndef cost(a):\n    return a\n"
		edit := mutation.Edit{File: sampleFile, Qualname: "cost", NewSource: "def cost(a):\n    return 0"}
		if _, err := (mutation.Splicer{Locator: lineLocator{}, Assignments: lineAssignments{}}).Splice(sampleFile, []byte(source), edit); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("an unknown assignment is refused [verifies SP-84~1]", func(t *testing.T) {
		edit := mutation.Edit{File: sampleFile, Qualname: "MISSING", Target: mutation.TargetModuleAssignment, NewSource: "MISSING = 1"}
		if _, err := splicer.Splice(sampleFile, []byte(assignmentSource), edit); !errors.Is(err, mutation.ErrUnknownDefinition) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("a module assignment needs an assignment locator [verifies SP-84~1]", func(t *testing.T) {
		edit := mutation.Edit{File: sampleFile, Qualname: "GRACE", Target: mutation.TargetModuleAssignment, NewSource: "GRACE = 6"}
		_, err := mutation.Splicer{Locator: lineLocator{}}.Splice(sampleFile, []byte(assignmentSource), edit)
		if !errors.Is(err, mutation.ErrUnsupportedTarget) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("a module assignment whose new source declares another name breaks the invariant [verifies SP-84~1]", func(t *testing.T) {
		edit := mutation.Edit{File: sampleFile, Qualname: "GRACE", Target: mutation.TargetModuleAssignment, NewSource: "GRACE = 6\nOTHER = 1"}
		if _, err := splicer.Splice(sampleFile, []byte(assignmentSource), edit); !errors.Is(err, mutation.ErrInvariantViolated) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("a module addition is appended on its own line [verifies SP-84~1]", func(t *testing.T) {
		edit := mutation.Edit{File: sampleFile, Qualname: "_UNUSED", Target: mutation.TargetModuleAddition, NewSource: "_UNUSED = 0"}
		got, err := splicer.Splice(sampleFile, []byte(assignmentSource), edit)
		if err != nil || string(got) != assignmentSource+"\n_UNUSED = 0\n" {
			t.Fatalf("got %q, %v", got, err)
		}
	})

	t.Run("a module addition of a declared name is refused [verifies SP-84~1]", func(t *testing.T) {
		for _, name := range []string{"RATE", "cost"} {
			edit := mutation.Edit{File: sampleFile, Qualname: name, Target: mutation.TargetModuleAddition, NewSource: name + " = 0"}
			if _, err := splicer.Splice(sampleFile, []byte(assignmentSource), edit); !errors.Is(err, mutation.ErrNameTaken) {
				t.Errorf("%s: err = %v", name, err)
			}
		}
	})

	t.Run("a span outside the source is refused [verifies SP-84~1]", func(t *testing.T) {
		edit := mutation.Edit{File: sampleFile, Qualname: "f", NewSource: "def f(): pass"}
		_, err := mutation.Splicer{Locator: outOfBounds{}}.Splice(sampleFile, []byte(sampleSource), edit)
		if !errors.Is(err, ecosystem.ErrInvalidSpan) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("splicing an assignment's own source leaves the file unchanged [verifies SP-84~1]", func(t *testing.T) {
		edit := mutation.Edit{File: sampleFile, Qualname: "RATE", Target: mutation.TargetModuleAssignment, NewSource: "RATE = 1"}
		got, err := splicer.Splice(sampleFile, []byte(assignmentSource), edit)
		if err != nil || string(got) != assignmentSource {
			t.Fatalf("got %q, %v", got, err)
		}
	})
}

func TestSplice(t *testing.T) {
	splicer := mutation.Splicer{Locator: lineLocator{failOn: "BROKEN"}}

	t.Run("a definition is replaced and every other one is kept [verifies SP-60~1]", func(t *testing.T) {
		edit := mutation.Edit{File: sampleFile, Qualname: "sub", NewSource: "def sub(a, b):\n    return a + b"}
		got, err := splicer.Splice(sampleFile, []byte(sampleSource), edit)
		if err != nil {
			t.Fatal(err)
		}
		want := strings.Replace(sampleSource, "return a - b", "return a + b", 1)
		if string(got) != want {
			t.Fatalf("spliced = %q, want %q", got, want)
		}
	})

	t.Run("an unknown qualname is refused [verifies SP-60~1]", func(t *testing.T) {
		_, err := splicer.Splice(sampleFile, []byte(sampleSource), mutation.Edit{Qualname: "mul", NewSource: "def mul():\n    pass"})
		if !errors.Is(err, mutation.ErrUnknownDefinition) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("a repeated qualname is ambiguous [verifies SP-60~1]", func(t *testing.T) {
		source := sampleSource + "\ndef neg(a):\n    return a\n"
		_, err := splicer.Splice(sampleFile, []byte(source), mutation.Edit{Qualname: "neg", NewSource: "def neg(a):\n    return 0"})
		if !errors.Is(err, mutation.ErrAmbiguousDefinition) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("a splice that no longer parses is a reparse error [verifies SP-61~1]", func(t *testing.T) {
		_, err := splicer.Splice(sampleFile, []byte(sampleSource), mutation.Edit{Qualname: "sub", NewSource: "def sub(a, b):\n    BROKEN"})
		if !errors.Is(err, mutation.ErrReparse) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("a new source that defines another function breaks the invariant [verifies SP-61~1]", func(t *testing.T) {
		edit := mutation.Edit{Qualname: "sub", NewSource: "def sub(a, b):\n    return a\n\ndef extra():\n    pass"}
		_, err := splicer.Splice(sampleFile, []byte(sampleSource), edit)
		if !errors.Is(err, mutation.ErrInvariantViolated) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("a new source that renames the target breaks the invariant [verifies SP-61~1]", func(t *testing.T) {
		edit := mutation.Edit{Qualname: "sub", NewSource: "def other(a, b):\n    return a"}
		_, err := splicer.Splice(sampleFile, []byte(sampleSource), edit)
		if !errors.Is(err, mutation.ErrInvariantViolated) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("edits of one file apply in order [verifies SP-60~1]", func(t *testing.T) {
		edits := []mutation.Edit{
			{Qualname: "add", NewSource: "def add(a, b):\n    return 0"},
			{Qualname: "neg", NewSource: "def neg(a):\n    return a"},
		}
		got, err := splicer.SpliceAll(sampleFile, []byte(sampleSource), edits)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(got), "return 0") || !strings.Contains(string(got), "return a\n") {
			t.Fatalf("spliced = %q", got)
		}
	})
}

func TestSplicingADefinitionsOwnSourceIsTheIdentity(t *testing.T) {
	splicer := mutation.Splicer{Locator: lineLocator{}}
	t.Run("splicing a definition's own source leaves the file unchanged [verifies SP-63~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			names := rapid.SliceOfNDistinct(rapid.StringMatching(`[a-z]{1,6}`), 1, 5, rapid.ID[string]).Draw(t, "names")
			blocks := make([]string, 0, len(names))
			for _, name := range names {
				body := rapid.StringMatching(`    return [a-z0-9]{1,6}`).Draw(t, "body")
				blocks = append(blocks, defPrefix+name+"(x):\n"+body)
			}
			source := strings.Join(blocks, blockBreak) + "\n"
			index := rapid.IntRange(0, len(names)-1).Draw(t, "index")
			got, err := splicer.Splice(sampleFile, []byte(source), mutation.Edit{Qualname: names[index], NewSource: blocks[index]})
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != source {
				t.Fatalf("identity splice changed the source: %q != %q", got, source)
			}
		})
	})
}

func TestPolicy(t *testing.T) {
	policy := mutation.Policy{
		SourceRoots: []string{"src", "lib/core"},
		Layout:      ecosystem.Layout{TestFileGlobs: []string{"test_*.py", "*_test.go"}},
	}
	cases := []struct {
		file    string
		refused bool
	}{
		{"src/app.py", false},
		{"lib/core/deep/app.py", false},
		{"src/test_app.py", true},
		{"src/app_test.go", true},
		{"tests/app.py", true},
		{"lib/corelib/app.py", true},
		{"../src/app.py", true},
		{"/src/app.py", true},
		{"src/../tests/app.py", true},
		{`src\app.py`, true},
		{"", true},
	}
	for _, c := range cases {
		t.Run(c.file+" [verifies SP-62~1]", func(t *testing.T) {
			err := policy.Check(c.file)
			if c.refused != errors.Is(err, mutation.ErrPathRefused) {
				t.Fatalf("Check(%q) = %v, refused want %v", c.file, err, c.refused)
			}
			if c.refused && !mutation.IsRefusal(err) {
				t.Fatal("IsRefusal is false for a refusal")
			}
		})
	}

	t.Run("the whole project is a source root [verifies SP-62~1]", func(t *testing.T) {
		whole := mutation.Policy{SourceRoots: []string{"."}, Layout: policy.Layout}
		if err := whole.Check("anywhere/app.py"); err != nil {
			t.Fatal(err)
		}
	})
}

func TestEditSet(t *testing.T) {
	set := mutation.EditSet{Edits: []mutation.Edit{{File: "a.py", Qualname: "f"}, {File: "b.py", Qualname: "g"}, {File: "a.py", Qualname: "h"}}}
	t.Run("files are distinct and in first-seen order [verifies SP-60~1]", func(t *testing.T) {
		if got := set.Files(); len(got) != 2 || got[0] != "a.py" || got[1] != "b.py" {
			t.Fatalf("files = %v", got)
		}
		if got := set.EditsOf("a.py"); len(got) != 2 || got[1].Qualname != "h" {
			t.Fatalf("edits = %v", got)
		}
	})
}
