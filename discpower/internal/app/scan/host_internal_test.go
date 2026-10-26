package scan

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/promise"
	"github.com/RachidChabane/musthold/discpower/internal/domain/vacuity"
	"github.com/RachidChabane/musthold/discpower/internal/domain/witness"
	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto"
)

const (
	verifiesAskTags     = " [verifies SP-2243~1]"
	verifiesKeyBinding  = " [verifies SP-2238~1]"
	verifiesDifferences = " [verifies SP-2239~1]"
	verifiesWholeSuite  = " [verifies SP-2235~1]"
)

func TestAskTags(t *testing.T) {
	t.Run("every ask a scan makes reads back from its queue tag and the vote of its ticket"+verifiesAskTags, func(t *testing.T) {
		asks := []struct {
			ask  promise.Ask
			vote int
		}{
			{promise.Extraction(), 0},
			{promise.Writer("abc", 1), 1},
			{promise.Scope("abc", 0), 0},
			{promise.Generation("abc", 3), 0},
			{promise.LabelTicket(), 0},
		}
		for _, one := range asks {
			got, known := askOfTag(tagOf(one.ask), one.vote)
			if !known || got != one.ask {
				t.Fatalf("%+v read back as %+v, %t", one.ask, got, known)
			}
		}
	})
	t.Run("an emission no tag names is an ask of its own that spends the budget"+verifiesAskTags, func(t *testing.T) {
		if got := askOfEmission("not a ticket", nil); got != promise.Other() {
			t.Fatalf("ask %+v", got)
		}
		ticket := hostproto.TicketID{RequestID: hostproto.RequestID(strings.Repeat("a", 64)), Vote: 0, Emission: 1}.String()
		if got := askOfEmission(ticket, map[hostproto.RequestID][]string{}); got != promise.Other() {
			t.Fatalf("ask %+v", got)
		}
	})
}

func boundScan() binding {
	request := Request{Seed: 7, SampleSize: 40, Mutants: 150, Budget: time.Minute, Module: format.Module{Version: "0.1.0"}}
	sources := []quotableSource{{name: format.ScanSourceReadme, content: []byte("readme")}, {name: format.ScanSourceDescription, content: []byte("title\n\nbody")}}
	return newBinding(request, "c0ffee", "tree", change{base: "ba5e", pull: &PullRequest{Number: 12}}, sources)
}

func TestBinding(t *testing.T) {
	t.Run("the session key digests the whole binding: any field that changes changes the key"+verifiesKeyBinding, func(t *testing.T) {
		original, err := boundScan().canonical()
		if err != nil {
			t.Fatal(err)
		}
		changes := []func(*binding){
			func(b *binding) { b.Commit = "beef" },
			func(b *binding) { b.Tree = "other" },
			func(b *binding) { b.Base = "" },
			func(b *binding) { b.PullRequest = 13 },
			func(b *binding) { b.Seed = "8" },
			func(b *binding) { b.SampleSize = 12 },
			func(b *binding) { b.Mutants = 72 },
			func(b *binding) { b.Budget = "0s" },
			func(b *binding) { b.Version = "0.2.0" },
			func(b *binding) { b.Sources[0].Digest = "changed" },
		}
		for index, change := range changes {
			changed := boundScan()
			change(&changed)
			text, err := changed.canonical()
			if err != nil {
				t.Fatal(err)
			}
			if sessionKeyOf(text) == sessionKeyOf(original) {
				t.Fatalf("change %d keeps the key", index)
			}
		}
		if key := sessionKeyOf(original); !hostproto.ScanSession(key).IsScan() || len(key) != sessionKeyLength {
			t.Fatalf("key %q", key)
		}
	})
	t.Run("a binding names what it changes from an older one, sources included"+verifiesDifferences, func(t *testing.T) {
		older := boundScan()
		newer := boundScan()
		newer.Seed, newer.Commit = "8", "beef"
		newer.Sources = append(newer.Sources[1:], boundSource{Name: format.ScanSourceIssue(4), Digest: "d"})
		newer.Sources[0].Digest = "other"
		want := []string{"commit c0ffee → beef", "seed 7 → 8", "source README.md removed", "source description changed", "source issue #4 added"}
		if got := newer.differences(older); !slices.Equal(got, want) {
			t.Fatalf("differences %q", got)
		}
		if got := older.differences(older); len(got) != 0 {
			t.Fatalf("an equal binding differs: %q", got)
		}
	})
}

func TestSuiteRuns(t *testing.T) {
	t.Run("a run of the whole suite passes when every test green on the reference passed it, whatever the others did"+verifiesWholeSuite, func(t *testing.T) {
		green := []ecosystem.TestID{"a", "b"}
		runs := map[ecosystem.TestID][]vacuity.Run{
			"a":   {vacuity.RunPassed, vacuity.RunPassed, vacuity.RunPassed},
			"b":   {vacuity.RunPassed, vacuity.RunFailed, vacuity.RunPassed},
			"red": {vacuity.RunFailed, vacuity.RunFailed, vacuity.RunFailed},
		}
		want := []witness.Outcome{witness.OutcomePassed, witness.OutcomeFailed, witness.OutcomePassed}
		if got := suiteRuns(runs, green); !slices.Equal(got, want) {
			t.Fatalf("runs %v", got)
		}
		if got := suiteRuns(map[ecosystem.TestID][]vacuity.Run{}, green); slices.Contains(got, witness.OutcomePassed) {
			t.Fatalf("a suite with no run passed: %v", got)
		}
	})
}

func TestLabelSite(t *testing.T) {
	t.Run("a mutant's function is the innermost unit that holds its site, whatever unit its first edit names [verifies SP-2236~1]", func(t *testing.T) {
		units := []viewUnit{
			{file: "a.py", qualname: "Box", lines: []int{1, 2, 3, 4, 5}},
			{file: "a.py", qualname: "Box.size", lines: []int{3, 4}},
			{file: "b.py", qualname: "size", lines: []int{3, 4}},
		}
		unit, found := unitAt(units, format.ScanLine{File: "a.py", Line: 4})
		if !found || unit.key() != "a.py::Box.size" {
			t.Fatalf("unit %+v, %t", unit, found)
		}
		if _, found := unitAt(units, format.ScanLine{File: "a.py", Line: 9}); found {
			t.Fatal("a line outside every unit has a unit")
		}
	})
}
