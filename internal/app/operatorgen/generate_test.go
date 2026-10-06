package operatorgen

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/internal/domain/mutation"
	"github.com/RachidChabane/spec-proof/internal/domain/mutation/footprint"
	"github.com/RachidChabane/spec-proof/internal/domain/mutation/operator"
)

// fakeEngine proposes, for each definition `fN` of a file, three relational
// candidates, and one sentinel of each kind per file.
type fakeEngine struct {
	extension string
	functions []string
}

func (e fakeEngine) Extensions() []string        { return []string{e.extension} }
func (e fakeEngine) Families() []operator.Family { return []operator.Family{operator.FamilyRelational} }

func (e fakeEngine) Candidates(file mutation.SourceFile) ([]operator.Candidate, error) {
	var out []operator.Candidate
	for _, name := range e.functions {
		for variant := range 3 {
			out = append(out, operator.Candidate{
				Site:     operator.Site{File: file.Path, Qualname: name, Target: mutation.TargetDefinition, Span: ecosystem.Span{Start: variant, End: variant + 1}},
				Family:   operator.FamilyRelational,
				Original: "def " + name + "(): pass",
				Edit:     mutation.Edit{File: file.Path, Qualname: name, Target: mutation.TargetDefinition, NewSource: fmt.Sprintf("def %s(): v%d", name, variant)},
			})
		}
	}
	out = append(out, operator.Candidate{
		Site:     operator.Site{File: file.Path, Qualname: "BROKEN", Target: mutation.TargetDefinition},
		Family:   operator.FamilyRelational,
		Original: "def BROKEN(): pass",
		Edit:     mutation.Edit{File: file.Path, Qualname: "BROKEN", Target: mutation.TargetDefinition, NewSource: "def BROKEN(: pass"},
	})
	return out, nil
}

func (e fakeEngine) SentinelCandidates(file mutation.SourceFile, _ operator.Seed) ([]operator.SentinelCandidate, error) {
	var out []operator.SentinelCandidate
	for _, kind := range operator.SentinelKinds() {
		for _, name := range e.functions {
			out = append(out, operator.SentinelCandidate{
				Kind:     kind,
				Site:     operator.Site{File: file.Path, Qualname: name, Target: mutation.TargetDefinition},
				Original: "def " + name + "(): pass",
				Edit:     mutation.Edit{File: file.Path, Qualname: name, Target: mutation.TargetDefinition, NewSource: fmt.Sprintf("def %s(): %s", name, kind)},
			})
		}
	}
	return out, nil
}

func (e fakeEngine) CallGraph(file mutation.SourceFile) (footprint.FileGraph, error) {
	graph := footprint.FileGraph{File: file.Path}
	for i, name := range e.functions {
		node := footprint.Node{Qualname: name, Name: name}
		if i+1 < len(e.functions) && name == "f0" {
			node.Calls = []footprint.Reference{{Name: e.functions[i+1], Kind: footprint.ReferencePlain}}
		}
		graph.Nodes = append(graph.Nodes, node)
	}
	return graph, nil
}

func (fakeEngine) Normalize(_ string, _ mutation.TargetKind, source string) (string, error) {
	if strings.Count(source, "(") != strings.Count(source, ")") {
		return "", errors.New("unbalanced")
	}
	return source, nil
}

func (fakeEngine) LocateAssignments(string, []byte) ([]ecosystem.Definition, error) { return nil, nil }

const defPrefix = "def "

// fakeLocator finds a definition on each line that starts with "def ".
type fakeLocator struct{}

func (fakeLocator) Locate(_ string, source []byte) ([]ecosystem.Definition, error) {
	var found []ecosystem.Definition
	offset := 0
	for _, line := range strings.SplitAfter(string(source), "\n") {
		text := strings.TrimRight(line, "\n")
		if strings.HasPrefix(text, defPrefix) {
			name, _, _ := strings.Cut(text[len(defPrefix):], "(")
			found = append(found, ecosystem.Definition{Qualname: name, Kind: ecosystem.KindFunction, Span: ecosystem.Span{Start: offset, End: offset + len(text)}})
		}
		offset += len(line)
	}
	return found, nil
}

// ghostEngine proposes an edit of a definition the file does not have.
type ghostEngine struct{ fakeEngine }

func (e ghostEngine) Candidates(file mutation.SourceFile) ([]operator.Candidate, error) {
	return []operator.Candidate{{
		Site:     operator.Site{File: file.Path, Qualname: "ghost", Target: mutation.TargetDefinition},
		Family:   operator.FamilyRelational,
		Original: "def ghost(): pass",
		Edit:     mutation.Edit{File: file.Path, Qualname: "ghost", Target: mutation.TargetDefinition, NewSource: "def ghost(): v"},
	}}, nil
}

func newGenerator(t *testing.T, engines ...Engine) *Generator {
	t.Helper()
	registry := NewRegistry()
	for _, engine := range engines {
		if err := registry.Register(engine, fakeLocator{}); err != nil {
			t.Fatal(err)
		}
	}
	return NewGenerator(registry)
}

const sourceOfFakeFunctions = "def f0(): pass\ndef f1(): pass\ndef f2(): pass\n"

func files(paths ...string) []mutation.SourceFile {
	var out []mutation.SourceFile
	for _, path := range paths {
		out = append(out, mutation.SourceFile{Path: path, Content: []byte(sourceOfFakeFunctions)})
	}
	return out
}

func mutantIDs(mutants []operator.Mutant) []string {
	ids := make([]string, len(mutants))
	for i, m := range mutants {
		ids[i] = m.ID
	}
	return ids
}

