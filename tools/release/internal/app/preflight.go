package app

import (
	"fmt"

	"github.com/RachidChabane/musthold/tools/release/internal/domain"
)

// PreflightOptions tunes the pre-flight.
type PreflightOptions struct {
	// SkipTests leaves out the steps that run the Go tests: the module's test suite
	// and the regeneration of the plugin files.
	SkipTests bool
	// RequirePublic adds the rule that the repository is public.
	RequirePublic bool
	Visibility    domain.Visibility
}

// Preflight collects the facts of a tag and runs the pre-flight rule set over them.
type Preflight struct {
	Repository RepositoryReader
	Files      ModuleFiles
	Checks     Checks
}

// Run collects the facts for a tag and evaluates the rules.
func (p Preflight) Run(tag domain.Tag, options PreflightOptions) (domain.Report, error) {
	facts, err := p.collect(tag, options)
	if err != nil {
		return domain.Report{}, err
	}
	return domain.Evaluate(domain.Rules(), facts), nil
}

func (p Preflight) collect(tag domain.Tag, options PreflightOptions) (domain.Facts, error) {
	module := tag.Module
	facts := domain.Facts{Tag: tag, RequirePublic: options.RequirePublic, Visibility: options.Visibility}
	var err error
	if facts.HeadCommit, err = p.Repository.Head(); err != nil {
		return facts, fmt.Errorf("read HEAD: %w", err)
	}
	if facts.TagCommit, _, err = p.Repository.TagCommit(tag.String()); err != nil {
		return facts, fmt.Errorf("read the tag %s: %w", tag, err)
	}
	if facts.DirtyPaths, err = p.Repository.DirtyPaths(); err != nil {
		return facts, fmt.Errorf("read the working tree: %w", err)
	}
	if facts.Tags, err = p.Repository.Tags(); err != nil {
		return facts, fmt.Errorf("list the tags: %w", err)
	}
	if facts.GoMod, err = p.Files.GoMod(module.Name); err != nil {
		return facts, fmt.Errorf("read the go.mod of %s: %w", module.Name, err)
	}
	if facts.Notice, err = p.Files.Notice(); err != nil {
		return facts, fmt.Errorf("read %s: %w", domain.NoticeFile, err)
	}
	if module.ShipsPlugin() {
		if facts.Manifests, err = p.Files.Manifests(module.PluginRoots); err != nil {
			return facts, fmt.Errorf("read the plugin manifests of %s: %w", module.Name, err)
		}
		if facts.Marketplace, err = p.Files.Marketplace(); err != nil {
			return facts, fmt.Errorf("read the marketplace: %w", err)
		}
	}
	if module.ShipsBinary() {
		if facts.ThirdPartyNotices, err = p.Files.Exists(domain.ThirdPartyNoticesFile); err != nil {
			return facts, fmt.Errorf("look for %s: %w", domain.ThirdPartyNoticesFile, err)
		}
	}
	facts.Build = p.Checks.Build(module.Name)
	if !options.SkipTests {
		facts.Test = p.Checks.Test(module.Name)
		if module.ShipsPlugin() {
			facts.PluginGen = p.Checks.PluginGenerated(module.Name)
		}
	}
	return facts, nil
}
