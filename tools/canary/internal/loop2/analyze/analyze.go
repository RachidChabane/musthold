// Package analyze is the `analyze` verb of the loop-2 study. Once the run phase
// has ended and every protocol and token-exposed deviation is timestamped, it
// reads each counted run's record, computes every endpoint, and writes
// results/summary.json and results/REPORT.md rendered from it. It reads files
// and runs openssl on the timestamp tokens; it makes no network or model call.
package analyze

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	demoapp "github.com/RachidChabane/musthold/tools/canary/internal/demoloop/app"
	l1analyze "github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/phase"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/stamp"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/analysis"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/record"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/report"
)

// Loader reads the loop-2 record of a counted attempt named name, with the
// SHA-256 of every file it read by its path relative to the records directory.
type Loader interface {
	Load(name, records, project string) (record.Run, map[string]string, error)
}

// The verb's files and defaults.
const (
	DefaultInputs    = "experiments/loop-2/freeze/inputs.json"
	DefaultResults   = "experiments/loop-2/results"
	planCommitFile   = "plan-commit.txt"
	planTokenFile    = "plan-commit.tsr"
	preregTokenFile  = "prereg-commit.tsr"
	inputsTokenFile  = "inputs-commit.tsr"
	workDigestPrefix = "work/"
	homeVariable     = "HOME"
	jsonIndent       = "  "
	fileMode         = 0o644
	dirMode          = 0o755
	exitOK           = 0
	exitRefused      = 1
	exitUsage        = 2
)

// ErrInputs refuses an inputs freeze that cannot be used.
var ErrInputs = errors.New("the inputs freeze cannot be used")

// Options are the verb's inputs.
type Options struct {
	// Repo is the repository root, Inputs the inputs freeze, whose directory
	// holds the freeze tokens, Work the work directory and Results the results
	// directory.
	Repo, Inputs, Work, Results string
	// Home is the home directory the results write as ~.
	Home     string
	Now      time.Time
	Imprints stamp.Imprints
	Loader   Loader
}

// readInputs reads the inputs freeze, which must hold the order, and its
// SHA-256.
func readInputs(path string) (study.Inputs, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return study.Inputs{}, "", fmt.Errorf("%w: %v", ErrInputs, err)
	}
	inputs, err := study.ParseInputs(data)
	if err != nil {
		return study.Inputs{}, "", fmt.Errorf("%w: %v", ErrInputs, err)
	}
	if len(inputs.Order) == 0 {
		return study.Inputs{}, "", fmt.Errorf("%w: it holds no order", ErrInputs)
	}
	digest := sha256.Sum256(data)
	return inputs, hex.EncodeToString(digest[:]), nil
}

// provenance reads the tags' commits and the times of their tokens beside the
// inputs freeze; a token not there leaves its time null.
func provenance(ctx context.Context, o Options, inputs study.Inputs, digest string) analysis.Provenance {
	freeze := filepath.Dir(o.Inputs)
	found := analysis.Provenance{InputsDigest: digest, Tasks: []string{}}
	for _, task := range inputs.Tasks {
		found.Tasks = append(found.Tasks, task.Name)
	}
	plan, _ := os.ReadFile(filepath.Join(freeze, planCommitFile))
	for _, tag := range []struct{ name, commit, token string }{
		{analysis.TagPlan, strings.TrimSpace(string(plan)), planTokenFile},
		{analysis.TagPrereg, inputs.Commits.Prereg, preregTokenFile},
		{analysis.TagInputs, inputs.Commits.Inputs, inputsTokenFile},
	} {
		entry := l1analyze.Tag{Name: tag.name, Commit: tag.commit}
		if token, err := o.Imprints.Read(ctx, filepath.Join(freeze, tag.token)); err == nil {
			at := token.Time
			entry.Time = &at
		}
		found.Tags = append(found.Tags, entry)
	}
	return found
}

