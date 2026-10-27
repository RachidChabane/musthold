package freeze

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/config"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/plan"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/product"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// ShallnotRepository is the public repository whose releases are searched for the
// pin.
const ShallnotRepository = "RachidChabane/shallnot"

// The programs of the study's image, by absolute path since the box runs them
// under `env -i`. Claude Code is the image's, never the product's.
const (
	ClaudeProgram = "/usr/local/bin/claude"
	pythonProgram = "/usr/local/bin/python3"
	pytestProgram = "/usr/local/bin/pytest"
	uvProgram     = "/usr/local/bin/uv"
	nodeProgram   = "/usr/local/bin/node"
	gitProgram    = "/usr/bin/git"
	sha256sumProg = "/usr/bin/sha256sum"
)

// ImageToolchains maps each toolchain of the inputs freeze that the image holds
// to its program, which prints its version given VersionFlag.
func ImageToolchains() map[string]string {
	return map[string]string{
		ToolchainPython: pythonProgram, ToolchainPytest: pytestProgram, ToolchainUV: uvProgram,
		ToolchainNode: nodeProgram, ToolchainGit: gitProgram,
	}
}

// VersionFlag makes a program print its version.
const VersionFlag = "--version"

const (
	boxLimit       = 2 * time.Minute
	jsonIndent     = "  "
	outputFileMode = 0o644
	outputDirMode  = 0o755
)

// Errors of a resolve that cannot complete.
var (
	// ErrDirtyTree is returned when a tracked file differs from HEAD: the inputs
	// commit would not hold what the digests describe.
	ErrDirtyTree = errors.New("a tracked file differs from HEAD")
	// ErrProduct is returned for a product the inputs cannot describe.
	ErrProduct = errors.New("the product does not match the repository")
	// ErrPin is returned for a version or pin that PREREG §2 does not allow.
	ErrPin = errors.New("a pin differs from the protocol")
	// ErrBoxCommand is returned when a command in the study's image fails.
	ErrBoxCommand = errors.New("a command in the image failed")
	// ErrTask is returned for a task whose description and configuration disagree.
	ErrTask = errors.New("a task is inconsistent")
)

var goImageVersion = regexp.MustCompile(`^golang:([0-9]+(\.[0-9]+)+)`)

// Resolver computes the inputs freeze (PREREG §13.4).
type Resolver struct {
	// Root is the repository root; ConfigPath the configuration's path from it.
	Root       string
	ConfigPath string
	Config     config.Config
	Work       study.Layout
	// ProductDir is the host's product directory and ProductMount where the
	// study's containers see it.
	ProductDir   string
	ProductMount string

	Repo         Repository
	Box          box.Runner
	Engine       Engine
	Host         Host
	Releases     Releases
	Canary       CanarySource
	Construction ConstructionSource
	Preparer     Preparer
}

// Result is the inputs and the two files resolve writes.
type Result struct {
	Inputs study.Inputs
	// Files maps each repository path resolve writes to its content.
	Files map[string][]byte
}

// Resolve computes every key of inputs.json and the sums file.
//
// commits.inputs is HEAD: the tag loop-1-inputs is set afterwards, on the commit
// that adds inputs.json and its sums, so the tagged commit's parent is
// commits.inputs. commits.between lists the commits after the pre-registration's,
// up to HEAD included, that touch the product directories.
func (r Resolver) Resolve(ctx context.Context) (Result, error) {
	if err := r.requireCleanTree(ctx); err != nil {
		return Result{}, err
	}
	manifest, err := r.manifest(ctx)
	if err != nil {
		return Result{}, err
	}
	inputs := study.Inputs{Order: r.order()}
	inputs.Binaries, inputs.Images = describeProduct(manifest)
	steps := []func() error{
		func() (err error) { inputs.Commits, err = r.commits(ctx); return },
		func() (err error) { inputs.Toolchains, err = r.toolchains(ctx, manifest); return },
		func() (err error) { inputs.Shallnot, err = r.shallnot(ctx, manifest); return },
		func() (err error) { inputs.ClaudeCode, err = r.claudeCode(ctx); return },
		func() (err error) { return r.construction(ctx, &inputs) },
		func() (err error) { return r.canary(ctx, &inputs) },
		func() (err error) { inputs.Policy, err = ReadPolicy(ctx, r.Repo, inputs.Commits.Inputs); return },
		func() (err error) { inputs.Tasks, err = r.tasks(ctx, manifest); return },
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return Result{}, err
		}
	}
	return r.Render(ctx, inputs)
}

func (r Resolver) requireCleanTree(ctx context.Context) error {
	modified, err := r.Repo.Modified(ctx)
	if err != nil {
		return err
	}
	modified = slices.DeleteFunc(modified, func(file string) bool { return file == study.InputsPath || file == InputsSumsFile })
	if len(modified) > 0 {
		return fmt.Errorf("%w: %s", ErrDirtyTree, strings.Join(modified, ", "))
	}
	return nil
}

