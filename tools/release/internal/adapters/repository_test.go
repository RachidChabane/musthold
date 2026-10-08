package adapters_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/tools/release/internal/adapters"
	"github.com/RachidChabane/spec-proof/tools/release/internal/domain"
	"github.com/RachidChabane/spec-proof/tools/release/internal/testkit"
)

const (
	verifiesTagFacts    = " [verifies SP-1811~1]"
	verifiesGoModFacts  = " [verifies SP-1812~1]"
	verifiesBuildFacts  = " [verifies SP-1813~1]"
	verifiesPluginFacts = " [verifies SP-1814~1]"
	verifiesMarketFacts = " [verifies SP-1815~1]"
	verifiesNotesFacts  = " [verifies SP-1809~1]"
)

func parse(t *testing.T, text string) domain.Tag {
	t.Helper()
	tag, err := domain.ParseTag(text)
	if err != nil {
		t.Fatal(err)
	}
	return tag
}

func TestGit(t *testing.T) {
	repository := testkit.NewRepository(t)
	repository.Write("discpower/a.go", "package a\n", 0o644)
	repository.Write("format/b.go", "package b\n", 0o644)
	repository.Write("ledger/c.go", "package c\n", 0o644)
	first := repository.CommitAll("feat(dp): first")
	repository.Tag("discpower/v0.1.0")
	repository.Write("discpower/a.go", "package a // changed\n", 0o644)
	repository.CommitAll("fix(dp): second")
	repository.Write("ledger/c.go", "package c // changed\n", 0o644)
	repository.CommitAll("fix(ledger): third")
	repository.Write("format/b.go", "package b // changed\n", 0o644)
	head := repository.CommitAll("fix(format): fourth")
	repository.Tag("discpower/v0.2.0-rc.1")
	repository.Tag("discpower/v0.2.0")
	repository.Tag("format/v0.1.0")
	git := adapters.NewGit(repository.Root)

	t.Run("HEAD, a tag's commit and a missing tag are read"+verifiesTagFacts, func(t *testing.T) {
		got, err := git.Head()
		if err != nil || got != head {
			t.Fatalf("head %q, error %v", got, err)
		}
		commit, exists, err := git.TagCommit("discpower/v0.2.0")
		if err != nil || !exists || commit != head {
			t.Fatalf("tag commit %q exists %v, error %v", commit, exists, err)
		}
		oldCommit, exists, _ := git.TagCommit("discpower/v0.1.0")
		if !exists || oldCommit != first {
			t.Fatalf("old tag commit %q", oldCommit)
		}
		if _, exists, err := git.TagCommit("discpower/v9.9.9"); err != nil || exists {
			t.Fatalf("missing tag: exists %v, error %v", exists, err)
		}
	})

	t.Run("the tags are listed"+verifiesTagFacts, func(t *testing.T) {
		tags, err := git.Tags()
		if err != nil {
			t.Fatal(err)
		}
		slices.Sort(tags)
		want := []string{"discpower/v0.1.0", "discpower/v0.2.0", "discpower/v0.2.0-rc.1", "format/v0.1.0"}
		if !slices.Equal(tags, want) {
			t.Fatalf("tags = %v", tags)
		}
	})

	t.Run("the working tree is clean, then lists changed and added files and not ignored ones"+verifiesTagFacts, func(t *testing.T) {
		paths, err := git.DirtyPaths()
		if err != nil || len(paths) != 0 {
			t.Fatalf("paths %v, error %v", paths, err)
		}
		repository.Write("discpower/a.go", "package a // dirty\n", 0o644)
		repository.Write("new/file.txt", "x\n", 0o644)
		paths, err = git.DirtyPaths()
		slices.Sort(paths)
		if err != nil || !slices.Equal(paths, []string{"discpower/a.go", "new/file.txt"}) {
			t.Fatalf("paths %v, error %v", paths, err)
		}
		repository.Git("checkout", "--", "discpower/a.go")
		repository.Git("clean", "--force", "--quiet")
	})

	t.Run("the commit time is the committer time"+verifiesTagFacts, func(t *testing.T) {
		instant, err := git.CommitTime(head)
		if err != nil || instant.Unix() != 1_700_000_000 {
			t.Fatalf("time %v, error %v", instant, err)
		}
	})

	t.Run("the previous tag is the highest one below, pre-releases and other modules apart"+verifiesNotesFacts, func(t *testing.T) {
		cases := map[string]string{
			"discpower/v0.2.0":        "discpower/v0.2.0-rc.1",
			"discpower/v0.2.0-rc.1":   "discpower/v0.1.0",
			"discpower/v0.1.0":        "",
			"discpower/v1.0.0":        "discpower/v0.2.0",
			"ledger/v0.1.0":           "",
			"format/v0.2.0":           "format/v0.1.0",
			"discpower/v0.0.1-alpha":  "",
			"discpower/v0.1.0-beta.2": "",
		}
		for text, want := range cases {
			if got, err := git.PreviousTag(parse(t, text).Module.Name, parse(t, text)); err != nil || got != want {
				t.Errorf("before %s: %q, error %v, want %q", text, got, err, want)
			}
		}
	})

	t.Run("the commits since a tag are those that touched the paths"+verifiesNotesFacts, func(t *testing.T) {
		commits, err := git.CommitsSince("discpower/v0.1.0", []string{"discpower", "format"})
		if err != nil {
			t.Fatal(err)
		}
		var subjects []string
		for _, commit := range commits {
			subjects = append(subjects, commit.Subject)
			if len(commit.Hash) < 7 {
				t.Errorf("hash %q", commit.Hash)
			}
		}
		if !slices.Equal(subjects, []string{"fix(format): fourth", "fix(dp): second"}) {
			t.Fatalf("subjects = %v", subjects)
		}
		all, err := git.CommitsSince("", []string{"ledger"})
		if err != nil || len(all) != 2 {
			t.Fatalf("whole history of ledger: %v, error %v", all, err)
		}
	})
}

