package freeze

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/config"
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

// MainFunc is the entry of a verb.
type MainFunc func(ctx context.Context, args []string, stdout, stderr io.Writer) int

const (
	dockerScript = "experiments/loop-1/script/docker"
	withSlot     = "script/with-slot"
	slotWait     = time.Hour
)

type options struct {
	repo, config, work, product string
}

func parseOptions(name string, args []string, stderr io.Writer) (options, bool) {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(stderr)
	var chosen options
	flags.StringVar(&chosen.repo, "repo", ".", "repository root")
	flags.StringVar(&chosen.config, "config", ConfigFile, "run configuration")
	flags.StringVar(&chosen.work, "work", study.DefaultWork(), "work directory")
	flags.StringVar(&chosen.product, "product", study.DefaultProduct(), "product directory")
	if flags.Parse(args) != nil || flags.NArg() != 0 {
		return options{}, false
	}
	return chosen, true
}

// ResolveMain is the resolve verb (PREREG §13.4). The preparer computes a task's
// start commit and equipped tree for an arm, as a run does.
func ResolveMain(preparer Preparer) MainFunc {
	return func(ctx context.Context, args []string, stdout, stderr io.Writer) int {
		chosen, ok := parseOptions("resolve", args, stderr)
		if !ok {
			return ExitUsage
		}
		resolver, err := newResolver(chosen, preparer)
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
		fmt.Fprintf(stdout, "resolve: wrote %s and %s\n", InputsFile, InputsSumsFile)
		return ExitOK
	}
}

func newResolver(chosen options, preparer Preparer) (Resolver, error) {
	root, err := filepath.Abs(chosen.repo)
	if err != nil {
		return Resolver{}, err
	}
	configPath, err := repositoryPath(root, chosen.config)
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
	manifest, err := loadManifest(chosen.product)
	if err != nil {
		return Resolver{}, err
	}
	client := []string{filepath.Join(root, dockerScript)}
	runner, err := docker.New(docker.Config{
		Client:     client,
		RunWrapper: docker.SlotWrapper(filepath.Join(root, withSlot), slotWait),
		Image:      manifest.Images.Built,
		Product:    chosen.product,
		UID:        os.Getuid(),
		GID:        os.Getgid(),
	})
	if err != nil {
		return Resolver{}, err
	}
	layout := study.Layout{Root: chosen.work}
	command := OSExec{}
	return Resolver{
		Root: root, ConfigPath: filepath.ToSlash(configPath), Config: configured, Work: layout,
		ProductDir: chosen.product, ProductMount: docker.ProductTarget,
		Repo: Git{Dir: root, Exec: command}, Box: runner,
		Engine:   CLIEngine{Exec: command, Docker: client},
		Host:     UnameHost{Exec: command},
		Releases: GitHubReleases{Exec: command, Repository: ShallnotRepository},
		Canary:   FileCanary{Layout: layout}, Construction: FileConstruction{Layout: layout},
		Preparer: preparer,
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
	root, err := filepath.Abs(chosen.repo)
	if err != nil {
		fmt.Fprintln(stderr, "freeze-check:", err)
		return ExitFailed
	}
	command := OSExec{}
	checker := Checker{
		Root: root, ProductDir: chosen.product,
		Repo:   Git{Dir: root, Exec: command},
		Tokens: deviation.OpenSSL{},
		Runs:   FileRuns{Layout: study.Layout{Root: chosen.work}},
		Engine: CLIEngine{Exec: command, Docker: []string{filepath.Join(root, dockerScript)}},
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
