package golang

import (
	"encoding/json"
	"errors"
	"flag"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/discpower/internal/adapters/sourceparse/goparser"
	"github.com/RachidChabane/musthold/discpower/internal/app/operatorgen"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation/footprint"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation/operator"
)

var update = flag.Bool("update", false, "rewrite the golden files")

const (
	goldenSourcePath = "testdata/parkfee/fees.go"
	goldenFilePath   = "testdata/parkfee/golden.json"
	goldenName       = "parkfee/fees.go"
	header           = "package p\n\n"
)

// splice applies an edit with the production splicer, over the real definition
// locator of the language and this engine as the assignment locator.
func splice(source []byte, edit mutation.Edit) ([]byte, error) {
	splicer := mutation.Splicer{Locator: goparser.NewLocator(), Assignments: New()}
	return splicer.Splice(edit.File, source, edit)
}

func file(source string) mutation.SourceFile {
	return mutation.SourceFile{Path: "p/m.go", Content: []byte(header + source)}
}

func candidatesOf(t testing.TB, source string) []operator.Candidate {
	t.Helper()
	got, err := New().Candidates(file(source))
	if err != nil {
		t.Fatalf("Candidates: %v", err)
	}
	return got
}

func sourcesOf(candidates []operator.Candidate, family operator.Family) []string {
	var out []string
	for _, c := range candidates {
		if c.Family == family {
			out = append(out, c.Edit.NewSource)
		}
	}
	slices.Sort(out)
	return out
}

func sorted(items ...string) []string {
	slices.Sort(items)
	return items
}