func TestWorkspace(t *testing.T) {
	repository := testkit.NewReleaseRepository(t)
	workspace := adapters.NewWorkspace(repository.Root)

	t.Run("the replace rule and the format requirement of a go.mod are read"+verifiesGoModFacts, func(t *testing.T) {
		facts, err := workspace.GoMod("discpower")
		if err != nil {
			t.Fatal(err)
		}
		if facts.FormatRequirement != "v0.0.0" || len(facts.Replaces) != 1 || !strings.Contains(facts.Replaces[0], "../format") || len(facts.Excludes) != 0 {
			t.Fatalf("facts = %+v", facts)
		}
		clean, err := workspace.GoMod("format")
		if err != nil || len(clean.Replaces) != 0 || clean.FormatRequirement != "" {
			t.Fatalf("format facts = %+v, error %v", clean, err)
		}
	})

	t.Run("exclude rules are read"+verifiesGoModFacts, func(t *testing.T) {
		repository.Write("format/go.mod", "module github.com/RachidChabane/spec-proof/format\n\ngo 1.26.4\n\nexclude example.com/x v1.0.0\n", 0o644)
		facts, err := workspace.GoMod("format")
		if err != nil || len(facts.Excludes) != 1 || !strings.Contains(facts.Excludes[0], "example.com/x") {
			t.Fatalf("facts = %+v, error %v", facts, err)
		}
	})

	t.Run("a go.mod that is missing or malformed is an error"+verifiesGoModFacts, func(t *testing.T) {
		if _, err := workspace.GoMod("absent"); err == nil {
			t.Error("a missing go.mod was read")
		}
		repository.Write("broken/go.mod", "this is not a go.mod\n", 0o644)
		if _, err := workspace.GoMod("broken"); err == nil {
			t.Error("a malformed go.mod was read")
		}
	})

	t.Run("the version of every plugin manifest under the roots is read, a missing root apart"+verifiesPluginFacts, func(t *testing.T) {
		repository.Write("discpower/plugin/portable/plugin.json", `{"name":"x","version":"0.3.0"}`, 0o644)
		manifests, err := workspace.Manifests([]string{"discpower/plugin", "discpower/absent"})
		if err != nil {
			t.Fatal(err)
		}
		want := []domain.ManifestFact{
			{Path: "discpower/plugin/claude/.claude-plugin/plugin.json", Version: "0.1.0"},
			{Path: "discpower/plugin/portable/plugin.json", Version: "0.3.0"},
		}
		if !slices.Equal(manifests, want) {
			t.Fatalf("manifests = %v", manifests)
		}
	})

	t.Run("a manifest that is not JSON is an error"+verifiesPluginFacts, func(t *testing.T) {
		repository.Write("umbrella/plugin/plugin.json", "{", 0o644)
		if _, err := workspace.Manifests([]string{"umbrella/plugin"}); err == nil {
			t.Fatal("read")
		}
	})

	t.Run("the marketplace entries are read"+verifiesMarketFacts, func(t *testing.T) {
		entries, err := workspace.Marketplace()
		if err != nil {
			t.Fatal(err)
		}
		want := []domain.MarketplaceEntry{
			{Name: "spec-proof-dp", Repository: "RachidChabane/spec-proof"},
			{Name: "spec-proof", Relative: true},
		}
		if !slices.Equal(entries, want) {
			t.Fatalf("entries = %v", entries)
		}
	})
}

