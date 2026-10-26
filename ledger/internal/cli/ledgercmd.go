package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/cliconv"
	"github.com/RachidChabane/musthold/ledger/internal/adapters/filetree"
	"github.com/RachidChabane/musthold/ledger/internal/adapters/gitrepo"
	"github.com/RachidChabane/musthold/ledger/internal/adapters/signing/inspect"
	"github.com/RachidChabane/musthold/ledger/internal/adapters/signing/trustroot"
	"github.com/RachidChabane/musthold/ledger/internal/app"
	"github.com/RachidChabane/musthold/ledger/internal/domain/ledger"
	"github.com/RachidChabane/musthold/ledger/internal/domain/refusal"
)

const defaultRepository = "."

// ledgerSubcommands are the subcommands of `ledger`, in the order the usage gives them.
var ledgerSubcommands = []string{appendSubcommand, exportSubcommand, auditSubcommand, seedSubcommand, fetchTrustSubcommand}

func runLedger(environment Environment, args []string, stdout, _ io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("%w: ledger: no subcommand", errUsage)
	}
	subcommand, rest := args[0], args[1:]
	if cliconv.IsHelpRequest(args) {
		return cliconv.HelpRequest{Text: cliconv.VerbsUsage(environment.Binary, ledgerCommand, ledgerSubcommands)}
	}
	switch subcommand {
	case appendSubcommand:
		return runAppend(environment, rest, stdout)
	case exportSubcommand:
		return runExport(environment, rest, stdout)
	case auditSubcommand:
		return runAudit(environment, rest, stdout)
	case seedSubcommand:
		return runSeed(environment, rest, stdout)
	case fetchTrustSubcommand:
		return runFetchTrust(environment, rest, stdout)
	default:
		return fmt.Errorf("%w: ledger: unknown subcommand %q", errUsage, subcommand)
	}
}

func openRepository(environment Environment, directory string) *gitrepo.Repository {
	return gitrepo.New(directory, gitrepo.WithEnvironment(environment.GitEnvironment...))
}

func runAppend(environment Environment, args []string, stdout io.Writer) error {
	flags := newFlagSet(environment, ledgerCommand+" "+appendSubcommand)
	repository := flags.String("repo", defaultRepository, "git repository that holds the ledger")
	signed := flags.String("signed", "", "output directory of `sign`")
	mutants := flags.String("mutants", "", "directory of <cacheKey>.age encrypted mutants")
	trust := flags.String("trust", "", "directory with the TUF repository snapshot of the trusted root")
	headSHA := flags.String("head-sha", "", "head commit whose seed is recorded")
	runID := flags.String("run-id", "", "CI run that first saw the head commit")
	remote := flags.String("remote", "", "remote to fetch the ledger from and push it to")
	if err := parse(flags, args); err != nil {
		return err
	}
	sources, err := additionSources(*signed, *mutants, *trust, *headSHA, *runID)
	if err != nil {
		return fmt.Errorf("%w: %s: %v", errUsage, appendSubcommand, err)
	}
	additions, err := sources.Additions()
	if err != nil {
		return err
	}
	repo := openRepository(environment, *repository)
	var ledgerRemote app.LedgerRemote = app.NoRemote{}
	if *remote != "" {
		ledgerRemote = gitrepo.NewRemote(repo, *remote)
	}
	service := app.AppendService{Ref: repo, Committer: repo, Remote: ledgerRemote, Payloads: inspect.Reader{}, Clock: environment.Clock}
	report, err := service.Run(additions)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "ledger at %s: %d files added, %d already present\n", report.Commit, len(report.Added), len(report.Unchanged))
	return nil
}