func TestOperatorFamilies(t *testing.T) {
	tests := []struct {
		name   string
		source string
		family operator.Family
		want   []string
	}{
		{"relational", "func f(a int) bool {\n\treturn a < 1\n}\n", operator.FamilyRelational,
			sorted("func f(a int) bool {\n\treturn a <= 1\n}", "func f(a int) bool {\n\treturn a > 1\n}", "func f(a int) bool {\n\treturn a >= 1\n}", "func f(a int) bool {\n\treturn a == 1\n}", "func f(a int) bool {\n\treturn a != 1\n}")},
		{"equality on nil keeps equality operators", "func f(e error) bool {\n\treturn e == nil\n}\n", operator.FamilyRelational,
			sorted("func f(e error) bool {\n\treturn e != nil\n}")},
		{"arithmetic", "func f(a, b int) int {\n\treturn a + b\n}\n", operator.FamilyArithmetic,
			sorted("func f(a, b int) int {\n\treturn a - b\n}", "func f(a, b int) int {\n\treturn a * b\n}", "func f(a, b int) int {\n\treturn a / b\n}")},
		{"string concatenation is not an arithmetic site", "func f(a string) string {\n\treturn a + \"x\"\n}\n", operator.FamilyArithmetic, nil},
		{"numeric constant", "func f(a int) int {\n\treturn a + 15\n}\n", operator.FamilyNumericConstant,
			sorted("func f(a int) int {\n\treturn a + 16\n}", "func f(a int) int {\n\treturn a + 14\n}", "func f(a int) int {\n\treturn a + 150\n}")},
		{"boolean logical", "func f(a, b bool) int {\n\tif a && b {\n\t\treturn 1\n\t}\n\treturn 2\n}\n", operator.FamilyBooleanLogical,
			sorted(
				"func f(a, b bool) int {\n\tif !(a && b) {\n\t\treturn 1\n\t}\n\treturn 2\n}",
				"func f(a, b bool) int {\n\tif !(a) && b {\n\t\treturn 1\n\t}\n\treturn 2\n}",
				"func f(a, b bool) int {\n\tif a && !(b) {\n\t\treturn 1\n\t}\n\treturn 2\n}")},
		{"for condition negation", "func f(a int) {\n\tfor a < 3 {\n\t\ta++\n\t}\n}\n", operator.FamilyBooleanLogical,
			sorted("func f(a int) {\n\tfor !(a < 3) {\n\t\ta++\n\t}\n}")},
		{"return value zeroing", "func f(a int) (int, string) {\n\treturn a, \"x\"\n}\n", operator.FamilyReturnValue,
			sorted("func f(a int) (int, string) {\n\treturn 0, \"x\"\n}", "func f(a int) (int, string) {\n\treturn a, \"\"\n}")},
		{"module constant", "const rate = 15\n", operator.FamilyModuleConstant, sorted("rate = 16", "rate = 14", "rate = 150")},
		{"constructor argument call", "func f() time.Duration {\n\treturn time.Duration(15)\n}\n", operator.FamilyConstructorArgument,
			sorted("func f() time.Duration {\n\treturn time.Duration(16)\n}", "func f() time.Duration {\n\treturn time.Duration(14)\n}", "func f() time.Duration {\n\treturn time.Duration(150)\n}")},
		{"constructor argument composite", "var p = Point{X: 1}\n", operator.FamilyConstructorArgument, sorted("p = Point{X: 2}", "p = Point{X: 0}", "p = Point{X: 10}")},
		{"decimal string", "var price = decimal.RequireFromString(\"0.15\")\n", operator.FamilyDecimalString,
			sorted("price = decimal.RequireFromString(\"0.16\")", "price = decimal.RequireFromString(\"0.14\")", "price = decimal.RequireFromString(\"1.5\")")},
		{"min max", "func f(a int) int {\n\treturn min(a, max(a, 0))\n}\n", operator.FamilyMinMax,
			sorted("func f(a int) int {\n\treturn max(a, max(a, 0))\n}", "func f(a int) int {\n\treturn min(a, min(a, 0))\n}")},
		{"and or", "func f(a, b bool) bool {\n\treturn a || b\n}\n", operator.FamilyAndOr, sorted("func f(a, b bool) bool {\n\treturn a && b\n}")},
		{"boolean literal", "func f() bool {\n\treturn true\n}\n", operator.FamilyBooleanLiteral, sorted("func f() bool {\n\treturn false\n}")},
		{"error sentinel swap", "var errA = errors.New(\"a\")\nvar errB = errors.New(\"b\")\n\nfunc f() error {\n\treturn errA\n}\n", operator.FamilyExceptionClass,
			sorted("func f() error {\n\treturn errB\n}")},
	}
	for _, tt := range tests {
		t.Run(tt.name+" [verifies SP-95~1]", func(t *testing.T) {
			got := sourcesOf(candidatesOf(t, tt.source), tt.family)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("got\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

func TestUnits(t *testing.T) {
	t.Run("qualnames follow pkg.Func, (*T).Method and T.Method [verifies SP-95~1]", func(t *testing.T) {
		source := "func Free(a int) bool { return a < 1 }\nfunc (t *Tariff) Ptr(a int) bool { return a < 2 }\nfunc (t Tariff) Val(a int) bool { return a < 3 }\nfunc (t *Box[K]) Gen(a int) bool { return a < 4 }\n"
		qualnames := map[string]bool{}
		for _, c := range candidatesOf(t, source) {
			qualnames[c.Edit.Qualname] = true
			if c.Edit.Target != mutation.TargetDefinition {
				t.Fatalf("target %q", c.Edit.Target)
			}
		}
		want := []string{"p.Free", "(*Tariff).Ptr", "Tariff.Val", "(*Box).Gen"}
		for _, qualname := range want {
			if !qualnames[qualname] {
				t.Errorf("missing %s in %v", qualname, qualnames)
			}
		}
	})

	t.Run("only single-name specs with an explicit value are module units [verifies SP-95~1]", func(t *testing.T) {
		source := "const (\n\tA = iota\n\tB\n\tC = 3\n)\nvar D, E = 1, 2\nvar F int = 5\nvar _ = 6\nvar G = 7\n"
		got := map[string]bool{}
		for _, c := range candidatesOf(t, source) {
			if c.Edit.Target == mutation.TargetModuleAssignment {
				got[c.Edit.Qualname] = true
			}
		}
		if len(got) != 3 || !got["p.C"] || !got["p.F"] || !got["p.G"] {
			t.Fatalf("units = %v", got)
		}
	})

	t.Run("the site span covers the mutated node in the original file [verifies SP-95~1]", func(t *testing.T) {
		source := "func f(a int) bool { return a < 10 }\n"
		content := file(source).Content
		for _, c := range candidatesOf(t, source) {
			if c.Family == operator.FamilyRelational && string(content[c.Site.Span.Start:c.Site.Span.End]) != "<" {
				t.Fatalf("site text %q", content[c.Site.Span.Start:c.Site.Span.End])
			}
			if string(content[c.Replaces.Start:c.Replaces.End]) != c.Original {
				t.Fatalf("replaced span %q is not the original %q", content[c.Replaces.Start:c.Replaces.End], c.Original)
			}
		}
	})

	t.Run("a source with a syntax error is refused [verifies SP-95~1]", func(t *testing.T) {
		if _, err := New().Candidates(file("func f( {\n")); !errors.Is(err, ErrSyntax) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("the families the engine declares are the families it produces on the golden package [verifies SP-95~1]", func(t *testing.T) {
		if len(New().Families()) != len(operator.Families()) {
			t.Fatal("families missing")
		}
	})
}

func TestNormalize(t *testing.T) {
	engine := New()
	normalize := func(target mutation.TargetKind, source string) string {
		t.Helper()
		got, err := engine.Normalize("m.go", target, source)
		if err != nil {
			t.Fatalf("Normalize: %v", err)
		}
		return got
	}

	t.Run("comments and layout are ignored [verifies SP-97~1]", func(t *testing.T) {
		plain := normalize(mutation.TargetDefinition, "func f(a int) int {\n\treturn a + 1\n}")
		decorated := normalize(mutation.TargetDefinition, "func f( a int ) int {\n\t// why\n\treturn a+1 /* x */\n}")
		if plain != decorated {
			t.Fatalf("%q != %q", plain, decorated)
		}
	})

	t.Run("a change of meaning survives normalization [verifies SP-97~1]", func(t *testing.T) {
		original := normalize(mutation.TargetDefinition, "func f(a int) int {\n\treturn a + 1\n}")
		for _, changed := range []string{"func f(a int) int {\n\treturn a - 1\n}", "func g(a int) int {\n\treturn a + 1\n}"} {
			if normalize(mutation.TargetDefinition, changed) == original {
				t.Errorf("%q normalizes to the original", changed)
			}
		}
	})

	t.Run("specs and additions normalize as units [verifies SP-97~1]", func(t *testing.T) {
		if normalize(mutation.TargetModuleAssignment, "rate   =  15") != normalize(mutation.TargetModuleAssignment, "rate = 15") {
			t.Fatal("layout changed a spec")
		}
		if normalize(mutation.TargetModuleAddition, "const spUnused = 0") == "" {
			t.Fatal("empty normalization")
		}
	})

	t.Run("a unit that does not parse is refused [verifies SP-97~1]", func(t *testing.T) {
		if _, err := engine.Normalize("m.go", mutation.TargetDefinition, "func f( {"); !errors.Is(err, ErrSyntax) {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestCallGraph(t *testing.T) {
	content, err := os.ReadFile(goldenSourcePath)
	if err != nil {
		t.Fatal(err)
	}
	graph, err := New().CallGraph(mutation.SourceFile{Path: goldenName, Content: content})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("nodes carry owner, name and references [verifies SP-97~1]", func(t *testing.T) {
		nodes := map[string]footprint.Node{}
		for _, n := range graph.Nodes {
			nodes[n.Qualname] = n
		}
		for _, qualname := range []string{"parkfee.NewTariff", "(*Tariff).Breakdown", "(*Tariff).AtNight", "parkfee.validatedFree", "parkfee.Charge"} {
			if _, ok := nodes[qualname]; !ok {
				t.Fatalf("node %s missing", qualname)
			}
		}
		breakdown := nodes["(*Tariff).Breakdown"]
		if breakdown.Owner != "Tariff" || breakdown.Name != "Breakdown" ||
			!slices.Contains(breakdown.Calls, footprint.Reference{Name: "Mul", Kind: footprint.ReferenceMember}) ||
			!slices.Contains(breakdown.Calls, footprint.Reference{Name: "validatedFree", Kind: footprint.ReferencePlain}) {
			t.Fatalf("breakdown = %+v", breakdown)
		}
		if !slices.Contains(nodes["parkfee.NewTariff"].Reads, "hourlyRate") {
			t.Fatalf("reads = %v", nodes["parkfee.NewTariff"].Reads)
		}
	})

	t.Run("package-level names are listed with their qualnames [verifies SP-97~1]", func(t *testing.T) {
		got := map[string]string{}
		for _, a := range graph.Assignments {
			got[a.Name] = a.Qualname
		}
		if got["dailyCap"] != "parkfee.dailyCap" || got["errOverstay"] != "parkfee.errOverstay" || len(got) != 7 {
			t.Fatalf("assignments = %v", got)
		}
	})

	t.Run("a helper reachable only through a call stays in the footprint [verifies SP-97~1]", func(t *testing.T) {
		closed := footprint.Close([]footprint.FileGraph{graph}, []footprint.Ref{{File: goldenName, Qualname: "parkfee.Charge"}})
		for _, qualname := range []string{"parkfee.Charge", "parkfee.NewTariff", "(*Tariff).Breakdown", "parkfee.validatedFree"} {
			if !closed.ContainsDefinition(footprint.Ref{File: goldenName, Qualname: qualname}) {
				t.Errorf("%s missing from %v", qualname, closed.Definitions)
			}
		}
		if closed.ContainsDefinition(footprint.Ref{File: goldenName, Qualname: "(*Tariff).AtNight"}) || closed.ContainsAssignment(footprint.Ref{File: goldenName, Qualname: "parkfee.nightStart"}) {
			t.Fatal("an unreached member is in the footprint")
		}
		if !closed.ContainsAssignment(footprint.Ref{File: goldenName, Qualname: "parkfee.validationWindow"}) {
			t.Fatal("a referenced package constant is missing")
		}
	})
}

// injectedName matches the identifiers sentinels inject: a word and a suffix in
// lower camel case.
var injectedName = regexp.MustCompile(`\b(?:anchor|backlog|buffer|carry|cursor|margin|offset|pending|pivot|probe|scratch|slot|spare|stash|staging|window)[A-Z0-9][a-z0-9]\b`)

const placeholder = "N"

// sentinelSources lists the new sources of a kind, the injected identifiers
// replaced by a placeholder.
func sentinelSources(t testing.TB, source string, kind operator.SentinelKind) []string {
	t.Helper()
	candidates, err := New().SentinelCandidates(file(source), operator.LocalSeed)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, c := range candidates {
		if c.Kind == kind {
			out = append(out, injectedName.ReplaceAllString(c.Edit.NewSource, placeholder))
		}
	}
	return out
}

func TestSentinels(t *testing.T) {
	t.Run("a dead assignment is offered before each statement with a use that keeps it compiling [verifies SP-96~1]", func(t *testing.T) {
		got := sentinelSources(t, "func f(a int) int {\n\tb := a + 1\n\treturn b\n}\n", operator.SentinelDeadAssignment)
		want := []string{
			"func f(a int) int {\n\tN := 0\n\t_ = N\n\tb := a + 1\n\treturn b\n}",
			"func f(a int) int {\n\tb := a + 1\n\tN := 0\n\t_ = N\n\treturn b\n}",
		}
		if !slices.Equal(got, want) {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("a function with a goto or a label only offers the first position [verifies SP-96~1]", func(t *testing.T) {
		got := sentinelSources(t, "func f(a int) int {\n\tif a > 0 {\n\t\tgoto done\n\t}\n\ta++\ndone:\n\treturn a\n}\n", operator.SentinelDeadAssignment)
		if len(got) != 1 || !strings.HasPrefix(got[0], "func f(a int) int {\n\tN := 0\n\t_ = N\n\tif a > 0 {") {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("a one-line body and an empty body get no dead assignment [verifies SP-96~1]", func(t *testing.T) {
		if got := sentinelSources(t, "func f() int { return 1 }\nfunc g() {\n}\n", operator.SentinelDeadAssignment); len(got) != 0 {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("a local is renamed through every use, parameters and fields are not [verifies SP-96~1]", func(t *testing.T) {
		source := "func f(rate int, p Point) int {\n\ttotal := p.total + rate\n\tfn := func() int { return total }\n\treturn fn()\n}\n"
		got := sentinelSources(t, source, operator.SentinelAlphaRename)
		want := []string{
			"func f(rate int, p Point) int {\n\ttotal := p.total + rate\n\tN := func() int { return total }\n\treturn N()\n}",
			"func f(rate int, p Point) int {\n\tN := p.total + rate\n\tfn := func() int { return N }\n\treturn fn()\n}",
		}
		slices.Sort(got)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Fatalf("got %q\nwant %q", got, want)
		}
	})

	t.Run("a name used as a composite literal key is not renamed [verifies SP-96~1]", func(t *testing.T) {
		if got := sentinelSources(t, "func f() Point {\n\tx := 1\n\treturn Point{x: 2}\n}\n", operator.SentinelAlphaRename); len(got) != 0 {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("a function that uses reflect, runtime or unsafe is skipped [verifies SP-96~1]", func(t *testing.T) {
		for _, pkg := range introspectionPackages {
			source := "func f(a int) int {\n\tv := " + pkg + ".Probe(a)\n\treturn v\n}\n"
			for _, kind := range []operator.SentinelKind{operator.SentinelDeadAssignment, operator.SentinelAlphaRename} {
				if got := sentinelSources(t, source, kind); len(got) != 0 {
					t.Errorf("%s: %s sentinel proposed: %q", pkg, kind, got)
				}
			}
		}
	})

	t.Run("the unused constant is a private package addition with a fresh name [verifies SP-96~1]", func(t *testing.T) {
		candidates, err := New().SentinelCandidates(file("const spUnused = 1\n"), operator.LocalSeed)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range candidates {
			if c.Kind == operator.SentinelUnusedConstant {
				name := strings.TrimPrefix(c.Edit.Qualname, "p.")
				if c.Edit.Target != mutation.TargetModuleAddition || c.Edit.NewSource != "const "+name+" = 0" || name == "spUnused" || !injectedName.MatchString(name) {
					t.Fatalf("constant = %+v", c)
				}
				return
			}
		}
		t.Fatal("no unused constant candidate")
	})
}

func TestSentinelsVary(t *testing.T) {
	t.Run("injected identifiers and positions change with the seed and carry no fixed marker [verifies SP-89~1]", func(t *testing.T) {
		content, err := os.ReadFile(goldenSourcePath)
		if err != nil {
			t.Fatal(err)
		}
		names := map[string]bool{}
		positions := map[ecosystem.Span]bool{}
		for seed := operator.Seed(1); seed <= 40; seed++ {
			candidates, err := New().SentinelCandidates(mutation.SourceFile{Path: goldenName, Content: content}, seed)
			if err != nil {
				t.Fatal(err)
			}
			for _, c := range candidates {
				for _, marker := range []string{"spDead", "spRenamed", "spUnused"} {
					if strings.Contains(c.Edit.NewSource, marker) {
						t.Fatalf("%s sentinel carries the marker %q", c.Kind, marker)
					}
				}
				for _, name := range injectedName.FindAllString(c.Edit.NewSource, -1) {
					names[name] = true
				}
				if c.Kind == operator.SentinelDeadAssignment {
					positions[c.Site.Span] = true
				}
			}
		}
		if len(names) < 20 || len(positions) < 4 {
			t.Fatalf("%d names and %d positions over 40 seeds", len(names), len(positions))
		}
	})
}

// goldenRecord is what the golden file pins.
type goldenRecord struct {
	Enumerated      int                 `json:"enumerated"`
	Strata          int                 `json:"strata"`
	Sampled         []goldenMutant      `json:"sampled"`
	Sentinels       []goldenSentinel    `json:"sentinels"`
	Footprint       footprintSummary    `json:"footprint"`
	SampledByFamily map[string]int      `json:"sampledByFamily"`
	ByFamily        map[string]int      `json:"enumeratedByFamily"`
	Unrestricted    map[string]int      `json:"unrestrictedByFamily"`
	Seeds           map[string][]string `json:"sentinelsBySeed"`
}

type goldenMutant struct {
	ID       string `json:"id"`
	Family   string `json:"family"`
	Qualname string `json:"qualname"`
}

type goldenSentinel struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Qualname string `json:"qualname"`
}

type footprintSummary struct {
	Definitions []string `json:"definitions"`
	Assignments []string `json:"assignments"`
}

func generateGolden(t *testing.T, config operator.Config, entries []footprint.Ref) operatorgen.Result {
	t.Helper()
	content, err := os.ReadFile(goldenSourcePath)
	if err != nil {
		t.Fatal(err)
	}
	registry := operatorgen.NewRegistry()
	if err := registry.Register(New(), goparser.NewLocator()); err != nil {
		t.Fatal(err)
	}
	result, err := operatorgen.NewGenerator(registry).Generate(operatorgen.Request{
		Files:   []mutation.SourceFile{{Path: goldenName, Content: content}},
		Entries: entries,
		Config:  config,
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func familyCounts(counts map[operator.Family]int) map[string]int {
	out := map[string]int{}
	for family, count := range counts {
		out[string(family)] = count
	}
	return out
}

func TestGolden(t *testing.T) {
	entries := []footprint.Ref{{File: goldenName, Qualname: "parkfee.Quote"}, {File: goldenName, Qualname: "(*Tariff).AtNight"}}
	full := generateGolden(t, operator.Config{KOp: 1000, Seed: operator.LocalSeed}, entries)
	capped := generateGolden(t, operator.Config{KOp: 25, Seed: operator.LocalSeed}, entries)

	record := goldenRecord{
		Enumerated:      full.Stats.Enumerated,
		ByFamily:        familyCounts(full.Stats.EnumeratedByFamily),
		Strata:          full.Stats.Strata,
		SampledByFamily: familyCounts(capped.Stats.SampledByFamily),
		Unrestricted:    familyCounts(generateGolden(t, operator.Config{KOp: 1000, Seed: operator.LocalSeed}, nil).Stats.EnumeratedByFamily),
		Seeds:           map[string][]string{},
	}
	for _, m := range capped.Operator {
		record.Sampled = append(record.Sampled, goldenMutant{ID: m.ID, Family: string(m.Family), Qualname: m.Site.Qualname})
	}
	for _, m := range capped.Sentinels {
		record.Sentinels = append(record.Sentinels, goldenSentinel{ID: m.ID, Kind: string(m.Sentinel), Qualname: m.Site.Qualname})
	}
	for _, d := range full.Footprint.Definitions {
		record.Footprint.Definitions = append(record.Footprint.Definitions, d.Qualname)
	}
	for _, a := range full.Footprint.Assignments {
		record.Footprint.Assignments = append(record.Footprint.Assignments, a.Qualname)
	}
	for _, seed := range []operator.Seed{1, 2, 3} {
		for _, m := range generateGolden(t, operator.Config{KOp: 25, Seed: seed}, entries).Sentinels {
			key := string(rune('0' + seed))
			record.Seeds[key] = append(record.Seeds[key], string(m.Sentinel)+":"+m.Site.Qualname)
		}
	}

	encoded, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	encoded = append(encoded, '\n')
	if *update {
		if err := os.WriteFile(goldenFilePath, encoded, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(goldenFilePath)
	if err != nil {
		t.Fatalf("read golden (run with -update): %v", err)
	}

	t.Run("the generation of the parkfee package matches the golden file [verifies SP-95~1]", func(t *testing.T) {
		if string(encoded) != string(want) {
			t.Fatalf("golden mismatch; run with -update to review the change\n%s", encoded)
		}
	})

	t.Run("every family the engine declares has a hit in the package [verifies SP-95~1]", func(t *testing.T) {
		for _, family := range New().Families() {
			if record.Unrestricted[string(family)] == 0 {
				t.Errorf("no %s mutant", family)
			}
		}
	})

	t.Run("the sample respects the cap and the sentinels stay outside it [verifies SP-99~1]", func(t *testing.T) {
		if len(capped.Operator) != 25 || len(capped.Sentinels) != operator.SentinelsPerFile {
			t.Fatalf("operator %d sentinels %d", len(capped.Operator), len(capped.Sentinels))
		}
	})
}

// goFunction draws a small function whose body exercises the operators.
func goFunction(t *rapid.T) string {
	number := rapid.SampledFrom([]string{"0", "1", "7", "15", "100"})
	name := rapid.SampledFrom([]string{"a", "b", "c"})
	var boolean, integer func(depth int) *rapid.Generator[string]
	integer = func(depth int) *rapid.Generator[string] {
		leaf := rapid.OneOf(number, name)
		if depth == 0 {
			return leaf
		}
		sub := integer(depth - 1)
		return rapid.OneOf(
			leaf,
			rapid.Custom(func(t *rapid.T) string {
				op := rapid.SampledFrom([]string{"+", "-", "*", "/"}).Draw(t, "op")
				return "(" + sub.Draw(t, "left") + " " + op + " " + sub.Draw(t, "right") + ")"
			}),
			rapid.Custom(func(t *rapid.T) string {
				fn := rapid.SampledFrom([]string{"min", "max"}).Draw(t, "fn")
				return fn + "(" + sub.Draw(t, "x") + ", " + sub.Draw(t, "y") + ")"
			}),
		)
	}
	boolean = func(depth int) *rapid.Generator[string] {
		ints := integer(1)
		compare := rapid.Custom(func(t *rapid.T) string {
			op := rapid.SampledFrom([]string{"<", "<=", ">", ">=", "==", "!="}).Draw(t, "cmp")
			return "(" + ints.Draw(t, "l") + " " + op + " " + ints.Draw(t, "r") + ")"
		})
		if depth == 0 {
			return rapid.OneOf(compare, rapid.SampledFrom([]string{"true", "false"}))
		}
		sub := boolean(depth - 1)
		return rapid.OneOf(
			compare,
			rapid.Custom(func(t *rapid.T) string {
				op := rapid.SampledFrom([]string{"&&", "||"}).Draw(t, "logic")
				return "(" + sub.Draw(t, "l") + " " + op + " " + sub.Draw(t, "r") + ")"
			}),
		)
	}
	conditions, values := boolean(2), integer(2)
	var body strings.Builder
	body.WriteString("func f(a, b, c int) int {\n")
	for range rapid.IntRange(0, 3).Draw(t, "statements") {
		switch rapid.IntRange(0, 2).Draw(t, "kind") {
		case 0:
			body.WriteString("\tif " + conditions.Draw(t, "cond") + " {\n\t\treturn " + values.Draw(t, "value") + "\n\t}\n")
		case 1:
			body.WriteString("\tlocal := " + values.Draw(t, "assigned") + "\n\t_ = local\n")
		default:
			body.WriteString("\tfor " + conditions.Draw(t, "loop") + " {\n\t\tbreak\n\t}\n")
		}
	}
	body.WriteString("\treturn " + values.Draw(t, "result") + "\n}\n")
	return "const rate = " + number.Draw(t, "rate") + "\n\n" + body.String()
}

func TestProperties(t *testing.T) {
	engine := New()
	parses := func(t *rapid.T, source string) {
		t.Helper()
		if _, err := parseSource("p/m.go", []byte(source)); err != nil {
			t.Fatalf("does not parse: %v\n%s", err, source)
		}
	}

	t.Run("every operator mutant re-parses, differs from the original and splices into a parsable file [verifies SP-100~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			source := goFunction(t)
			candidates, err := engine.Candidates(file(source))
			if err != nil {
				t.Fatalf("source %q: %v", source, err)
			}
			for _, c := range candidates {
				if c.Edit.NewSource == c.Original {
					t.Fatalf("mutant equals the original: %q", c.Edit.NewSource)
				}
				spliced, err := splice(file(source).Content, c.Edit)
				if err != nil {
					t.Fatal(err)
				}
				parses(t, string(spliced))
			}
		})
	})

	t.Run("the screen keeps only mutants that normalize and differ [verifies SP-100~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			candidates, err := engine.Candidates(file(goFunction(t)))
			if err != nil {
				t.Fatal(err)
			}
			kept, _, err := operator.Screen(candidates, engine)
			if err != nil {
				t.Fatal(err)
			}
			for _, m := range kept {
				edit := m.EditSet.Edits[0]
				mutated, err := engine.Normalize("m.go", edit.Target, edit.NewSource)
				original, _ := engine.Normalize("m.go", edit.Target, m.Original)
				if err != nil || mutated == original {
					t.Fatalf("kept mutant %q: %v", edit.NewSource, err)
				}
			}
		})
	})

	t.Run("sentinels parse in a parsable file [verifies SP-100~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			source := goFunction(t)
			sentinels, err := engine.SentinelCandidates(file(source), operator.LocalSeed)
			if err != nil || len(sentinels) == 0 {
				t.Fatalf("sentinels %d, %v", len(sentinels), err)
			}
			for _, s := range sentinels {
				spliced, err := splice(file(source).Content, s.Edit)
				if err != nil {
					t.Fatal(err)
				}
				parses(t, string(spliced))
			}
		})
	})

	t.Run("enumeration and sampling are deterministic for a fixed seed [verifies SP-100~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			source := goFunction(t)
			seed := operator.Seed(rapid.Uint64().Draw(t, "seed"))
			run := func() ([]string, []string) {
				candidates, _ := engine.Candidates(file(source))
				kept, _, _ := operator.Screen(candidates, engine)
				sampled := operator.Sample(kept, 7, seed)
				sentinelCandidates, _ := engine.SentinelCandidates(file(source), seed)
				drawn, _ := operator.DrawSentinels("m.go", sentinelCandidates, seed)
				var a, b []string
				for _, m := range sampled {
					a = append(a, m.ID)
				}
				for _, m := range drawn {
					b = append(b, m.ID)
				}
				return a, b
			}
			a1, b1 := run()
			a2, b2 := run()
			if !slices.Equal(a1, a2) || !slices.Equal(b1, b2) || len(a1) > 7 {
				t.Fatal("two runs with the same seed differ or exceed the cap")
			}
		})
	})
}

func TestFixtureSplicesAgreeWithTheSpans(t *testing.T) {
	content, err := os.ReadFile(goldenSourcePath)
	if err != nil {
		t.Fatal(err)
	}
	source := mutation.SourceFile{Path: goldenName, Content: content}
	candidates, err := New().Candidates(source)
	if err != nil {
		t.Fatal(err)
	}
	sentinels, err := New().SentinelCandidates(source, operator.LocalSeed)
	if err != nil {
		t.Fatal(err)
	}
	type site struct {
		edit     mutation.Edit
		replaces ecosystem.Span
	}
	var sites []site
	for _, c := range candidates {
		sites = append(sites, site{c.Edit, c.Replaces})
	}
	for _, s := range sentinels {
		sites = append(sites, site{s.Edit, s.Replaces})
	}
	t.Run("the production splice equals the replacement of the reported span [verifies SP-100~1]", func(t *testing.T) {
		for _, s := range sites {
			got, err := splice(content, s.edit)
			if err != nil {
				t.Fatalf("%s %s: %v", s.edit.Target, s.edit.Qualname, err)
			}
			want := string(content[:s.replaces.Start]) + s.edit.NewSource + string(content[s.replaces.End:])
			if s.edit.Target == mutation.TargetModuleAddition {
				want = string(content) + "\n" + s.edit.NewSource + "\n"
			}
			if string(got) != want {
				t.Fatalf("%s %s: splice differs from the span replacement", s.edit.Target, s.edit.Qualname)
			}
		}
	})
}