func TestRegistry(t *testing.T) {
	t.Run("an engine is chosen by the extension of the file [verifies SP-98~1]", func(t *testing.T) {
		python, golang := fakeEngine{extension: ".py"}, fakeEngine{extension: ".go"}
		registry := NewRegistry()
		for _, engine := range []Engine{python, golang} {
			if err := registry.Register(engine, fakeLocator{}); err != nil {
				t.Fatal(err)
			}
		}
		got, err := registry.For("pkg/mod.go")
		if err != nil || got.Extensions()[0] != ".go" {
			t.Fatalf("got %v, %v", got, err)
		}
		if !slices.Equal(registry.Extensions(), []string{".go", ".py"}) {
			t.Fatalf("extensions = %v", registry.Extensions())
		}
	})

	t.Run("a file with no engine and a taken extension are refused [verifies SP-98~1]", func(t *testing.T) {
		registry := NewRegistry()
		if err := registry.Register(fakeEngine{extension: ".py"}, fakeLocator{}); err != nil {
			t.Fatal(err)
		}
		if _, err := registry.For("a.rs"); !errors.Is(err, ErrNoEngine) {
			t.Fatalf("err = %v", err)
		}
		if err := registry.Register(fakeEngine{extension: ".py"}, fakeLocator{}); !errors.Is(err, ErrExtensionTaken) {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestGenerate(t *testing.T) {
	engine := fakeEngine{extension: ".py", functions: []string{"f0", "f1", "f2"}}
	config := operator.Config{KOp: 5, Seed: 9}

	t.Run("the operator mutants are capped and sentinels do not count against the cap [verifies SP-99~1]", func(t *testing.T) {
		result, err := newGenerator(t, engine).Generate(Request{Files: files("a.py", "b.py"), Config: config})
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Operator) != config.KOp {
			t.Fatalf("operator mutants = %d", len(result.Operator))
		}
		if len(result.Sentinels) != 2*operator.SentinelsPerFile {
			t.Fatalf("sentinels = %d", len(result.Sentinels))
		}
		for _, m := range result.Sentinels {
			if !m.IsSentinel() {
				t.Fatalf("%+v is not a sentinel", m)
			}
		}
		if result.Stats.Enumerated != 2*(3*3+1) || result.Stats.Screen.Stillborn != 2 || result.Stats.Screened != 18 {
			t.Fatalf("stats = %+v", result.Stats)
		}
	})

	t.Run("a footprint restricts mutants and sentinels to its definitions and files [verifies SP-99~1]", func(t *testing.T) {
		result, err := newGenerator(t, engine).Generate(Request{
			Files:   files("a.py", "b.py"),
			Entries: []footprint.Ref{{File: "a.py", Qualname: "f1"}},
			Config:  operator.Config{KOp: 100, Seed: 1},
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range slices.Concat(result.Operator, result.Sentinels) {
			if m.Site.File != "a.py" || m.Site.Qualname != "f1" {
				t.Fatalf("out of the footprint: %+v", m.Site)
			}
		}
		if len(result.Operator) != 3 || len(result.Sentinels) != operator.SentinelsPerFile {
			t.Fatalf("operator %d sentinels %d", len(result.Operator), len(result.Sentinels))
		}
	})

	t.Run("a footprint follows calls to other definitions [verifies SP-99~1]", func(t *testing.T) {
		result, err := newGenerator(t, engine).Generate(Request{
			Files:   files("a.py"),
			Entries: []footprint.Ref{{File: "a.py", Qualname: "f0"}},
			Config:  operator.Config{KOp: 100, Seed: 1},
		})
		if err != nil {
			t.Fatal(err)
		}
		qualnames := map[string]bool{}
		for _, m := range result.Operator {
			qualnames[m.Site.Qualname] = true
		}
		if len(qualnames) != 2 || !qualnames["f0"] || !qualnames["f1"] {
			t.Fatalf("qualnames = %v", qualnames)
		}
	})

	t.Run("a run is a pure function of the files and the seed [verifies SP-99~1]", func(t *testing.T) {
		run := func(seed operator.Seed) Result {
			result, err := newGenerator(t, engine).Generate(Request{Files: files("b.py", "a.py"), Config: operator.Config{KOp: 5, Seed: seed}})
			if err != nil {
				t.Fatal(err)
			}
			return result
		}
		first, again, other := run(1), run(1), run(2)
		if !slices.Equal(mutantIDs(first.Operator), mutantIDs(again.Operator)) || !slices.Equal(mutantIDs(first.Sentinels), mutantIDs(again.Sentinels)) {
			t.Fatal("a repeated run differs")
		}
		if slices.Equal(mutantIDs(first.Operator), mutantIDs(other.Operator)) && slices.Equal(mutantIDs(first.Sentinels), mutantIDs(other.Sentinels)) {
			t.Fatal("another seed gave the same run")
		}
	})

	t.Run("a file with no engine fails the run [verifies SP-99~1]", func(t *testing.T) {
		_, err := newGenerator(t, engine).Generate(Request{Files: files("a.rs"), Config: config})
		if !errors.Is(err, ErrNoEngine) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("every generated mutant is spliced into its file, and one that does not splice fails the run [verifies SP-100~1]", func(t *testing.T) {
		if _, err := newGenerator(t, engine).Generate(Request{Files: files("a.py"), Config: config}); err != nil {
			t.Fatal(err)
		}
		_, err := newGenerator(t, ghostEngine{engine}).Generate(Request{Files: files("a.py"), Config: config})
		if !errors.Is(err, ErrUnsplicable) || !errors.Is(err, mutation.ErrUnknownDefinition) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("an engine needs a definition locator [verifies SP-98~1]", func(t *testing.T) {
		if err := NewRegistry().Register(engine, nil); !errors.Is(err, ErrNoLocator) {
			t.Fatalf("err = %v", err)
		}
	})
}