func additionSources(signed, mutants, trust, headSHA, runID string) (filetree.AdditionSources, error) {
	var sources filetree.AdditionSources
	if signed != "" {
		sources = append(sources, filetree.AdditionDir{Root: signed})
	}
	if mutants != "" {
		sources = append(sources, filetree.AdditionDir{Root: mutants, Prefix: format.MutantsDirectory})
	}
	if trust != "" {
		sources = append(sources, filetree.TrustSnapshotDir{Root: trust})
	}
	if (headSHA == "") != (runID == "") {
		return nil, fmt.Errorf("--head-sha and --run-id go together")
	}
	if headSHA != "" {
		seed, err := ledger.NewSeedRecord(headSHA, runID)
		if err != nil {
			return nil, err
		}
		addition, err := seed.Addition()
		if err != nil {
			return nil, err
		}
		sources = append(sources, filetree.FixedAdditions{addition})
	}
	if len(sources) == 0 {
		return nil, fmt.Errorf("nothing to append: give --signed, --mutants, --trust or a seed")
	}
	return sources, nil
}

func runExport(environment Environment, args []string, stdout io.Writer) error {
	flags := newFlagSet(environment, ledgerCommand+" "+exportSubcommand)
	repository := flags.String("repo", defaultRepository, "git repository that holds the ledger")
	to := flags.String("to", "", "directory the proofs and the trust snapshot are written to")
	if err := parse(flags, args); err != nil {
		return err
	}
	if err := requireFlag(flags, "to", *to); err != nil {
		return err
	}
	repo := openRepository(environment, *repository)
	report, err := app.ExportService{Ref: repo, Files: repo, Sink: filetree.ExportDir{Root: *to}}.Run()
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "exported %d proofs and %d trust files to %s\n", report.Proofs, report.TrustFiles, *to)
	return nil
}

func runAudit(environment Environment, args []string, stdout io.Writer) error {
	flags := newFlagSet(environment, ledgerCommand+" "+auditSubcommand)
	repository := flags.String("repo", defaultRepository, "git repository that holds the ledger")
	if err := parse(flags, args); err != nil {
		return err
	}
	repo := openRepository(environment, *repository)
	report, err := app.AuditService{Ref: repo, Files: repo, History: repo, Payloads: inspect.Reader{}}.Run()
	if err != nil {
		return err
	}
	if len(report.Findings) > 0 {
		for _, finding := range report.Findings {
			fmt.Fprintln(stdout, finding)
		}
		return refusal.Newf("the ledger breaks %d properties", len(report.Findings))
	}
	fmt.Fprintf(stdout, "ledger is append-only and content-addressed: %d commits, %d files\n", report.Commits, report.Files)
	return nil
}

func runFetchTrust(environment Environment, args []string, stdout io.Writer) error {
	flags := newFlagSet(environment, ledgerCommand+" "+fetchTrustSubcommand)
	out := flags.String("out", "", "directory the TUF snapshot is written to, for `ledger append --trust`")
	mirror := flags.String("mirror", trustroot.PublicGoodMirror, "TUF mirror of the trusted root")
	if err := parse(flags, args); err != nil {
		return err
	}
	if err := requireFlag(flags, "out", *out); err != nil {
		return err
	}
	tufRoot := environment.TUFRoot
	if tufRoot == nil {
		tufRoot = format.PublicGoodTUFRoot()
	}
	service := app.FetchTrustService{
		Source: trustroot.Mirror{Root: tufRoot, URL: *mirror},
		Sink:   filetree.ExportDir{Root: *out},
	}
	count, err := service.Run(context.Background())
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "fetched %d TUF files from %s into %s\n", count, *mirror, *out)
	return nil
}

func runSeed(environment Environment, args []string, stdout io.Writer) error {
	flags := newFlagSet(environment, ledgerCommand+" "+seedSubcommand)
	repository := flags.String("repo", defaultRepository, "git repository that holds the ledger")
	headSHA := flags.String("head-sha", "", "head commit")
	runID := flags.String("run-id", "", "CI run that first saw the head commit")
	if err := parse(flags, args); err != nil {
		return err
	}
	for name, value := range map[string]string{"head-sha": *headSHA, "run-id": *runID} {
		if err := requireFlag(flags, name, value); err != nil {
			return err
		}
	}
	repo := openRepository(environment, *repository)
	seed, _, err := app.SeedService{Ref: repo, Files: repo}.Resolve(*headSHA, *runID)
	if err != nil {
		return err
	}
	fmt.Fprintln(stdout, seed.Seed)
	return nil
}