func TestChecks(t *testing.T) {
	repository := testkit.NewReleaseRepository(t)
	toolchain := adapters.NewGoToolchain(repository.Root)

	t.Run("a module that builds and tests passes both steps outside the workspace"+verifiesBuildFacts, func(t *testing.T) {
		repository.Write("format/format_test.go", "package format\n\nimport \"testing\"\n\nfunc TestMarker(t *testing.T) {\n\tif Marker == \"\" {\n\t\tt.Fatal(\"empty\")\n\t}\n}\n", 0o644)
		if outcome := toolchain.Build("format"); !outcome.Ran || outcome.Failure != "" {
			t.Fatalf("build = %+v", outcome)
		}
		if outcome := toolchain.Test("format"); !outcome.Ran || outcome.Failure != "" {
			t.Fatalf("test = %+v", outcome)
		}
	})

	t.Run("a module that does not compile fails the build with the compiler's words"+verifiesBuildFacts, func(t *testing.T) {
		repository.Write("format/broken.go", "package format\n\nvar _ = undefinedName\n", 0o644)
		outcome := toolchain.Build("format")
		if !outcome.Ran || !strings.Contains(outcome.Failure, "undefinedName") {
			t.Fatalf("build = %+v", outcome)
		}
	})

	t.Run("a test that fails fails the test step"+verifiesBuildFacts, func(t *testing.T) {
		repository.Write("format/broken.go", "package format\n", 0o644)
		repository.Write("format/format_test.go", "package format\n\nimport \"testing\"\n\nfunc TestFails(t *testing.T) { t.Fatal(\"boom\") }\n", 0o644)
		outcome := toolchain.Test("format")
		if !outcome.Ran || !strings.Contains(outcome.Failure, "boom") {
			t.Fatalf("test = %+v", outcome)
		}
	})
}

func TestPluginGenerator(t *testing.T) {
	repository := testkit.NewReleaseRepository(t)
	generator := adapters.NewPluginGenerator(repository.Root)

	t.Run("a generator that changes nothing passes"+verifiesPluginFacts, func(t *testing.T) {
		if outcome := generator.PluginGenerated("discpower"); !outcome.Ran || outcome.Failure != "" {
			t.Fatalf("outcome = %+v", outcome)
		}
	})

	t.Run("a generator that rewrites a committed file of the module fails"+verifiesPluginFacts, func(t *testing.T) {
		repository.Write("script/plugin-gen", "#!/usr/bin/env bash\necho changed > discpower/plugin/claude/.claude-plugin/plugin.json\n", 0o755)
		repository.CommitAll("generator rewrites the manifest")
		outcome := generator.PluginGenerated("discpower")
		if !outcome.Ran || !strings.Contains(outcome.Failure, "plugin.json") {
			t.Fatalf("outcome = %+v", outcome)
		}
		repository.Git("checkout", "--", "discpower")
	})

	t.Run("a generator that adds a file to the module fails"+verifiesPluginFacts, func(t *testing.T) {
		repository.Write("script/plugin-gen", "#!/usr/bin/env bash\necho new > discpower/plugin/claude/new.json\n", 0o755)
		repository.CommitAll("generator adds a file")
		outcome := generator.PluginGenerated("discpower")
		if !outcome.Ran || !strings.Contains(outcome.Failure, "new.json") {
			t.Fatalf("outcome = %+v", outcome)
		}
	})

	t.Run("a missing generator fails"+verifiesPluginFacts, func(t *testing.T) {
		missing := adapters.NewPluginGenerator(t.TempDir())
		if outcome := missing.PluginGenerated("discpower"); outcome.Failure == "" {
			t.Fatalf("outcome = %+v", outcome)
		}
	})
}
