// Package cli is the command line of musthold-release: it parses a verb and its
// flags, wires the use cases to their adapters and prints what they found.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/tools/release/internal/app"
	"github.com/RachidChabane/musthold/tools/release/internal/domain"
)

// Exit codes of the command.
const (
	ExitClean       = 0
	ExitFailure     = 1
	ExitUsage       = 2
	ExitRefused     = 3
	ExitDistInvalid = 4
)

// Binary is the name of the command.
const Binary = "musthold-release"

// Verbs of the command.
const (
	VerbPlan       = "plan"
	VerbPreflight  = "preflight"
	VerbBuild      = "build"
	VerbVerifyDist = "verify-dist"
	VerbFormula    = "formula"
	VerbNotes      = "notes"
)

const (
	defaultRepository = "."
	defaultDist       = "dist"
	targetsSeparator  = ","
	visibilityPublic  = "public"
	visibilityPrivate = "private"
)

// Services are the ports the verbs run against.
type Services struct {
	Repository app.RepositoryReader
	History    app.HistoryReader
	Files      app.ModuleFiles
	Checks     app.Checks
	Compiler   app.Compiler
	Archiver   app.Archiver
	Inspector  app.BinaryInspector
	Disk       app.Files
	Dist       app.DistReader
}

// Wiring builds the services for the repository rooted at a directory.
type Wiring func(root string) Services

// verbs lists the verbs in the order the usage shows them.
var verbs = []string{VerbPlan, VerbPreflight, VerbBuild, VerbVerifyDist, VerbFormula, VerbNotes}

type options struct {
	repository    string
	dist          string
	targets       string
	jobs          int
	out           string
	dryRun        bool
	skipTests     bool
	requirePublic bool
	visibility    string
}

// Run runs a command line and returns its exit code.
func Run(wiring Wiring, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage())
		return ExitUsage
	}
	verb, rest := args[0], args[1:]
	if verb == "help" || verb == "-h" || verb == "--help" {
		fmt.Fprint(stdout, usage())
		return ExitClean
	}
	if !slices.Contains(verbs, verb) {
		fmt.Fprintf(stderr, "%s: unknown verb %q\n%s", Binary, verb, usage())
		return ExitUsage
	}
	var opts options
	flags := flag.NewFlagSet(Binary+" "+verb, flag.ContinueOnError)
	flags.SetOutput(stderr)
	declareFlags(flags, verb, &opts)
	if err := flags.Parse(rest); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return ExitClean
		}
		return ExitUsage
	}
	if flags.NArg() != 1 {
		fmt.Fprintf(stderr, "%s %s needs one <tag>, for example discpower/v0.2.0\n", Binary, verb)
		return ExitUsage
	}
	tag, err := domain.ParseTag(flags.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", Binary, err)
		return ExitUsage
	}
	return dispatch(verb, wiring, tag, opts, stdout, stderr)
}

func dispatch(verb string, wiring Wiring, tag domain.Tag, opts options, stdout, stderr io.Writer) int {
	if verb == VerbPlan {
		return plan(tag, stdout)
	}
	root, err := filepath.Abs(opts.repository)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", Binary, err)
		return ExitFailure
	}
	services := wiring(root)
	switch verb {
	case VerbPreflight:
		return preflight(services, tag, opts, stdout, stderr)
	case VerbBuild:
		return build(services, root, tag, opts, stdout, stderr)
	case VerbVerifyDist:
		return verifyDist(services, root, tag, opts, stdout, stderr)
	case VerbFormula:
		return formula(services, root, tag, opts, stdout, stderr)
	default:
		return notes(services, root, tag, opts, stderr)
	}
}

func declareFlags(flags *flag.FlagSet, verb string, opts *options) {
	if verb == VerbPlan {
		return
	}
	flags.StringVar(&opts.repository, "repo", defaultRepository, "the repository root")
	switch verb {
	case VerbPreflight:
		flags.BoolVar(&opts.dryRun, "dry-run", false, "report the findings only a real release needs and go on")
		flags.BoolVar(&opts.skipTests, "skip-tests", false, "leave out the Go tests and the plugin regeneration")
		flags.BoolVar(&opts.requirePublic, "require-public", false, "refuse a repository that is not public")
		flags.StringVar(&opts.visibility, "visibility", "", "the repository visibility: public or private")
	case VerbBuild:
		flags.StringVar(&opts.dist, "dist", defaultDist, "the directory that receives the release files")
		flags.StringVar(&opts.targets, "targets", "", "comma-separated os/arch pairs (default: the whole matrix)")
		flags.IntVar(&opts.jobs, "jobs", app.DefaultJobs, "how many targets build at once")
	case VerbVerifyDist:
		flags.StringVar(&opts.dist, "dist", defaultDist, "the directory that holds the release files")
		flags.StringVar(&opts.targets, "targets", "", "comma-separated os/arch pairs (default: the whole matrix)")
	case VerbFormula:
		flags.StringVar(&opts.dist, "dist", defaultDist, "the directory that holds the release files")
	case VerbNotes:
		flags.StringVar(&opts.out, "out", "notes.md", "the file that receives the notes")
	}
}

