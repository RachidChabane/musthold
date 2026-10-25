package domain

import (
	"fmt"
	"slices"
	"strings"
)

// Scope says when a finding stops a run.
type Scope int

const (
	// ScopeAlways is a finding that stops every run, a dry run included: the tree
	// cannot be built, or the tag belongs to another commit.
	ScopeAlways Scope = iota
	// ScopeRelease is a finding about the state a real release needs: a dry run
	// reports it and goes on, a real release stops.
	ScopeRelease
)

// Finding is one reason a release is refused.
type Finding struct {
	Rule    string
	Scope   Scope
	Message string
}

// String renders the finding as `rule: message`.
func (f Finding) String() string { return f.Rule + ": " + f.Message }

// Outcome is the result of a step that runs a command.
type Outcome struct {
	// Ran is false for a step that was skipped.
	Ran bool
	// Failure is the output of the failed step, empty when it passed.
	Failure string
}

// GoModFacts is what the go.mod of the released module says.
type GoModFacts struct {
	Replaces []string
	Excludes []string
	// FormatRequirement is the version of the format library the module requires,
	// empty when it requires none.
	FormatRequirement string
}

// ManifestFact is the version one generated plugin manifest carries.
type ManifestFact struct {
	Path    string
	Version string
}

// MarketplaceEntry is a plugin of the root marketplace.
type MarketplaceEntry struct {
	Name string
	// Relative is true for a plugin whose source is a path inside the marketplace.
	Relative bool
	// Repository is the `owner/name` of an external source.
	Repository string
	Ref        string
}

// NoticeFacts is what the NOTICE file at the repository root says.
type NoticeFacts struct {
	// Present is false when the repository has no NOTICE file.
	Present bool
	Text    string
}

// Visibility is whether the repository is public.
type Visibility int

// The visibilities.
const (
	VisibilityUnknown Visibility = iota
	VisibilityPublic
	VisibilityPrivate
)

// Facts is everything the pre-flight rules read. The adapters collect it; the
// rules never touch the repository.
type Facts struct {
	Tag        Tag
	HeadCommit string
	// TagCommit is the commit the tag names, empty when the tag does not exist.
	TagCommit string
	// DirtyPaths are the paths the working tree has changed.
	DirtyPaths []string
	GoMod      GoModFacts
	// Tags are the tags of the repository.
	Tags      []string
	Build     Outcome
	Test      Outcome
	PluginGen Outcome
	Manifests []ManifestFact
	// Marketplace lists the plugins of the root marketplace.
	Marketplace []MarketplaceEntry
	// ThirdPartyNotices is whether the repository root holds the third-party
	// notices, read only for a module that ships a binary.
	ThirdPartyNotices bool
	// Notice is the NOTICE file of the repository root.
	Notice NoticeFacts
	// Visibility is the repository's, read only when RequirePublic is set.
	Visibility    Visibility
	RequirePublic bool
}

// Rule is one check of the pre-flight.
type Rule interface {
	Check(facts Facts) []Finding
}

// Names of the rules.
const (
	RuleTagAtHead        = "tag-at-head"
	RuleCleanTree        = "clean-tree"
	RuleGoModReleasable  = "go-mod-releasable"
	RuleFormatTagged     = "format-tagged"
	RuleBuild            = "build"
	RuleTest             = "test"
	RulePluginVersions   = "plugin-versions"
	RulePluginGenerated  = "plugin-generated"
	RuleMarketplaceRef   = "marketplace-ref"
	RuleMarketplaceRefs  = "marketplace-refs-resolve"
	RuleRepositoryPublic = "repository-public"
	RuleNotices          = "third-party-notices"
	RuleNoticeHolder     = "notice-holder"
)

// Rules returns the pre-flight rule set that CI and the dry run share.
func Rules() []Rule {
	return []Rule{
		tagAtHead{}, cleanTree{}, goModReleasable{}, formatTagged{},
		stepPasses{name: RuleBuild, step: func(f Facts) Outcome { return f.Build }, describe: "GOWORK=off go build ./..."},
		stepPasses{name: RuleTest, step: func(f Facts) Outcome { return f.Test }, describe: "GOWORK=off go test ./..."},
		pluginVersions{}, pluginGenerated{}, marketplaceRef{}, marketplaceRefsResolve{}, noticeHolder{}, repositoryPublic{}, thirdPartyNotices{},
	}
}

// Report is the outcome of the rules over a set of facts.
type Report struct{ Findings []Finding }

// Evaluate runs the rules over the facts.
func Evaluate(rules []Rule, facts Facts) Report {
	var report Report
	for _, rule := range rules {
		report.Findings = append(report.Findings, rule.Check(facts)...)
	}
	return report
}

// Refusals are the findings that stop a run: all of them for a real release, only
// the ScopeAlways ones for a dry run.
func (r Report) Refusals(dryRun bool) []Finding {
	var refusals []Finding
	for _, finding := range r.Findings {
		if !dryRun || finding.Scope == ScopeAlways {
			refusals = append(refusals, finding)
		}
	}
	return refusals
}

type tagAtHead struct{}

func (tagAtHead) Check(f Facts) []Finding {
	switch {
	case f.TagCommit == "":
		return []Finding{{RuleTagAtHead, ScopeRelease, fmt.Sprintf("the tag %s does not exist", f.Tag)}}
	case f.TagCommit != f.HeadCommit:
		return []Finding{{RuleTagAtHead, ScopeAlways, fmt.Sprintf("the tag %s names %s but HEAD is %s", f.Tag, f.TagCommit, f.HeadCommit)}}
	}
	return nil
}

type cleanTree struct{}

