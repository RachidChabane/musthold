package python

import (
	"encoding/json"
	"errors"
	"flag"
	"os"
	"slices"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/discpower/internal/adapters/sourceparse/treesitter"
	"github.com/RachidChabane/musthold/discpower/internal/app/operatorgen"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation/footprint"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation/operator"
)

var update = flag.Bool("update", false, "rewrite the golden files")

const (
	goldenSourcePath = "testdata/parkfee/fees.py"
	goldenFilePath   = "testdata/parkfee/golden.json"
	goldenName       = "parkfee/fees.py"
)

// splice applies an edit with the production splicer, over the real definition
// locator of the language and this engine as the assignment locator.
func splice(source []byte, edit mutation.Edit) ([]byte, error) {
	splicer := mutation.Splicer{Locator: treesitter.NewLocator(), Assignments: New()}
	return splicer.Splice(edit.File, source, edit)
}

func file(source string) mutation.SourceFile {
	return mutation.SourceFile{Path: "m.py", Content: []byte(source)}
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
	return out
}

func TestOperatorFamilies(t *testing.T) {
	tests := []struct {
		name   string
		source string
		family operator.Family
		want   []string
	}{
		{"relational", "def f(a):\n    return a < 1\n", operator.FamilyRelational,
			[]string{"def f(a):\n    return a <= 1", "def f(a):\n    return a > 1", "def f(a):\n    return a >= 1", "def f(a):\n    return a == 1", "def f(a):\n    return a != 1"}},
		{"chained relational", "def f(a, b):\n    return a < b < 3\n", operator.FamilyRelational, nil},
		{"arithmetic", "def f(a, b):\n    return a + b\n", operator.FamilyArithmetic,
			[]string{"def f(a, b):\n    return a - b", "def f(a, b):\n    return a * b", "def f(a, b):\n    return a / b", "def f(a, b):\n    return a // b"}},
		{"numeric constant", "def f(a):\n    return a + 15\n", operator.FamilyNumericConstant,
			[]string{"def f(a):\n    return a + 16", "def f(a):\n    return a + 14", "def f(a):\n    return a + 150"}},
		{"float constant", "def f(a):\n    return a * 0.5\n", operator.FamilyNumericConstant,
			[]string{"def f(a):\n    return a * 1.5", "def f(a):\n    return a * (-0.5)", "def f(a):\n    return a * 5.0"}},
		{"boolean logical", "def f(a, b):\n    if a and b:\n        return 1\n    return 2\n", operator.FamilyBooleanLogical,
			[]string{
				"def f(a, b):\n    if not (a and b):\n        return 1\n    return 2",
				"def f(a, b):\n    if not (a) and b:\n        return 1\n    return 2",
				"def f(a, b):\n    if a and not (b):\n        return 1\n    return 2",
			}},
		{"ternary and while negation", "def f(a):\n    while a:\n        a = 0 if a else 1\n", operator.FamilyBooleanLogical,
			[]string{
				"def f(a):\n    while not (a):\n        a = 0 if a else 1",
				"def f(a):\n    while a:\n        a = 0 if not (a) else 1",
			}},
		{"return deletion and return None", "def f(a):\n    if a:\n        return 1\n    return a\n", operator.FamilyReturnValue,
			[]string{
				"def f(a):\n    if a:\n        pass\n    return a",
				"def f(a):\n    if a:\n        return None\n    return a",
				"def f(a):\n    if a:\n        return 1\n    return None",
			}},
		{"module constant", "RATE = 15\n", operator.FamilyModuleConstant, []string{"RATE = 16", "RATE = 14", "RATE = 150"}},
		{"constructor argument keyword", "GRACE = timedelta(minutes=15)\n", operator.FamilyConstructorArgument,
			[]string{"GRACE = timedelta(minutes=16)", "GRACE = timedelta(minutes=14)", "GRACE = timedelta(minutes=150)"}},
		{"constructor argument positional in a function", "def f():\n    return time(22, 0)\n", operator.FamilyConstructorArgument,
			[]string{"def f():\n    return time(23, 0)", "def f():\n    return time(21, 0)", "def f():\n    return time(220, 0)", "def f():\n    return time(22, 1)", "def f():\n    return time(22, (-1))"}},
		{"constructor class call", "def f():\n    return Money(5)\n", operator.FamilyConstructorArgument,
			[]string{"def f():\n    return Money(6)", "def f():\n    return Money(4)", "def f():\n    return Money(50)"}},
		{"decimal string", "PRICE = Decimal(\"0.15\")\n", operator.FamilyDecimalString,
			[]string{"PRICE = Decimal(\"0.16\")", "PRICE = Decimal(\"0.14\")", "PRICE = Decimal(\"1.5\")"}},
		{"min max", "def f(a):\n    return min(a, max(a, 0))\n", operator.FamilyMinMax,
			[]string{"def f(a):\n    return max(a, max(a, 0))", "def f(a):\n    return min(a, min(a, 0))"}},
		{"and or", "def f(a, b):\n    return a or b\n", operator.FamilyAndOr, []string{"def f(a, b):\n    return a and b"}},
		{"boolean literal", "def f(a=True):\n    return False\n", operator.FamilyBooleanLiteral,
			[]string{"def f(a=False):\n    return False", "def f(a=True):\n    return True"}},
		{"exception class", "def f(a):\n    try:\n        raise ValueError(a)\n    except KeyError:\n        raise CustomFailure\n    except PricingError as e:\n        pass\n", operator.FamilyExceptionClass,
			[]string{
				"def f(a):\n    try:\n        raise TypeError(a)\n    except KeyError:\n        raise CustomFailure\n    except PricingError as e:\n        pass",
				"def f(a):\n    try:\n        raise ValueError(a)\n    except IndexError:\n        raise CustomFailure\n    except PricingError as e:\n        pass",
				"def f(a):\n    try:\n        raise ValueError(a)\n    except KeyError:\n        raise CustomFailure\n    except RuntimeError as e:\n        pass",
			}},
	}
	for _, tt := range tests {
		t.Run(tt.name+" [verifies SP-91~1]", func(t *testing.T) {
			got := sourcesOf(candidatesOf(t, tt.source), tt.family)
			if tt.want == nil {
				if len(got) == 0 {
					t.Fatalf("no %s candidate", tt.family)
				}
				return
			}
			slices.Sort(got)
			want := slices.Clone(tt.want)
			slices.Sort(want)
			if !slices.Equal(got, want) {
				t.Fatalf("got\n%q\nwant\n%q", got, want)
			}
		})
	}
}