func plan(tag domain.Tag, stdout io.Writer) int {
	for _, output := range domain.PlanOutputs(tag) {
		fmt.Fprintf(stdout, "%s=%s\n", output.Key, output.Value)
	}
	return ExitClean
}

func preflight(services Services, tag domain.Tag, opts options, stdout, stderr io.Writer) int {
	visibility, err := parseVisibility(opts.visibility)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", Binary, err)
		return ExitUsage
	}
	report, err := app.Preflight{Repository: services.Repository, Files: services.Files, Checks: services.Checks}.
		Run(tag, app.PreflightOptions{SkipTests: opts.skipTests, RequirePublic: opts.requirePublic, Visibility: visibility})
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", Binary, err)
		return ExitFailure
	}
	refusals := report.Refusals(opts.dryRun)
	deferred := len(report.Findings) - len(refusals)
	for _, finding := range refusals {
		fmt.Fprintf(stdout, "refused: %s\n", finding)
	}
	for _, finding := range report.Findings {
		if opts.dryRun && finding.Scope == domain.ScopeRelease {
			fmt.Fprintf(stdout, "a real release would be refused: %s\n", finding)
		}
	}
	if len(refusals) > 0 {
		fmt.Fprintf(stdout, "preflight %s: %d finding(s) stop the run\n", tag, len(refusals))
		return ExitRefused
	}
	fmt.Fprintf(stdout, "preflight %s: ok (%d finding(s) a real release would refuse)\n", tag, deferred)
	return ExitClean
}

func build(services Services, root string, tag domain.Tag, opts options, stdout, stderr io.Writer) int {
	targets, err := parseTargets(opts.targets)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", Binary, err)
		return ExitUsage
	}
	commit, commitTime, err := headCommit(services)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", Binary, err)
		return ExitFailure
	}
	sums, err := app.Builder{Compiler: services.Compiler, Archiver: services.Archiver, Inspector: services.Inspector, Files: services.Disk}.
		Build(app.BuildRequest{
			Tag: tag, Commit: commit, CommitTime: commitTime, Targets: targets,
			Jobs: opts.jobs, DistDir: absolute(root, opts.dist),
		})
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", Binary, err)
		return ExitFailure
	}
	fmt.Fprintf(stdout, "built %d file(s) for %s into %s\n", len(sums), tag, absolute(root, opts.dist))
	return ExitClean
}

func verifyDist(services Services, root string, tag domain.Tag, opts options, stdout, stderr io.Writer) int {
	targets, err := parseTargets(opts.targets)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", Binary, err)
		return ExitUsage
	}
	commit, commitTime, err := headCommit(services)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", Binary, err)
		return ExitFailure
	}
	findings, err := app.DistVerifier{Reader: services.Dist}.Verify(absolute(root, opts.dist), tag, commit, commitTime, targets)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", Binary, err)
		return ExitFailure
	}
	for _, finding := range findings {
		fmt.Fprintf(stdout, "invalid: %s\n", finding)
	}
	if len(findings) > 0 {
		fmt.Fprintf(stdout, "dist %s: %d finding(s)\n", tag, len(findings))
		return ExitDistInvalid
	}
	fmt.Fprintf(stdout, "dist %s: ok\n", tag)
	return ExitClean
}

func formula(services Services, root string, tag domain.Tag, opts options, stdout, stderr io.Writer) int {
	written, err := app.FormulaWriter{Files: services.Disk}.Write(absolute(root, opts.dist), tag)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", Binary, err)
		return ExitFailure
	}
	fmt.Fprintf(stdout, "wrote %s\n", written)
	return ExitClean
}

func notes(services Services, root string, tag domain.Tag, opts options, stderr io.Writer) int {
	if err := (app.NotesWriter{History: services.History, Files: services.Disk}).Write(absolute(root, opts.out), tag); err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", Binary, err)
		return ExitFailure
	}
	return ExitClean
}

func headCommit(services Services) (string, int64, error) {
	commit, err := services.Repository.Head()
	if err != nil {
		return "", 0, err
	}
	instant, err := services.Repository.CommitTime(commit)
	if err != nil {
		return "", 0, err
	}
	return commit, instant.Unix(), nil
}

func absolute(root, name string) string {
	if filepath.IsAbs(name) {
		return name
	}
	return filepath.Join(root, name)
}

func parseTargets(text string) ([]domain.Target, error) {
	if text == "" {
		return domain.Targets(), nil
	}
	var targets []domain.Target
	for _, item := range strings.Split(text, targetsSeparator) {
		target, err := domain.ParseTarget(strings.TrimSpace(item))
		if err != nil {
			return nil, err
		}
		targets = append(targets, target)
	}
	return targets, nil
}

func parseVisibility(text string) (domain.Visibility, error) {
	switch text {
	case "":
		return domain.VisibilityUnknown, nil
	case visibilityPublic:
		return domain.VisibilityPublic, nil
	case visibilityPrivate:
		return domain.VisibilityPrivate, nil
	}
	return domain.VisibilityUnknown, fmt.Errorf("visibility %q is neither %s nor %s", text, visibilityPublic, visibilityPrivate)
}

func usage() string {
	return fmt.Sprintf("usage: %s <%s> [flags] <tag>\n", Binary, strings.Join(verbs, "|"))
}
