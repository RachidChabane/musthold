package domain_test

import (
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/tools/release/internal/domain"
)

const (
	verifiesTagRules      = " [verifies SP-1811~1]"
	verifiesGoModRules    = " [verifies SP-1812~1]"
	verifiesStepRules     = " [verifies SP-1813~1]"
	verifiesPluginRules   = " [verifies SP-1814~1]"
	verifiesMarketRules   = " [verifies SP-1815~1]"
	verifiesVisibility    = " [verifies SP-1816~1]"
	verifiesRefusalPolicy = " [verifies SP-1817~1]"
	releasedCommit        = "1111111111111111111111111111111111111111"
	otherCommit           = "2222222222222222222222222222222222222222"
)

// releasableFacts are the facts of a tag that passes every rule.
func releasableFacts(t *testing.T, text string) domain.Facts {
	t.Helper()
	tag := mustTag(t, text)
	facts := domain.Facts{
		Tag: tag, HeadCommit: releasedCommit, TagCommit: releasedCommit,
		GoMod: domain.GoModFacts{FormatRequirement: "v0.1.0"},
		Tags:  []string{"format/v0.1.0"},
		Build: domain.Outcome{Ran: true}, Test: domain.Outcome{Ran: true}, PluginGen: domain.Outcome{Ran: true},
		Visibility: domain.VisibilityPublic, RequirePublic: true,
	}
	if tag.Module.ShipsPlugin() {
		facts.Manifests = []domain.ManifestFact{{Path: tag.Module.Name + "/plugin/claude/.claude-plugin/plugin.json", Version: tag.Semver()}}
		facts.Marketplace = []domain.MarketplaceEntry{{Name: tag.Module.PluginName, Repository: domain.RepositorySlug, Ref: tag.String()}}
	}
	return facts
}

func findingsOf(facts domain.Facts) []domain.Finding {
	return domain.Evaluate(domain.Rules(), facts).Findings
}

func rulesOf(findings []domain.Finding) []string {
	var names []string
	for _, finding := range findings {
		names = append(names, finding.Rule)
	}
	return names
}

func onlyFinding(t *testing.T, facts domain.Facts, rule string, scope domain.Scope) domain.Finding {
	t.Helper()
	findings := findingsOf(facts)
	if len(findings) != 1 || findings[0].Rule != rule || findings[0].Scope != scope {
		t.Fatalf("findings = %v, want one %s of scope %d", findings, rule, scope)
	}
	return findings[0]
}

func TestReleasableTagHasNoFinding(t *testing.T) {
	t.Run("a tag whose facts are all in order has no finding, for every module"+verifiesRefusalPolicy, func(t *testing.T) {
		for _, module := range domain.Modules() {
			facts := releasableFacts(t, module.Name+"/v0.2.0")
			if module.Name == domain.FormatModuleName {
				facts.GoMod.FormatRequirement = ""
			}
			if findings := findingsOf(facts); len(findings) != 0 {
				t.Errorf("%s: %v", module.Name, findings)
			}
		}
	})
}

func TestTagRules(t *testing.T) {
	t.Run("a tag that names another commit than HEAD stops every run"+verifiesTagRules, func(t *testing.T) {
		facts := releasableFacts(t, "discpower/v0.2.0")
		facts.TagCommit = otherCommit
		finding := onlyFinding(t, facts, domain.RuleTagAtHead, domain.ScopeAlways)
		if !strings.Contains(finding.Message, otherCommit) || !strings.Contains(finding.Message, releasedCommit) {
			t.Errorf("message = %q", finding.Message)
		}
	})

	t.Run("a tag that does not exist is a finding only a real release needs"+verifiesTagRules, func(t *testing.T) {
		facts := releasableFacts(t, "discpower/v0.2.0")
		facts.TagCommit = ""
		onlyFinding(t, facts, domain.RuleTagAtHead, domain.ScopeRelease)
	})

	t.Run("a changed working tree is a finding only a real release needs"+verifiesTagRules, func(t *testing.T) {
		facts := releasableFacts(t, "discpower/v0.2.0")
		facts.DirtyPaths = []string{"discpower/x.go", "?? y"}
		finding := onlyFinding(t, facts, domain.RuleCleanTree, domain.ScopeRelease)
		if !strings.Contains(finding.Message, "discpower/x.go") {
			t.Errorf("message = %q", finding.Message)
		}
	})
}