func TestUnits(t *testing.T) {
	t.Run("an edit targets the outermost definition and a method by its qualified name [verifies SP-91~1]", func(t *testing.T) {
		source := "class A:\n    def m(self):\n        def inner(x):\n            return x < 2\n        return inner(1)\n"
		got := candidatesOf(t, source)
		if len(got) == 0 {
			t.Fatal("no candidate")
		}
		for _, c := range got {
			if c.Edit.Qualname != "A.m" || c.Edit.Target != mutation.TargetDefinition || c.Replaces != (ecosystem.Span{Start: strings.Index(source, "def m"), End: len(source) - 1}) {
				t.Fatalf("candidate %+v", c)
			}
		}
	})

	t.Run("a module-level assignment is a unit and others are not [verifies SP-91~1]", func(t *testing.T) {
		source := "A = 1\nB: int = 2\nC, D = 3, 4\nif A:\n    E = 5\nclass K:\n    F = 6\n"
		qualnames := map[string]bool{}
		for _, c := range candidatesOf(t, source) {
			if c.Edit.Target == mutation.TargetModuleAssignment {
				qualnames[c.Edit.Qualname] = true
			}
		}
		if !qualnames["A"] || !qualnames["B"] || len(qualnames) != 2 {
			t.Fatalf("assignment units = %v", qualnames)
		}
	})

	t.Run("the site span covers the mutated node in the original file [verifies SP-91~1]", func(t *testing.T) {
		source := "def f(a):\n    return a < 10\n"
		for _, c := range candidatesOf(t, source) {
			text := source[c.Site.Span.Start:c.Site.Span.End]
			if c.Family == operator.FamilyRelational && text != "<" {
				t.Fatalf("site text %q", text)
			}
		}
	})

	t.Run("a source with a syntax error is refused [verifies SP-91~1]", func(t *testing.T) {
		if _, err := New().Candidates(file("def f(:\n")); !errors.Is(err, ErrSyntax) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("the engine declares every family [verifies SP-91~1]", func(t *testing.T) {
		if len(New().Families()) != len(operator.Families()) {
			t.Fatal("families missing")
		}
	})
}

func TestNormalize(t *testing.T) {
	engine := New()
	normalize := func(source string) string {
		t.Helper()
		got, err := engine.Normalize("m.py", mutation.TargetDefinition, source)
		if err != nil {
			t.Fatalf("Normalize: %v", err)
		}
		return got
	}

	t.Run("comments, docstrings and layout are ignored [verifies SP-93~1]", func(t *testing.T) {
		plain := normalize("def f(a):\n    return a+1\n")
		decorated := normalize("def f( a ):\n    \"\"\"Doc.\"\"\"\n    # why\n    return a + 1  # trailing\n")
		if plain != decorated {
			t.Fatalf("%q != %q", plain, decorated)
		}
	})

	t.Run("a change of meaning survives normalization [verifies SP-93~1]", func(t *testing.T) {
		for _, changed := range []string{"def f(a):\n    return a-1\n", "def f(a):\n    if a:\n        return a + 1\n", "def g(a):\n    return a + 1\n"} {
			if normalize(changed) == normalize("def f(a):\n    return a + 1\n") {
				t.Errorf("%q normalizes to the original", changed)
			}
		}
	})

	t.Run("a unit that does not parse is refused [verifies SP-93~1]", func(t *testing.T) {
		if _, err := engine.Normalize("m.py", mutation.TargetDefinition, "def f(:\n    pass"); !errors.Is(err, ErrSyntax) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("an indented method normalizes as a unit [verifies SP-93~1]", func(t *testing.T) {
		if _, err := engine.Normalize("m.py", mutation.TargetDefinition, "def m(self):\n        if self:\n            return 1\n        return 2"); err != nil {
			t.Fatal(err)
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

	t.Run("nodes carry owner, name, aliases and references [verifies SP-94~1]", func(t *testing.T) {
		nodes := map[string]footprint.Node{}
		for _, n := range graph.Nodes {
			nodes[n.Qualname] = n
		}
		if len(nodes) != 5 {
			t.Fatalf("nodes = %v", slices.Collect(func(yield func(string) bool) {
				for q := range nodes {
					yield(q)
				}
			}))
		}
		constructor := nodes["Tariff.__init__"]
		if constructor.Owner != "Tariff" || !slices.Equal(constructor.Aliases, []string{"Tariff"}) || !slices.Contains(constructor.Reads, "HOURLY_RATE") {
			t.Fatalf("constructor = %+v", constructor)
		}
		charge := nodes["charge"]
		if !slices.Contains(charge.Calls, footprint.Reference{Name: "breakdown", Kind: footprint.ReferenceMember}) ||
			!slices.Contains(charge.Calls, footprint.Reference{Name: "Tariff", Kind: footprint.ReferencePlain}) {
			t.Fatalf("charge calls = %+v", charge.Calls)
		}
	})

	t.Run("module-level assignments are listed with their names [verifies SP-94~1]", func(t *testing.T) {
		var names []string
		for _, a := range graph.Assignments {
			names = append(names, a.Name)
		}
		want := []string{"GRACE", "HOURLY_RATE", "DAILY_CAP", "NIGHT_START", "VALIDATION_WINDOW", "LABEL"}
		if !slices.Equal(names, want) {
			t.Fatalf("assignments = %v", names)
		}
	})

	t.Run("a helper reachable only through a call stays in the footprint [verifies SP-94~1]", func(t *testing.T) {
		closed := footprint.Close([]footprint.FileGraph{graph}, []footprint.Ref{{File: goldenName, Qualname: "charge"}})
		for _, qualname := range []string{"charge", "Tariff.__init__", "Tariff.breakdown", "_validated_free"} {
			if !closed.ContainsDefinition(footprint.Ref{File: goldenName, Qualname: qualname}) {
				t.Errorf("%s missing from %v", qualname, closed.Definitions)
			}
		}
		if closed.ContainsDefinition(footprint.Ref{File: goldenName, Qualname: "Tariff.at_night"}) || closed.ContainsAssignment(footprint.Ref{File: goldenName, Qualname: "NIGHT_START"}) {
			t.Fatal("an unreached member is in the footprint")
		}
		if !closed.ContainsAssignment(footprint.Ref{File: goldenName, Qualname: "VALIDATION_WINDOW"}) {
			t.Fatal("a referenced module constant is missing")
		}
	})
}

// goldenRecord is what the golden file pins.
type goldenRecord struct {
	Enumerated         int                 `json:"enumerated"`
	EnumeratedByFamily map[string]int      `json:"enumeratedByFamily"`
	Strata             int                 `json:"strata"`
	Sampled            []goldenMutant      `json:"sampled"`
	Sentinels          []goldenSentinel    `json:"sentinels"`
	Footprint          footprintSummary    `json:"footprint"`
	SampledByFamily    map[string]int      `json:"sampledByFamily"`
	Unrestricted       map[string]int      `json:"unrestrictedByFamily"`
	Seeds              map[string][]string `json:"sentinelsBySeed"`
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
	if err := registry.Register(New(), treesitter.NewLocator()); err != nil {
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
	entries := []footprint.Ref{{File: goldenName, Qualname: "charge"}}
	full := generateGolden(t, operator.Config{KOp: 1000, Seed: operator.LocalSeed}, entries)
	capped := generateGolden(t, operator.Config{KOp: 25, Seed: operator.LocalSeed}, entries)

	record := goldenRecord{
		Enumerated:         full.Stats.Enumerated,
		EnumeratedByFamily: familyCounts(full.Stats.EnumeratedByFamily),
		Strata:             full.Stats.Strata,
		SampledByFamily:    familyCounts(capped.Stats.SampledByFamily),
		Unrestricted:       familyCounts(generateGolden(t, operator.Config{KOp: 1000, Seed: operator.LocalSeed}, nil).Stats.EnumeratedByFamily),
		Seeds:              map[string][]string{},
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

	t.Run("the generation of the parkfee module matches the golden file [verifies SP-91~1]", func(t *testing.T) {
		if string(encoded) != string(want) {
			t.Fatalf("golden mismatch; run with -update to review the change\n%s", encoded)
		}
	})

	t.Run("every family the engine declares has a hit in the module [verifies SP-91~1]", func(t *testing.T) {
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
		for _, m := range capped.Sentinels {
			if !m.IsSentinel() {
				t.Fatalf("%+v", m)
			}
		}
	})

	t.Run("the footprint keeps the helper reached by a call and drops the rest [verifies SP-99~1]", func(t *testing.T) {
		for _, m := range full.Operator {
			if m.Site.Qualname == "Tariff.at_night" || m.Site.Qualname == "NIGHT_START" {
				t.Fatalf("mutant outside the footprint: %s", m.Site.Qualname)
			}
		}
		if !slices.Contains(record.Footprint.Definitions, "_validated_free") {
			t.Fatal("the helper is missing from the footprint")
		}
	})
}

// pythonFunction draws a small function whose body exercises the operators.
func pythonFunction(t *rapid.T) string {
	number := rapid.SampledFrom([]string{"0", "1", "7", "15", "0.5", "2.25", "100"})
	name := rapid.SampledFrom([]string{"a", "b", "c"})
	var expression func(depth int) *rapid.Generator[string]
	expression = func(depth int) *rapid.Generator[string] {
		leaf := rapid.OneOf(number, name, rapid.SampledFrom([]string{"True", "False", `Decimal("1.50")`, "timedelta(minutes=15)", "time(22, 0)"}))
		if depth == 0 {
			return leaf
		}
		sub := expression(depth - 1)
		return rapid.OneOf(
			leaf,
			rapid.Custom(func(t *rapid.T) string {
				op := rapid.SampledFrom([]string{"+", "-", "*", "/", "//", "<", "<=", ">", ">=", "==", "!=", "and", "or"}).Draw(t, "op")
				return "(" + sub.Draw(t, "left") + " " + op + " " + sub.Draw(t, "right") + ")"
			}),
			rapid.Custom(func(t *rapid.T) string {
				fn := rapid.SampledFrom([]string{"min", "max"}).Draw(t, "fn")
				return fn + "(" + sub.Draw(t, "x") + ", " + sub.Draw(t, "y") + ")"
			}),
			rapid.Custom(func(t *rapid.T) string { return "(not " + leaf.Draw(t, "operand") + ")" }),
		)
	}
	expr := expression(2)
	var body strings.Builder
	body.WriteString("def f(a, b, c):\n")
	for range rapid.IntRange(0, 3).Draw(t, "statements") {
		switch rapid.IntRange(0, 3).Draw(t, "kind") {
		case 0:
			body.WriteString("    if " + expr.Draw(t, "cond") + ":\n        return " + expr.Draw(t, "value") + "\n")
		case 1:
			body.WriteString("    local = " + expr.Draw(t, "assigned") + "\n")
		case 2:
			body.WriteString("    while " + expr.Draw(t, "loop") + ":\n        break\n")
		default:
			body.WriteString("    try:\n        raise ValueError(" + expr.Draw(t, "arg") + ")\n    except KeyError:\n        pass\n")
		}
	}
	body.WriteString("    return " + expr.Draw(t, "result") + "\n")
	return "RATE = " + number.Draw(t, "rate") + "\n\n" + body.String()
}

func TestProperties(t *testing.T) {
	engine := New()

	t.Run("every operator mutant re-parses, differs from the original and splices into a parsable file [verifies SP-100~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			source := pythonFunction(t)
			candidates, err := engine.Candidates(file(source))
			if err != nil {
				t.Fatalf("source %q: %v", source, err)
			}
			for _, c := range candidates {
				if c.Edit.NewSource == c.Original {
					t.Fatalf("mutant equals the original: %q", c.Edit.NewSource)
				}
				spliced, err := splice([]byte(source), c.Edit)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := engine.CallGraph(file(string(spliced))); err != nil {
					t.Fatalf("mutant gives an unparsable file: %v\n%s", err, spliced)
				}
			}
		})
	})

	t.Run("the screen keeps only mutants that parse and differ after normalization [verifies SP-100~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			source := pythonFunction(t)
			candidates, err := engine.Candidates(file(source))
			if err != nil {
				t.Fatal(err)
			}
			kept, _, err := operator.Screen(candidates, engine)
			if err != nil {
				t.Fatal(err)
			}
			for _, m := range kept {
				newSource := m.EditSet.Edits[0].NewSource
				mutated, err := engine.Normalize("m.py", m.EditSet.Edits[0].Target, newSource)
				if err != nil {
					t.Fatalf("kept mutant does not parse: %q", newSource)
				}
				original, _ := engine.Normalize("m.py", m.EditSet.Edits[0].Target, m.Original)
				if mutated == original {
					t.Fatalf("kept mutant equals the original: %q", newSource)
				}
				spliced, err := splice([]byte(source), m.EditSet.Edits[0])
				if err != nil {
					t.Fatal(err)
				}
				if _, err := engine.CallGraph(file(string(spliced))); err != nil {
					t.Fatalf("kept mutant gives an unparsable file: %v\n%s", err, spliced)
				}
			}
		})
	})

	t.Run("sentinels parse in a parsable file [verifies SP-100~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			source := pythonFunction(t)
			sentinels, err := engine.SentinelCandidates(file(source), operator.LocalSeed)
			if err != nil {
				t.Fatal(err)
			}
			if len(sentinels) == 0 {
				t.Fatal("no sentinel candidate")
			}
			for _, s := range sentinels {
				spliced, err := splice([]byte(source), s.Edit)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := engine.CallGraph(file(string(spliced))); err != nil {
					t.Fatalf("sentinel %s gives an unparsable file: %v\n%s", s.Kind, err, spliced)
				}
			}
		})
	})

	t.Run("enumeration and sampling are deterministic for a fixed seed [verifies SP-100~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			source := pythonFunction(t)
			seed := operator.Seed(rapid.Uint64().Draw(t, "seed"))
			run := func() ([]string, []string) {
				candidates, _ := engine.Candidates(file(source))
				kept, _, _ := operator.Screen(candidates, engine)
				sampled := operator.Sample(kept, 7, seed)
				sentinelCandidates, _ := engine.SentinelCandidates(file(source), seed)
				drawn, _ := operator.DrawSentinels("m.py", sentinelCandidates, seed)
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
			if !slices.Equal(a1, a2) || !slices.Equal(b1, b2) {
				t.Fatal("two runs with the same seed differ")
			}
			if len(a1) > 7 {
				t.Fatalf("sampled %d above the cap", len(a1))
			}
		})
	})
}

func TestFixtureSplicesAgreeWithTheSpans(t *testing.T) {
	content, err := os.ReadFile(goldenSourcePath)
	if err != nil {
		t.Fatal(err)
	}
	source := mutation.SourceFile{Path: "fees.py", Content: content}
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