func (cleanTree) Check(f Facts) []Finding {
	if len(f.DirtyPaths) == 0 {
		return nil
	}
	return []Finding{{RuleCleanTree, ScopeRelease, "the working tree has changes: " + strings.Join(f.DirtyPaths, ", ")}}
}

type goModReleasable struct{}

func (goModReleasable) Check(f Facts) []Finding {
	var findings []Finding
	for _, replace := range f.GoMod.Replaces {
		findings = append(findings, Finding{RuleGoModReleasable, ScopeRelease, "go.mod has a replace rule: " + replace + " (go install refuses a module with one)"})
	}
	for _, exclude := range f.GoMod.Excludes {
		findings = append(findings, Finding{RuleGoModReleasable, ScopeRelease, "go.mod has an exclude rule: " + exclude})
	}
	return findings
}

type formatTagged struct{}

func (formatTagged) Check(f Facts) []Finding {
	requirement := f.GoMod.FormatRequirement
	if requirement == "" || !f.Tag.Module.RequiresFormat() {
		return nil
	}
	if slices.Contains(f.Tags, FormatModuleName+"/"+requirement) {
		return nil
	}
	return []Finding{{RuleFormatTagged, ScopeRelease, fmt.Sprintf("go.mod requires %s %s, which has no tag %s/%s: tag format first", FormatModulePath, requirement, FormatModuleName, requirement)}}
}

type stepPasses struct {
	name     string
	step     func(Facts) Outcome
	describe string
}

func (s stepPasses) Check(f Facts) []Finding {
	if outcome := s.step(f); outcome.Ran && outcome.Failure != "" {
		return []Finding{{s.name, ScopeAlways, s.describe + " fails: " + outcome.Failure}}
	}
	return nil
}

type pluginVersions struct{}

func (pluginVersions) Check(f Facts) []Finding {
	if !f.Tag.Module.ShipsPlugin() {
		return nil
	}
	if len(f.Manifests) == 0 {
		return []Finding{{RulePluginVersions, ScopeAlways, "no plugin manifest found for " + f.Tag.Module.Name}}
	}
	var findings []Finding
	for _, manifest := range f.Manifests {
		if manifest.Version != f.Tag.Semver() {
			findings = append(findings, Finding{RulePluginVersions, ScopeRelease, fmt.Sprintf("%s carries version %s, the tag is %s", manifest.Path, manifest.Version, f.Tag.Semver())})
		}
	}
	return findings
}

type pluginGenerated struct{}

func (pluginGenerated) Check(f Facts) []Finding {
	if !f.Tag.Module.ShipsPlugin() || !f.PluginGen.Ran || f.PluginGen.Failure == "" {
		return nil
	}
	return []Finding{{RulePluginGenerated, ScopeAlways, PluginGenScript + " changes the committed plugin files: " + f.PluginGen.Failure}}
}

type marketplaceRef struct{}

func (marketplaceRef) Check(f Facts) []Finding {
	module := f.Tag.Module
	if !module.ShipsPlugin() {
		return nil
	}
	for _, entry := range f.Marketplace {
		if entry.Name != module.PluginName {
			continue
		}
		if entry.Relative || entry.Ref == f.Tag.String() {
			return nil
		}
		return []Finding{{RuleMarketplaceRef, ScopeRelease, fmt.Sprintf("the marketplace entry %s has ref %q, want %q", entry.Name, entry.Ref, f.Tag)}}
	}
	return []Finding{{RuleMarketplaceRef, ScopeAlways, fmt.Sprintf("the marketplace has no entry named %s", module.PluginName)}}
}

type marketplaceRefsResolve struct{}

func (marketplaceRefsResolve) Check(f Facts) []Finding {
	if !f.Tag.Module.ShipsPlugin() {
		return nil
	}
	var findings []Finding
	for _, entry := range f.Marketplace {
		if entry.Repository != RepositorySlug || entry.Ref == "" || entry.Ref == f.Tag.String() || slices.Contains(f.Tags, entry.Ref) {
			continue
		}
		findings = append(findings, Finding{RuleMarketplaceRefs, ScopeRelease, fmt.Sprintf("the marketplace entry %s has ref %q, which is no tag", entry.Name, entry.Ref)})
	}
	return findings
}

type repositoryPublic struct{}

func (repositoryPublic) Check(f Facts) []Finding {
	if !f.RequirePublic || f.Visibility == VisibilityPublic {
		return nil
	}
	reason := "the repository is private"
	if f.Visibility == VisibilityUnknown {
		reason = "the visibility of the repository is unknown"
	}
	return []Finding{{RuleRepositoryPublic, ScopeRelease, reason + ": a release is published only from a public repository"}}
}

type thirdPartyNotices struct{}

func (thirdPartyNotices) Check(f Facts) []Finding {
	if !f.Tag.Module.ShipsBinary() || f.ThirdPartyNotices {
		return nil
	}
	return []Finding{{RuleNotices, ScopeAlways, ThirdPartyNoticesFile + " is missing from the repository root: every archive ships it"}}
}

type noticeHolder struct{}

func (noticeHolder) Check(f Facts) []Finding {
	switch {
	case !f.Notice.Present:
		return []Finding{{RuleNoticeHolder, ScopeRelease, NoticeFile + " is missing: a release names its copyright holder in it"}}
	case strings.Contains(f.Notice.Text, NoticeHolderPlaceholder):
		return []Finding{{RuleNoticeHolder, ScopeRelease, fmt.Sprintf("%s still holds the placeholder %s: name the copyright holder", NoticeFile, NoticeHolderPlaceholder)}}
	}
	return nil
}
