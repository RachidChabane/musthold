package freeze

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/config"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/construct/records"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/deviation"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/product"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// The conditions of PREREG §13.5, as a failure names them.
const (
	CondTags         = "tags"
	CondCommitFiles  = "commit-files"
	CondImprints     = "imprints"
	CondInputsCommit = "inputs-commit"
	CondOrder        = "order"
	CondSums         = "sums"
	CondPins         = "pins"
	CondPolicy       = "policy"
	CondInstructions = "instructions"
	CondBinaries     = "binaries"
	CondImage        = "image"
)

// Failure is one condition that does not hold.
type Failure struct {
	Condition string
	Detail    string
}

func (f Failure) String() string { return f.Condition + ": " + f.Detail }

// Checker verifies the freeze (PREREG §13.5, with the amendment's tag).
type Checker struct {
	// Root is the repository root.
	Root       string
	ProductDir string

	Repo   Repository
	Tokens deviation.TokenReader
	Runs   Runs
	Engine Engine
}

// Check evaluates every condition and returns every failure.
func (c Checker) Check(ctx context.Context) []Failure {
	var failures []Failure
	report := func(condition string, details ...string) {
		for _, detail := range details {
			failures = append(failures, Failure{condition, detail})
		}
	}
	times := c.freezes(ctx, report)
	inputs, err := c.inputs(ctx)
	if err != nil {
		report(CondInputsCommit, err.Error())
		failures = append(failures, c.sumsFailures(ctx, nil)...)
		return failures
	}
	report(CondInputsCommit, c.inputsCommitFailures(ctx, inputs)...)
	report(CondOrder, c.orderFailures(ctx, inputs, times)...)
	failures = append(failures, c.sumsFailures(ctx, &inputs)...)
	report(CondPins, c.pinFailures(ctx, inputs)...)
	report(CondPolicy, c.policyFailures(ctx, inputs)...)
	for _, task := range inputs.Tasks {
		report(CondInstructions, InstructionFailures(task)...)
	}
	failures = append(failures, c.productFailures(ctx, inputs)...)
	return failures
}

// freezes checks each tag, its commit file and its token, and returns the
// generation times of the tokens that can be read.
func (c Checker) freezes(ctx context.Context, report func(string, ...string)) map[string]time.Time {
	times := map[string]time.Time{}
	for _, freeze := range Freezes() {
		commit, err := c.Repo.PeelTag(ctx, freeze.Tag)
		if err != nil {
			report(CondTags, err.Error())
			continue
		}
		file := filepath.Join(c.Root, filepath.FromSlash(freeze.CommitFile()))
		content, err := os.ReadFile(file)
		switch {
		case err != nil:
			report(CondCommitFiles, err.Error())
			continue
		case strings.TrimSpace(string(content)) != commit:
			report(CondCommitFiles, fmt.Sprintf("%s holds %q, %s peels to %s", freeze.CommitFile(), strings.TrimSpace(string(content)), freeze.Tag, commit))
		}
		token, err := c.Tokens.Read(ctx, deviation.ReplyPath(file))
		switch {
		case err != nil:
			report(CondImprints, fmt.Sprintf("%s: %v", freeze.Tag, err))
			continue
		case token.Imprint != deviation.Imprint(content):
			report(CondImprints, fmt.Sprintf("the token of %s stamps %s, the SHA-256 of %s is %s", freeze.Tag, token.Imprint, freeze.CommitFile(), deviation.Imprint(content)))
			continue
		}
		times[freeze.Tag] = token.GenTime
	}
	return times
}

func (c Checker) inputs(ctx context.Context) (study.Inputs, error) {
	data, err := c.Repo.Show(ctx, InputsTag, study.InputsPath)
	if err != nil {
		return study.Inputs{}, err
	}
	return study.ParseInputs(data)
}