func TestGoModRules(t *testing.T) {
	t.Run("every replace and exclude rule is a finding only a real release needs"+verifiesGoModRules, func(t *testing.T) {
		facts := releasableFacts(t, "discpower/v0.2.0")
		facts.GoMod.Replaces = []string{"github.com/RachidChabane/spec-proof/format => ../format", "x => y"}
		facts.GoMod.Excludes = []string{"z v1.0.0"}
		findings := findingsOf(facts)
		if len(findings) != 3 {
			t.Fatalf("findings = %v", findings)
		}
		for _, finding := range findings {
			if finding.Rule != domain.RuleGoModReleasable || finding.Scope != domain.ScopeRelease {
				t.Errorf("finding = %v", finding)
			}
		}
	})

	t.Run("a format requirement with no tag names the tag to create"+verifiesGoModRules, func(t *testing.T) {
		facts := releasableFacts(t, "discpower/v0.2.0")
		facts.GoMod.FormatRequirement = "v0.0.0"
		finding := onlyFinding(t, facts, domain.RuleFormatTagged, domain.ScopeRelease)
		if !strings.Contains(finding.Message, "format/v0.0.0") {
			t.Errorf("message = %q", finding.Message)
		}
	})

	t.Run("a module that requires no format library, and the library itself, are not asked for a format tag"+verifiesGoModRules, func(t *testing.T) {
		facts := releasableFacts(t, "ledger/v0.2.0")
		facts.GoMod.FormatRequirement = ""
		facts.Tags = nil
		if findings := findingsOf(facts); len(findings) != 0 {
			t.Errorf("findings = %v", findings)
		}
		library := releasableFacts(t, "format/v0.2.0")
		library.GoMod.FormatRequirement = "v9.9.9"
		if findings := findingsOf(library); len(findings) != 0 {
			t.Errorf("findings = %v", findings)
		}
	})
}

func TestStepRules(t *testing.T) {
	t.Run("a failed build and a failed test each stop every run"+verifiesStepRules, func(t *testing.T) {
		facts := releasableFacts(t, "discpower/v0.2.0")
		facts.Build = domain.Outcome{Ran: true, Failure: "undefined: x"}
		finding := onlyFinding(t, facts, domain.RuleBuild, domain.ScopeAlways)
		if !strings.Contains(finding.Message, "undefined: x") {
			t.Errorf("message = %q", finding.Message)
		}
		facts = releasableFacts(t, "discpower/v0.2.0")
		facts.Test = domain.Outcome{Ran: true, Failure: "FAIL"}
		onlyFinding(t, facts, domain.RuleTest, domain.ScopeAlways)
	})

	t.Run("a step that did not run is no finding"+verifiesStepRules, func(t *testing.T) {
		facts := releasableFacts(t, "discpower/v0.2.0")
		facts.Test, facts.PluginGen = domain.Outcome{}, domain.Outcome{}
		if findings := findingsOf(facts); len(findings) != 0 {
			t.Errorf("findings = %v", findings)
		}
	})
}

func TestPluginRules(t *testing.T) {
	t.Run("a manifest of another version is a finding only a real release needs"+verifiesPluginRules, func(t *testing.T) {
		facts := releasableFacts(t, "discpower/v0.2.0")
		facts.Manifests = append(facts.Manifests, domain.ManifestFact{Path: "discpower/plugin/portable/plugin.json", Version: "0.1.0"})
		finding := onlyFinding(t, facts, domain.RulePluginVersions, domain.ScopeRelease)
		if !strings.Contains(finding.Message, "discpower/plugin/portable/plugin.json") {
			t.Errorf("message = %q", finding.Message)
		}
	})

	t.Run("a plugin module with no manifest stops every run"+verifiesPluginRules, func(t *testing.T) {
		facts := releasableFacts(t, "discpower/v0.2.0")
		facts.Manifests = nil
		onlyFinding(t, facts, domain.RulePluginVersions, domain.ScopeAlways)
	})

	t.Run("a generator that changes the committed files stops every run"+verifiesPluginRules, func(t *testing.T) {
		facts := releasableFacts(t, "discpower/v0.2.0")
		facts.PluginGen = domain.Outcome{Ran: true, Failure: "M discpower/plugin/x"}
		onlyFinding(t, facts, domain.RulePluginGenerated, domain.ScopeAlways)
	})

	t.Run("a module without a plugin is asked for no manifest and no generator"+verifiesPluginRules, func(t *testing.T) {
		facts := releasableFacts(t, "ledger/v0.2.0")
		facts.PluginGen = domain.Outcome{Ran: true, Failure: "M x"}
		if findings := findingsOf(facts); len(findings) != 0 {
			t.Errorf("findings = %v", findings)
		}
	})
}

