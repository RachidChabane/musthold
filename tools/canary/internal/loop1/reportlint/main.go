package reportlint

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

const (
	exitOK        = 0
	exitRefused   = 1
	exitUsage     = 2
	defaultConfig = "experiments/loop-1/loop-1.json"
	workVariable  = "LOOP1_WORK"
	homeVariable  = "HOME"
	defaultWork   = ".musthold-loop-1"
)

// Main lints results/REPORT.md beside the configuration against results/
// summary.json and the deviation log, results/DEVIATIONS.md or else the work
// directory's: 0 when it is clean, 1 when a rule fails or a file cannot be read,
// 2 on a usage error.
func Main(_ context.Context, args []string, stdout, stderr io.Writer) int {
	work := os.Getenv(workVariable)
	if work == "" {
		work = filepath.Join(os.Getenv(homeVariable), defaultWork)
	}
	flags := flag.NewFlagSet("report-lint", flag.ContinueOnError)
	flags.SetOutput(stderr)
	repo := flags.String("repo", ".", "repository root")
	config := flags.String("config", defaultConfig, "the study's configuration, beside results/")
	workDir := flags.String("work", work, "the work directory")
	if err := flags.Parse(args); err != nil {
		return exitUsage
	}
	if flags.NArg() > 0 {
		fmt.Fprintf(stderr, "report-lint: unexpected arguments %v\n", flags.Args())
		return exitUsage
	}
	configPath := *config
	if !filepath.IsAbs(configPath) {
		configPath = filepath.Join(*repo, configPath)
	}
	results := filepath.Join(filepath.Dir(configPath), analyze.ResultsDir)
	in, err := read(results, study.Layout{Root: *workDir})
	if err != nil {
		fmt.Fprintf(stderr, "report-lint: %v\n", err)
		return exitRefused
	}
	problems, err := Lint(in)
	if err != nil {
		fmt.Fprintf(stderr, "report-lint: %v\n", err)
		return exitRefused
	}
	for _, problem := range problems {
		fmt.Fprintf(stderr, "%s:%s\n", analyze.ReportFile, problem)
	}
	if len(problems) > 0 {
		fmt.Fprintf(stderr, "report-lint: %d problem(s)\n", len(problems))
		return exitRefused
	}
	fmt.Fprintf(stdout, "report-lint: %s is clean\n", filepath.Join(results, analyze.ReportFile))
	return exitOK
}

func read(results string, work study.Layout) (Input, error) {
	report, err := os.ReadFile(filepath.Join(results, analyze.ReportFile))
	if err != nil {
		return Input{}, err
	}
	summary, err := os.ReadFile(filepath.Join(results, analyze.SummaryFile))
	if err != nil {
		return Input{}, err
	}
	deviations, err := os.ReadFile(filepath.Join(results, study.DeviationsFile))
	if errors.Is(err, os.ErrNotExist) {
		deviations, err = os.ReadFile(work.Deviations())
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Input{}, err
	}
	return Input{Report: string(report), Summary: summary, Deviations: string(deviations)}, nil
}