// inputsCommitFailures checks that the tag loop-1-inputs sits on the commit that
// adds inputs.json and its sums, on top of the commit commits.inputs names, and
// descends from the pre-registration's.
func (c Checker) inputsCommitFailures(ctx context.Context, inputs study.Inputs) []string {
	var failures []string
	tagged, err := c.Repo.PeelTag(ctx, InputsTag)
	if err != nil {
		return []string{err.Error()}
	}
	if parent, err := c.Repo.Parent(ctx, tagged); err != nil || parent != inputs.Commits.Inputs {
		failures = append(failures, fmt.Sprintf("the parent of %s is %q, commits.inputs is %s", InputsTag, parent, inputs.Commits.Inputs))
	}
	if changed, err := c.Repo.Changed(ctx, tagged); err != nil || !slices.Equal(slices.Sorted(slices.Values(changed)), slices.Sorted(slices.Values([]string{study.InputsPath, InputsSumsFile}))) {
		failures = append(failures, fmt.Sprintf("%s changes %v, not the inputs and their sums alone", InputsTag, changed))
	}
	prereg, err := c.Repo.PeelTag(ctx, PreregTag)
	if err != nil {
		return append(failures, err.Error())
	}
	if prereg != inputs.Commits.Prereg {
		failures = append(failures, fmt.Sprintf("commits.prereg is %s, %s peels to %s", inputs.Commits.Prereg, PreregTag, prereg))
	}
	if descends, err := c.Repo.IsAncestor(ctx, prereg, tagged); err != nil || !descends {
		failures = append(failures, fmt.Sprintf("%s does not descend from %s", InputsTag, PreregTag))
	}
	return failures
}

func (c Checker) orderFailures(ctx context.Context, inputs study.Inputs, times map[string]time.Time) []string {
	log, err := records.Decode(inputs.Construction)
	if err != nil {
		return []string{err.Error()}
	}
	starts, err := c.Runs.Starts(ctx)
	if err != nil {
		return []string{err.Error()}
	}
	return OrderFailures(Timeline{
		Prereg: times[PreregTag], Amendment: times[AmendmentTag], Inputs: times[InputsTag],
		Calls: log.Calls, Canary: inputs.Canary, Runs: starts,
	})
}

// sumsFailures checks the two sums files against the files at their tags. The
// inputs' sums need the inputs for the configuration they were computed from.
func (c Checker) sumsFailures(ctx context.Context, inputs *study.Inputs) []Failure {
	var failures []Failure
	report := func(details ...string) {
		for _, detail := range details {
			failures = append(failures, Failure{CondSums, detail})
		}
	}
	report(c.sumsAt(ctx, PreregTag, PreregSumsFile, c.preregExpected)...)
	if inputs != nil {
		report(c.sumsAt(ctx, InputsTag, InputsSumsFile, c.inputsExpected)...)
	}
	return failures
}

// sumsAt checks that a sums file at a tag lists exactly the expected files, and
// that each listed digest is that of the file at the tag.
func (c Checker) sumsAt(ctx context.Context, tag, sumsFile string, expected func(context.Context) ([]string, error)) []string {
	data, err := c.Repo.Show(ctx, tag, sumsFile)
	if err != nil {
		return []string{err.Error()}
	}
	sums, err := ParseSums(string(data))
	if err != nil {
		return []string{fmt.Sprintf("%s: %v", sumsFile, err)}
	}
	var failures []string
	listed := map[string]bool{}
	for _, sum := range sums {
		listed[sum.Path] = true
		content, err := c.Repo.Show(ctx, tag, sum.Path)
		switch {
		case err != nil:
			failures = append(failures, fmt.Sprintf("%s lists %s, which %s does not hold", sumsFile, sum.Path, tag))
		case digestOf(content) != sum.Digest:
			failures = append(failures, fmt.Sprintf("%s lists %s with %s, its SHA-256 at %s is %s", sumsFile, sum.Path, sum.Digest, tag, digestOf(content)))
		}
	}
	files, err := expected(ctx)
	if err != nil {
		return append(failures, err.Error())
	}
	for _, file := range files {
		if !listed[file] {
			failures = append(failures, fmt.Sprintf("%s does not list %s", sumsFile, file))
		}
	}
	for file := range listed {
		if !slices.Contains(files, file) {
			failures = append(failures, fmt.Sprintf("%s lists %s, which it should not", sumsFile, file))
		}
	}
	return failures
}

// preregExpected are the files of the commit of the pre-registration tag under
// the roots of PREREG §13.1, except the freeze directory.
func (c Checker) preregExpected(ctx context.Context) ([]string, error) {
	files, err := c.Repo.TreeFiles(ctx, PreregTag, PreregRoots...)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(files, func(file string) bool { return strings.HasPrefix(file, PreregExcluded) }), nil
}