func TestMarketplaceRules(t *testing.T) {
	t.Run("an entry whose ref is not the tag is a finding only a real release needs"+verifiesMarketRules, func(t *testing.T) {
		facts := releasableFacts(t, "discpower/v0.2.0")
		facts.Marketplace[0].Ref = "discpower/v0.1.0"
		facts.Tags = append(facts.Tags, "discpower/v0.1.0")
		onlyFinding(t, facts, domain.RuleMarketplaceRef, domain.ScopeRelease)
	})

	t.Run("an entry with no ref is a finding only a real release needs"+verifiesMarketRules, func(t *testing.T) {
		facts := releasableFacts(t, "discpower/v0.2.0")
		facts.Marketplace[0].Ref = ""
		onlyFinding(t, facts, domain.RuleMarketplaceRef, domain.ScopeRelease)
	})

	t.Run("a marketplace with no entry for the plugin stops every run"+verifiesMarketRules, func(t *testing.T) {
		facts := releasableFacts(t, "discpower/v0.2.0")
		facts.Marketplace[0].Name = "another"
		onlyFinding(t, facts, domain.RuleMarketplaceRef, domain.ScopeAlways)
	})

	t.Run("a ref of this repository that is no tag is a finding, a ref of another repository is not"+verifiesMarketRules, func(t *testing.T) {
		facts := releasableFacts(t, "discpower/v0.2.0")
		facts.Marketplace = append(facts.Marketplace,
			domain.MarketplaceEntry{Name: "alignment", Repository: domain.RepositorySlug, Ref: "alignment/v9.9.9"},
			domain.MarketplaceEntry{Name: "shallnot", Repository: "RachidChabane/shallnot", Ref: "v9.9.9"},
		)
		finding := onlyFinding(t, facts, domain.RuleMarketplaceRefs, domain.ScopeRelease)
		if !strings.Contains(finding.Message, "alignment/v9.9.9") {
			t.Errorf("message = %q", finding.Message)
		}
	})

	t.Run("an entry whose source is a path inside the marketplace needs no ref"+verifiesMarketRules, func(t *testing.T) {
		facts := releasableFacts(t, "umbrella/v0.2.0")
		facts.Marketplace = []domain.MarketplaceEntry{{Name: "spec-proof", Relative: true}}
		if findings := findingsOf(facts); len(findings) != 0 {
			t.Errorf("findings = %v", findings)
		}
	})

	t.Run("the marketplace file is read as paths and pinned repositories"+verifiesMarketRules, func(t *testing.T) {
		entries, err := domain.ParseMarketplace([]byte(`{"plugins":[
			{"name":"a","source":"./umbrella/plugins/a"},
			{"name":"b","source":{"source":"git-subdir","url":"RachidChabane/spec-proof","path":"b/plugin/claude","ref":"b/v1.0.0"}},
			{"name":"c","source":{"source":"git-subdir","url":"https://github.com/RachidChabane/spec-proof.git","path":"c"}}]}`))
		if err != nil {
			t.Fatal(err)
		}
		want := []domain.MarketplaceEntry{
			{Name: "a", Relative: true},
			{Name: "b", Repository: "RachidChabane/spec-proof", Ref: "b/v1.0.0"},
			{Name: "c", Repository: "RachidChabane/spec-proof"},
		}
		if len(entries) != len(want) {
			t.Fatalf("entries = %v", entries)
		}
		for index := range want {
			if entries[index] != want[index] {
				t.Errorf("entry %d = %+v, want %+v", index, entries[index], want[index])
			}
		}
		if _, err := domain.ParseMarketplace([]byte("{")); err == nil {
			t.Error("a broken file parsed")
		}
	})

	t.Run("a plugin manifest is read for its version"+verifiesMarketRules, func(t *testing.T) {
		version, err := domain.ParseManifestVersion([]byte(`{"name":"x","version":"0.3.1"}`))
		if err != nil || version != "0.3.1" {
			t.Fatalf("version %q, error %v", version, err)
		}
		if _, err := domain.ParseManifestVersion([]byte("[")); err == nil {
			t.Error("a broken manifest parsed")
		}
	})
}

func TestVisibilityRule(t *testing.T) {
	t.Run("a private or unknown visibility is a finding only a real release needs, when public is required"+verifiesVisibility, func(t *testing.T) {
		for _, visibility := range []domain.Visibility{domain.VisibilityPrivate, domain.VisibilityUnknown} {
			facts := releasableFacts(t, "discpower/v0.2.0")
			facts.Visibility = visibility
			onlyFinding(t, facts, domain.RuleRepositoryPublic, domain.ScopeRelease)
		}
	})

	t.Run("the visibility is not asked for unless public is required"+verifiesVisibility, func(t *testing.T) {
		facts := releasableFacts(t, "discpower/v0.2.0")
		facts.RequirePublic, facts.Visibility = false, domain.VisibilityPrivate
		if findings := findingsOf(facts); len(findings) != 0 {
			t.Errorf("findings = %v", findings)
		}
	})
}

func TestRefusalPolicy(t *testing.T) {
	facts := releasableFacts(t, "discpower/v0.2.0")
	facts.GoMod.Replaces = []string{"format => ../format"}
	facts.Build = domain.Outcome{Ran: true, Failure: "broken"}
	report := domain.Evaluate(domain.Rules(), facts)

	t.Run("a real release is refused on every finding"+verifiesRefusalPolicy, func(t *testing.T) {
		if got := rulesOf(report.Refusals(false)); len(got) != 2 {
			t.Fatalf("refusals = %v", got)
		}
	})

	t.Run("a dry run is refused only on the findings that stop it"+verifiesRefusalPolicy, func(t *testing.T) {
		got := report.Refusals(true)
		if len(got) != 1 || got[0].Rule != domain.RuleBuild {
			t.Fatalf("refusals = %v", got)
		}
	})

	t.Run("a dry run with only release findings is not refused"+verifiesRefusalPolicy, func(t *testing.T) {
		facts.Build = domain.Outcome{Ran: true}
		if got := domain.Evaluate(domain.Rules(), facts).Refusals(true); len(got) != 0 {
			t.Fatalf("refusals = %v", got)
		}
	})
}
