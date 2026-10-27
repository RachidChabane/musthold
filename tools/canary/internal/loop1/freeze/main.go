package freeze

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/config"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box/docker"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/deviation"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/product"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// Exit codes of the verbs.
const (
	ExitOK     = 0
	ExitFailed = 1
	ExitUsage  = 2
)

// Options are the flags of the two verbs.
type Options struct {
	Repo, Config, Work, Product string
}

func parseOptions(name string, args []string, stderr io.Writer) (Options, bool) {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(stderr)
	var chosen Options
	flags.StringVar(&chosen.Repo, "repo", ".", "repository root")
	flags.StringVar(&chosen.Config, "config", study.ConfigFile, "run configuration")
	flags.StringVar(&chosen.Work, "work", study.DefaultWork(), "work directory")
	flags.StringVar(&chosen.Product, "product", study.DefaultProduct(), "product directory")
	if flags.Parse(args) != nil || flags.NArg() != 0 {
		return Options{}, false
	}
	return chosen, true
}

// ResolveWiring is what resolve takes from the other verbs: the preparation of a
// task for an arm as a run does it, and the canary's output.
type ResolveWiring struct {
	Preparer Preparer
	Canary   CanarySource
}

// ResolveCommand is the resolve verb (PREREG §13.4). Wire builds its wiring from
// the verb's options and the box its commands run in.
type ResolveCommand struct {
	Wire func(options Options, runner box.Runner) (ResolveWiring, error)
}

// Main runs the resolve verb and returns its exit code.
func (c ResolveCommand) Main(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	chosen, ok := parseOptions("resolve", args, stderr)
	if !ok {
		return ExitUsage
	}
	resolver, err := c.newResolver(chosen)
	if err != nil {
		fmt.Fprintln(stderr, "resolve:", err)
		return ExitFailed
	}
	result, err := resolver.Resolve(ctx)
	if err == nil {
		err = resolver.Write(result)
	}
	if err != nil {
		fmt.Fprintln(stderr, "resolve:", err)
		return ExitFailed
	}
	fmt.Fprintf(stdout, "resolve: wrote %s and %s\n", study.InputsPath, InputsSumsFile)
	return ExitOK
}

func (c ResolveCommand) newResolver(chosen Options) (Resolver, error) {
	root, err := filepath.Abs(chosen.Repo)
	if err != nil {
		return Resolver{}, err
	}
	configPath, err := repositoryPath(root, chosen.Config)
	if err != nil {
		return Resolver{}, err
	}
	data, err := os.ReadFile(filepath.Join(root, configPath))
	if err != nil {
		return Resolver{}, err
	}
	configured, err := config.Parse(data)
	if err != nil {
		return Resolver{}, err
	}
	manifest, err := loadManifest(chosen.Product)
	if err != nil {
		return Resolver{}, err
	}
	runner, err := docker.StudyRunner(root, chosen.Product, manifest.Images.Built)
	if err != nil {
		return Resolver{}, err
	}
	wiring, err := c.Wire(chosen, runner)
	if err != nil {
		return Resolver{}, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return Resolver{}, err
	}
	layout := study.Layout{Root: chosen.Work}
	command := OSExec{}
	return Resolver{
		Root: root, ConfigPath: filepath.ToSlash(configPath), Config: configured, Work: layout, Home: home,
		ProductDir: chosen.Product, ProductMount: docker.ProductTarget,
		Repo: Git{Dir: root, Exec: command}, Box: runner,
		Engine:   CLIEngine{Exec: command, Docker: []string{filepath.Join(root, study.DockerClient)}},
		Host:     UnameHost{Exec: command},
		Releases: GitHubReleases{Exec: command, Repository: ShallnotRepository},
		Canary:   wiring.Canary, Construction: FileConstruction{Layout: layout},
		Preparer: wiring.Preparer,
	}, nil
}

func loadManifest(productDir string) (product.Manifest, error) {
	data, err := os.ReadFile(filepath.Join(productDir, product.ManifestFile))
	if err != nil {
		return product.Manifest{}, err
	}
	return product.Parse(data)
}

// repositoryPath is a path relative to the repository root, whether the flag gave
// it relative to the root or absolute.
func repositoryPath(root, file string) (string, error) {
	if !filepath.IsAbs(file) {
		return filepath.Clean(file), nil
	}
	return filepath.Rel(root, file)
}

// CheckMain is the freeze-check verb (PREREG §13.5).
func CheckMain(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	chosen, ok := parseOptions("freeze-check", args, stderr)
	if !ok {
		return ExitUsage
	}
	root, err := filepath.Abs(chosen.Repo)
	if err != nil {
		fmt.Fprintln(stderr, "freeze-check:", err)
		return ExitFailed
	}
	command := OSExec{}
	checker := Checker{
		Root: root, ProductDir: chosen.Product,
		Repo:   Git{Dir: root, Exec: command},
		Tokens: deviation.OpenSSL{},
		Runs:   FileRuns{Layout: study.Layout{Root: chosen.Work}},
		Engine: CLIEngine{Exec: command, Docker: []string{filepath.Join(root, study.DockerClient)}},
	}
	failures := checker.Check(ctx)
	for _, failure := range failures {
		fmt.Fprintln(stderr, "freeze-check: FAIL", failure)
	}
	if len(failures) > 0 {
		return ExitFailed
	}
	fmt.Fprintln(stdout, "freeze-check: ok")
	return ExitOK
}