func (r Resolver) manifest(ctx context.Context) (product.Manifest, error) {
	manifest, err := loadManifest(r.ProductDir)
	if err != nil {
		return product.Manifest{}, err
	}
	head, err := r.Repo.Head(ctx)
	if err != nil {
		return product.Manifest{}, err
	}
	pin, err := r.readPin(ShallnotPinFile)
	if err != nil {
		return product.Manifest{}, err
	}
	switch {
	case manifest.Dirty:
		return product.Manifest{}, fmt.Errorf("%w: it was built from a modified working tree", ErrProduct)
	case manifest.Commit != head:
		return product.Manifest{}, fmt.Errorf("%w: it was built from %s, HEAD is %s", ErrProduct, manifest.Commit, head)
	case manifest.ShallnotPin != pin:
		return product.Manifest{}, fmt.Errorf("%w: it embeds Shallnot %s, %s holds %s", ErrProduct, manifest.ShallnotPin, ShallnotPinFile, pin)
	}
	return manifest, nil
}

func (r Resolver) readPin(file string) (string, error) {
	data, err := os.ReadFile(filepath.Join(r.Root, filepath.FromSlash(file)))
	return strings.TrimSpace(string(data)), err
}

func (r Resolver) commits(ctx context.Context) (study.Commits, error) {
	prereg, err := r.Repo.PeelTag(ctx, PreregTag)
	if err != nil {
		return study.Commits{}, err
	}
	head, err := r.Repo.Head(ctx)
	if err != nil {
		return study.Commits{}, err
	}
	between, err := r.Repo.Between(ctx, prereg, head, ProductDirs)
	if err != nil {
		return study.Commits{}, err
	}
	return study.Commits{Prereg: prereg, Inputs: head, Between: nonNil(between)}, nil
}

// describeProduct maps each binary to its SHA-256 and each plugin tree, under its
// path prefixed with PluginKeyPrefix, to its tree digest.
func describeProduct(manifest product.Manifest) (map[string]string, study.Images) {
	binaries := maps.Clone(manifest.Binaries)
	for plugin, digest := range manifest.Plugins {
		binaries[PluginKeyPrefix+plugin] = digest
	}
	return binaries, DescribeImages(manifest)
}

// DescribeImages are the images of a product's manifest as the inputs freeze
// records them: the built image also runs the egress proxy.
func DescribeImages(manifest product.Manifest) study.Images {
	return study.Images{Build: manifest.Images.Build, Base: manifest.Images.Base, UV: manifest.Images.UV, Built: manifest.Images.Built, Proxy: manifest.Images.Built}
}

func (r Resolver) inBox(ctx context.Context, argv ...string) (string, error) {
	result, err := r.Box.Run(ctx, box.Command{
		Argv:   argv,
		Env:    []string{box.SystemPath},
		Dir:    r.Work.Root,
		Mounts: []box.Mount{{Path: r.Work.Root, ReadOnly: true}},
		Limit:  boxLimit,
	})
	if err != nil {
		return "", err
	}
	if result.ExitCode != 0 || result.TimedOut {
		return "", fmt.Errorf("%w: %s (exit %d): %s", ErrBoxCommand, strings.Join(argv, " "), result.ExitCode, strings.TrimSpace(string(result.Stderr)))
	}
	return strings.TrimSpace(string(result.Stdout)), nil
}

func (r Resolver) toolchains(ctx context.Context, manifest product.Manifest) (map[string]string, error) {
	versions := map[string]string{}
	for key, program := range ImageToolchains() {
		line, err := r.inBox(ctx, program, VersionFlag)
		if err != nil {
			return nil, err
		}
		if versions[key], err = VersionOf(line); err != nil {
			return nil, err
		}
	}
	match := goImageVersion.FindStringSubmatch(manifest.Images.Build)
	if match == nil {
		return nil, fmt.Errorf("%w: the build image %q names no Go version", ErrProduct, manifest.Images.Build)
	}
	versions[ToolchainGo] = match[1]
	if err := r.engineVersions(ctx, versions); err != nil {
		return nil, err
	}
	return versions, nil
}

func (r Resolver) engineVersions(ctx context.Context, versions map[string]string) error {
	docker, err := r.Engine.DockerVersion(ctx)
	if err != nil {
		return err
	}
	colima, err := r.Engine.ColimaVersion(ctx)
	if err != nil {
		return err
	}
	settings, err := r.Engine.Settings(ctx, ColimaProfile)
	if err != nil {
		return err
	}
	host, err := r.Host.Info(ctx)
	if err != nil {
		return err
	}
	maps.Copy(versions, map[string]string{
		ToolchainDocker: docker, ToolchainColima: colima,
		ToolchainColimaC: settings.CPUs, ToolchainColimaM: settings.MemoryBytes, ToolchainColimaD: settings.DiskBytes,
		ToolchainHostOS: host.OS, ToolchainHostVer: host.Version, ToolchainHostArch: host.Arch,
	})
	return nil
}

