package javascript

import (
	"encoding/json"
	"flag"
	"os"
	"slices"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/adapters/sourceparse/treesitter"
	"github.com/RachidChabane/musthold/discpower/internal/app/operatorgen"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation/footprint"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation/operator"
)

var update = flag.Bool("update", false, "rewrite the golden files")

const (
	goldenKOp          = 1000
	goldenCappedKOp    = 25
	goldenSeedBase     = '0'
	goldenEntryName    = "charge"
	goldenUnreachedDef = "Tariff.atNight"
	goldenUnreachedVar = "NIGHT_START"
	goldenHelper       = "validatedFree"
)

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
	entries := []footprint.Ref{{File: goldenName, Qualname: goldenEntryName}}
	full := generateGolden(t, operator.Config{KOp: goldenKOp, Seed: operator.LocalSeed}, entries)
	capped := generateGolden(t, operator.Config{KOp: goldenCappedKOp, Seed: operator.LocalSeed}, entries)

	record := goldenRecord{
		Enumerated:         full.Stats.Enumerated,
		EnumeratedByFamily: familyCounts(full.Stats.EnumeratedByFamily),
		Strata:             full.Stats.Strata,
		SampledByFamily:    familyCounts(capped.Stats.SampledByFamily),
		Unrestricted:       familyCounts(generateGolden(t, operator.Config{KOp: goldenKOp, Seed: operator.LocalSeed}, nil).Stats.EnumeratedByFamily),
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
		for _, m := range generateGolden(t, operator.Config{KOp: goldenCappedKOp, Seed: seed}, entries).Sentinels {
			key := string(rune(goldenSeedBase + int(seed)))
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

	t.Run("the generation of the parkfee module matches the golden file [verifies SP-1000~1]", func(t *testing.T) {
		if string(encoded) != string(want) {
			t.Fatalf("golden mismatch; run with -update to review the change\n%s", encoded)
		}
	})

	t.Run("every family the engine declares has a hit in the module [verifies SP-1000~1]", func(t *testing.T) {
		for _, family := range New().Families() {
			if record.Unrestricted[string(family)] == 0 {
				t.Errorf("no %s mutant", family)
			}
		}
	})

	t.Run("every sentinel kind of the catalogue is offered for the module [verifies SP-1001~1]", func(t *testing.T) {
		content, err := os.ReadFile(goldenSourcePath)
		if err != nil {
			t.Fatal(err)
		}
		candidates, err := New().SentinelCandidates(mutation.SourceFile{Path: goldenName, Content: content}, operator.LocalSeed)
		if err != nil {
			t.Fatal(err)
		}
		offered := map[operator.SentinelKind]bool{}
		for _, c := range candidates {
			offered[c.Kind] = true
		}
		for _, kind := range operator.SentinelKinds() {
			if !offered[kind] {
				t.Errorf("no %s sentinel", kind)
			}
		}
	})

	t.Run("the sample respects the cap and the sentinels stay outside it [verifies SP-1000~1]", func(t *testing.T) {
		if len(capped.Operator) != goldenCappedKOp || len(capped.Sentinels) != operator.SentinelsPerFile {
			t.Fatalf("operator %d sentinels %d", len(capped.Operator), len(capped.Sentinels))
		}
		for _, m := range capped.Sentinels {
			if !m.IsSentinel() {
				t.Fatalf("%+v", m)
			}
		}
	})

	t.Run("the footprint keeps the helper reached by a call and drops the rest [verifies SP-1004~1]", func(t *testing.T) {
		for _, m := range full.Operator {
			if m.Site.Qualname == goldenUnreachedDef || m.Site.Qualname == goldenUnreachedVar {
				t.Fatalf("mutant outside the footprint: %s", m.Site.Qualname)
			}
		}
		if !slices.Contains(record.Footprint.Definitions, goldenHelper) {
			t.Fatal("the helper is missing from the footprint")
		}
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

	t.Run("the production splice of every candidate and sentinel equals the replacement of the reported span [verifies SP-1002~1]", func(t *testing.T) {
		if len(sites) == 0 {
			t.Fatal("nothing to splice")
		}
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

	t.Run("the assignment units equal the declarators the engine enumerates [verifies SP-1002~1]", func(t *testing.T) {
		located, err := New().LocateAssignments(goldenName, content)
		if err != nil {
			t.Fatal(err)
		}
		fromLocator := map[string]string{}
		for _, a := range located {
			fromLocator[a.Qualname] = string(content[a.Span.Start:a.Span.End])
		}
		for _, c := range candidates {
			if c.Edit.Target == mutation.TargetModuleAssignment && fromLocator[c.Edit.Qualname] != c.Original {
				t.Fatalf("%s: located %q, candidate original %q", c.Edit.Qualname, fromLocator[c.Edit.Qualname], c.Original)
			}
		}
	})
}