// Analyze runs the verb.
func Analyze(ctx context.Context, o Options, out io.Writer) error {
	inputs, digest, err := readInputs(o.Inputs)
	if err != nil {
		return err
	}
	layout := study.Layout{Root: o.Work}
	attempts, err := phase.Load(layout, inputs.Order)
	if err != nil {
		return err
	}
	judged := analysis.Calendar.Judge(inputs.Order, attempts, o.Now)
	if err := judged.Require(); err != nil {
		return err
	}
	log, err := os.ReadFile(layout.Deviations())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := stamp.CheckDeviations(ctx, log, layout.Timestamps(), o.Imprints); err != nil {
		return err
	}
	digests := map[string]string{study.InputsFile: digest}
	records, err := countedRecords(o, judged, digests)
	if err != nil {
		return err
	}
	summary := analysis.Build(analysis.BuildInput{
		Phase: judged, Records: records, Deviations: study.ParseDeviations(string(log)),
		Provenance: provenance(ctx, o, inputs, digest), Digests: digests,
	})
	if err := writeResults(o, summary); err != nil {
		return err
	}
	fmt.Fprintf(out, "analyze: %d counted runs; outcome %s; results in %s\n", len(records), summary.Analysis.Primary.Outcome, o.Results)
	return nil
}

// countedRecords reads the record of each counted attempt, adding the digest of
// every file read under its path in the work directory.
func countedRecords(o Options, judged phase.Phase, digests map[string]string) (map[string]record.Run, error) {
	records := map[string]record.Run{}
	for _, state := range judged.Runs {
		attempt, ok := state.CountedAttempt()
		if !ok {
			continue
		}
		run, read, err := o.Loader.Load(state.Name, attempt.Records(), filepath.Join(attempt.Dir, study.ProjectDir))
		if err != nil {
			return nil, err
		}
		rel, _ := filepath.Rel(o.Work, attempt.Records())
		for name, sum := range read {
			digests[workDigestPrefix+filepath.ToSlash(filepath.Join(rel, name))] = sum
		}
		records[state.Name] = run
	}
	return records, nil
}

// writeResults writes summary.json with the machine's paths as placeholders,
// then REPORT.md rendered from what summary.json holds.
func writeResults(o Options, summary analysis.Summary) error {
	data, err := json.MarshalIndent(summary, "", jsonIndent)
	if err != nil {
		return err
	}
	data = append(demoapp.RedactPaths(data, "", o.Repo, o.Home), '\n')
	var written analysis.Summary
	if err := json.NewDecoder(bytes.NewReader(data)).Decode(&written); err != nil {
		return err
	}
	if err := os.MkdirAll(o.Results, dirMode); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(o.Results, l1analyze.SummaryFile), data, fileMode); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(o.Results, l1analyze.ReportFile), []byte(report.Render(written)), fileMode)
}

// Command is the `analyze` verb wired to its adapters.
type Command struct {
	Loader   Loader
	Imprints stamp.Imprints
}

// Main parses the verb's flags and runs it: 0 when the results are written, 1
// when the verb refuses or fails, 2 on a usage error.
func (c Command) Main(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("analyze", flag.ContinueOnError)
	flags.SetOutput(stderr)
	repo := flags.String("repo", ".", "repository root")
	inputs := flags.String("inputs", DefaultInputs, "the inputs freeze, relative to the repository root")
	work := flags.String("work", "", "the work directory of the run phase")
	results := flags.String("results", DefaultResults, "the results directory, relative to the repository root")
	if err := flags.Parse(args); err != nil {
		return exitUsage
	}
	if flags.NArg() > 0 || *work == "" {
		fmt.Fprintln(stderr, "analyze: --work is required, and no argument")
		return exitUsage
	}
	root, err := filepath.Abs(*repo)
	if err != nil {
		fmt.Fprintf(stderr, "analyze: %v\n", err)
		return exitUsage
	}
	o := Options{
		Repo: root, Inputs: under(root, *inputs), Work: *work, Results: under(root, *results),
		Home: os.Getenv(homeVariable), Now: time.Now(), Imprints: c.Imprints, Loader: c.Loader,
	}
	if err := Analyze(ctx, o, stdout); err != nil {
		fmt.Fprintf(stderr, "analyze: %v\n", err)
		return exitRefused
	}
	return exitOK
}

// under resolves a path relative to the repository root.
func under(root, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(root, path)
}
