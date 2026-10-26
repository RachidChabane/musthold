package differential_test

import (
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/discpower/internal/domain/differential"
	"github.com/RachidChabane/musthold/discpower/internal/domain/power"
)

const (
	verifiesSP1973 = " [verifies SP-1973~1]"

	siteA       = "src/a.py::f"
	siteB       = "src/a.py::g"
	formBefore  = "form-before"
	formAfter   = "form-after"
	formElse    = "form-else"
	missingSite = "src/a.py::missing"

	sitePattern = `[a-z]{1,6}::[a-z]{1,6}`
	formPattern = `[a-f0-9]{1,8}`
	maxSites    = 6
)

func TestCompareForms(t *testing.T) {
	reference := differential.CompiledForms{siteA: formBefore, siteB: formElse}
	for _, c := range []struct {
		name   string
		sites  []string
		mutant differential.CompiledForms
		want   power.Identity
	}{
		{"every edited site keeping the reference form is identical", []string{siteA, siteB}, differential.CompiledForms{siteA: formBefore, siteB: formElse}, power.IdentityIdentical},
		{"one edited site changing its form differs", []string{siteA, siteB}, differential.CompiledForms{siteA: formBefore, siteB: formAfter}, power.IdentityDiffers},
		{"a site without form in the mutant is unknown", []string{siteA}, differential.CompiledForms{}, power.IdentityUnknown},
		{"a site without form in the reference is unknown", []string{missingSite}, differential.CompiledForms{missingSite: formAfter}, power.IdentityUnknown},
		{"an empty form is unknown", []string{siteA}, differential.CompiledForms{siteA: ""}, power.IdentityUnknown},
		{"a missing form wins over a differing one", []string{siteA, missingSite}, differential.CompiledForms{siteA: formAfter}, power.IdentityUnknown},
		{"a mutant editing nothing is unknown", nil, differential.CompiledForms{siteA: formBefore}, power.IdentityUnknown},
		{"an empty list of sites is unknown", []string{}, reference, power.IdentityUnknown},
	} {
		t.Run(c.name+verifiesSP1973, func(t *testing.T) {
			if got := differential.CompareForms(c.sites, reference, c.mutant); got != c.want {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
	t.Run("forms compared with themselves never differ"+verifiesSP1973, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			forms := rapid.MapOfN(rapid.StringMatching(sitePattern), rapid.StringMatching(`[a-f0-9]{0,8}`), 0, maxSites).Draw(t, "forms")
			sites := rapid.SliceOf(rapid.StringMatching(sitePattern)).Draw(t, "sites")
			if got := differential.CompareForms(sites, forms, forms); got == power.IdentityDiffers {
				t.Fatalf("a form differs from itself over %v", sites)
			}
		})
	})
	t.Run("a different non-empty form at an edited site always differs"+verifiesSP1973, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			before := rapid.StringMatching(formPattern).Draw(t, "before")
			after := rapid.StringMatching(formPattern).Filter(func(text string) bool { return text != before }).Draw(t, "after")
			got := differential.CompareForms([]string{siteA}, differential.CompiledForms{siteA: before}, differential.CompiledForms{siteA: after})
			if got != power.IdentityDiffers {
				t.Fatalf("got %v", got)
			}
		})
	})
}