func (r Resolver) shallnot(ctx context.Context, manifest product.Manifest) (study.Shallnot, error) {
	binary := plan.Layout{Product: r.ProductMount}.Bin(plan.ShallnotBinary)
	version, err := r.inBox(ctx, binary, VersionFlag)
	if err != nil {
		return study.Shallnot{}, err
	}
	release, err := FirstRelease(ctx, r.Releases, manifest.ShallnotPin)
	if err != nil {
		return study.Shallnot{}, err
	}
	return study.Shallnot{Pin: manifest.ShallnotPin, Version: version, Release: release}, nil
}

func (r Resolver) claudeCode(ctx context.Context) (study.ClaudeCode, error) {
	line, err := r.inBox(ctx, ClaudeProgram, VersionFlag)
	if err != nil {
		return study.ClaudeCode{}, err
	}
	version, err := VersionOf(line)
	if err != nil {
		return study.ClaudeCode{}, err
	}
	pin, err := r.readPin(ClaudeCodePinFile)
	if err != nil {
		return study.ClaudeCode{}, err
	}
	if version != ExpectedClaudeVersion || pin != ExpectedClaudeVersion {
		return study.ClaudeCode{}, fmt.Errorf("%w: Claude Code %s in the image, %s in %s, %s in the protocol", ErrPin, version, pin, ClaudeCodePinFile, ExpectedClaudeVersion)
	}
	sums, err := r.inBox(ctx, sha256sumProg, ClaudeProgram)
	if err != nil {
		return study.ClaudeCode{}, err
	}
	digest, _, _ := strings.Cut(sums, " ")
	if !digestPattern.MatchString(digest) {
		return study.ClaudeCode{}, fmt.Errorf("%w: sha256sum printed %q", ErrUnreadable, sums)
	}
	return study.ClaudeCode{Version: version, SHA256: digest}, nil
}

func (r Resolver) construction(ctx context.Context, inputs *study.Inputs) error {
	log, err := r.Construction.Records(ctx)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(log)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	inputs.Codex = []string{}
	for _, call := range log.Calls {
		if !seen[call.CodexVersion] {
			seen[call.CodexVersion] = true
			inputs.Codex = append(inputs.Codex, call.CodexVersion)
		}
	}
	slices.Sort(inputs.Codex)
	inputs.Construction = raw
	return nil
}

func (r Resolver) canary(ctx context.Context, inputs *study.Inputs) error {
	output, err := r.Canary.Output(ctx)
	if err != nil {
		return err
	}
	inputs.CLIAuxiliaryModels = nonNil(output.CLIAuxiliaryModels)
	inputs.EgressObserved = nonNil(output.EgressObserved)
	inputs.EgressAllowlist = nonNil(output.EgressAllowlist)
	inputs.QuotaPatterns = nonNil(output.QuotaPatterns)
	inputs.Canary = output.Records
	if inputs.Canary == nil {
		inputs.Canary = []study.CanaryRecord{}
	}
	return nil
}

func (r Resolver) order() []string {
	var names []string
	for _, run := range r.Config.Runs() {
		names = append(names, run.Name())
	}
	return names
}

func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func digestOf(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// Render marshals the inputs and computes the sums of the files they close.
func (r Resolver) Render(ctx context.Context, inputs study.Inputs) (Result, error) {
	document, err := json.MarshalIndent(inputs, "", jsonIndent)
	if err != nil {
		return Result{}, err
	}
	document = append(document, '\n')
	sums, err := r.sums(ctx, document)
	if err != nil {
		return Result{}, err
	}
	return Result{Inputs: inputs, Files: map[string][]byte{study.InputsPath: document, InputsSumsFile: []byte(FormatSums(sums))}}, nil
}

// sums lists the SHA-256 of every file of every task directory and fixture, of
// the configuration, of the image directory, and of inputs.json.
func (r Resolver) sums(ctx context.Context, inputsDocument []byte) ([]Sum, error) {
	roots := []string{r.ConfigPath, ImageDir}
	for _, task := range r.Config.Tasks {
		roots = append(roots, path.Join(study.TasksDir, task.Name), task.Fixture)
	}
	listed := map[string]bool{}
	var sums []Sum
	for _, root := range roots {
		files, err := r.Repo.Tracked(ctx, root)
		if err != nil {
			return nil, err
		}
		for _, file := range files {
			if listed[file] {
				continue
			}
			listed[file] = true
			content, err := os.ReadFile(filepath.Join(r.Root, filepath.FromSlash(file)))
			if err != nil {
				return nil, err
			}
			sums = append(sums, Sum{Digest: digestOf(content), Path: file})
		}
	}
	return append(sums, Sum{Digest: digestOf(inputsDocument), Path: study.InputsPath}), nil
}

// Write stores the files of a result under the repository root.
func (r Resolver) Write(result Result) error {
	for file, content := range result.Files {
		target := filepath.Join(r.Root, filepath.FromSlash(file))
		if err := os.MkdirAll(filepath.Dir(target), outputDirMode); err != nil {
			return err
		}
		if err := os.WriteFile(target, content, outputFileMode); err != nil {
			return err
		}
	}
	return nil
}
