// Package reportlint is the `report-lint` verb of the loop-2 study: it lints
// results/REPORT.md against results/summary.json and the deviation log by the
// rules of the loop-2 report package.
package reportlint

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	l1analyze "github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze"
	l1reportlint "github.com/RachidChabane/musthold/tools/canary/internal/loop1/reportlint"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/analyze"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/report"
)

const (
	exitOK      = 0
	exitRefused = 1
	exitUsage   = 2
)

// Main lints REPORT.md of the results directory against its summary.json and
// the deviation log, the results directory's DEVIATIONS.md or else the work
// directory's: 0 when it is clean, 1 when a rule fails or a file cannot be
// read, 2 on a usage error.
func Main(_ context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("report-lint", flag.ContinueOnError)
	flags.SetOutput(stderr)
	repo := flags.String("repo", ".", "repository root")
	results := flags.String("results", analyze.DefaultResults, "the results directory, relative to the repository root")
	work := flags.String("work", "", "the work directory, whose deviation log is read when the results hold none")
	if err := flags.Parse(args); err != nil {
		return exitUsage
	}
	if flags.NArg() > 0 {
		fmt.Fprintf(stderr, "report-lint: unexpected arguments %v\n", flags.Args())
		return exitUsage
	}
	dir := *results
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(*repo, dir)
	}
	in, err := read(dir, study.Layout{Root: *work})
	if err != nil {
		fmt.Fprintf(stderr, "report-lint: %v\n", err)
		return exitRefused
	}
	problems, err := report.Lint(in)
	if err != nil {
		fmt.Fprintf(stderr, "report-lint: %v\n", err)
		return exitRefused
	}
	for _, problem := range problems {
		fmt.Fprintf(stderr, "%s:%s\n", l1analyze.ReportFile, problem)
	}
	if len(problems) > 0 {
		fmt.Fprintf(stderr, "report-lint: %d problem(s)\n", len(problems))
		return exitRefused
	}
	fmt.Fprintf(stdout, "report-lint: %s is clean\n", filepath.Join(dir, l1analyze.ReportFile))
	return exitOK
}

func read(results string, work study.Layout) (l1reportlint.Input, error) {
	text, err := os.ReadFile(filepath.Join(results, l1analyze.ReportFile))
	if err != nil {
		return l1reportlint.Input{}, err
	}
	summary, err := os.ReadFile(filepath.Join(results, l1analyze.SummaryFile))
	if err != nil {
		return l1reportlint.Input{}, err
	}
	deviations, err := os.ReadFile(filepath.Join(results, study.DeviationsFile))
	if errors.Is(err, os.ErrNotExist) && work.Root != "" {
		deviations, err = os.ReadFile(work.Deviations())
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return l1reportlint.Input{}, err
	}
	return l1reportlint.Input{Report: string(text), Summary: summary, Deviations: string(deviations)}, nil
}