// inputsExpected are the files resolve lists, at the inputs tag.
func (c Checker) inputsExpected(ctx context.Context) ([]string, error) {
	data, err := c.Repo.Show(ctx, InputsTag, study.ConfigFile)
	if err != nil {
		return nil, err
	}
	configured, err := config.Parse(data)
	if err != nil {
		return nil, err
	}
	roots := []string{study.ConfigFile, ImageDir}
	for _, task := range configured.Tasks {
		roots = append(roots, path.Join(study.TasksDir, task.Name), task.Fixture)
	}
	files := []string{study.InputsPath}
	for _, root := range roots {
		listed, err := c.Repo.TreeFiles(ctx, InputsTag, root)
		if err != nil {
			return nil, err
		}
		files = append(files, listed...)
	}
	slices.Sort(files)
	return slices.Compact(files), nil
}

func (c Checker) pinFailures(ctx context.Context, inputs study.Inputs) []string {
	var files PinFiles
	for target, file := range map[*string]string{
		&files.Shallnot: ShallnotPinFile, &files.ClaudeCode: ClaudeCodePinFile, &files.RequirementsLock: path.Join(ImageDir, "requirements.lock"),
	} {
		content, err := c.Repo.Show(ctx, InputsTag, file)
		if err != nil {
			return []string{err.Error()}
		}
		*target = string(content)
	}
	return PinFailures(inputs, files)
}

// policyFailures checks the policy of the inputs against the protocol's table, and
// against the defaults the source declares at the product commit.
func (c Checker) policyFailures(ctx context.Context, inputs study.Inputs) []string {
	failures := PolicyFailures("inputs.json", inputs.Policy, ExpectedPolicy)
	source, err := ReadPolicy(ctx, c.Repo, inputs.Commits.Inputs)
	if err != nil {
		return append(failures, err.Error())
	}
	return append(failures, PolicyFailures("the source at "+inputs.Commits.Inputs, source, ExpectedPolicy)...)
}

// productFailures compares the binaries, the plugin trees and the image about to
// run with the inputs.
func (c Checker) productFailures(ctx context.Context, inputs study.Inputs) []Failure {
	var failures []Failure
	report := func(condition, detail string) { failures = append(failures, Failure{condition, detail}) }
	digests, err := product.Describe(c.ProductDir)
	if err != nil {
		report(CondBinaries, err.Error())
	} else {
		for _, detail := range binaryDifferences(inputs.Binaries, digests) {
			report(CondBinaries, detail)
		}
	}
	data, err := os.ReadFile(filepath.Join(c.ProductDir, product.ManifestFile))
	if err != nil {
		report(CondImage, err.Error())
		return failures
	}
	manifest, err := product.Parse(data)
	if err != nil {
		report(CondImage, err.Error())
		return failures
	}
	for _, detail := range imageDifferences(inputs.Images, manifest.Images) {
		report(CondImage, detail)
	}
	if id, err := c.Engine.ImageID(ctx, inputs.Images.Built); err != nil || id != inputs.Images.Built {
		report(CondImage, fmt.Sprintf("the engine holds image %q for %s (%v)", id, inputs.Images.Built, err))
	}
	return failures
}

func binaryDifferences(recorded map[string]string, found product.Digests) []string {
	actual := map[string]string{}
	for name, digest := range found.Binaries {
		actual[name] = digest
	}
	for name, digest := range found.Plugins {
		actual[PluginKeyPrefix+name] = digest
	}
	var details []string
	for _, name := range differingKeys(recorded, actual) {
		details = append(details, fmt.Sprintf("%s is %q in inputs.json and %q in the product", name, recorded[name], actual[name]))
	}
	return details
}

func imageDifferences(recorded study.Images, manifest product.Images) []string {
	var details []string
	for name, pair := range map[string][2]string{
		"build": {recorded.Build, manifest.Build}, "base": {recorded.Base, manifest.Base},
		"uv": {recorded.UV, manifest.UV}, "built": {recorded.Built, manifest.Built},
	} {
		if pair[0] != pair[1] {
			details = append(details, fmt.Sprintf("image %s is %q in inputs.json and %q in the product manifest", name, pair[0], pair[1]))
		}
	}
	slices.Sort(details)
	return details
}
