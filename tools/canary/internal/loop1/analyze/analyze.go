// Package analyze is the `analyze` verb of the loop-1 study (PREREG §6.6, §8 to
// §12). Once the run phase has ended and every protocol and token-exposed
// deviation is timestamped, it computes facts.json for each counted run, the
// decisions of H1 and H2, the secondary endpoints and the sensitivity analyses,
// and writes results/: summary.json, REPORT.md rendered from it, each counted
// run's records with the machine's paths as placeholders, the deviation log and
// SHA256SUMS. It reads files, runs openssl on the timestamp tokens, and makes no
// network or model call.
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
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/plan"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/facts"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/phase"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/stamp"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// Where the verb finds its inputs, relative to the directory of the
// configuration.
const (
	FreezeDir        = "freeze"
	InputsFile       = "inputs.json"
	preregTokenFile  = "prereg-commit.tsr"
	inputsTokenFile  = "inputs-commit.tsr"
	jsonIndent       = "  "
	workVariable     = "LOOP1_WORK"
	homeVariable     = "HOME"
	defaultWork      = ".musthold-loop-1"
	defaultConfig    = "experiments/loop-1/loop-1.json"
	exitOK           = 0
	exitRefused      = 1
	exitUsage        = 2
	workDigestPrefix = "work/"
)

// Errors of the verb.
var (
	ErrInputs = errors.New("freeze/inputs.json cannot be used")
)

// Options are the verb's inputs.
type Options struct {
	// Repo is the repository root; Config the configuration, whose directory
	// holds freeze/ and results/; Work the work directory.
	Repo, Config, Work string
	// Home is the home directory the results write as ~.
	Home string
	// Now is the time the end of the run phase is judged at.
	Now      time.Time
	Imprints stamp.Imprints
}

// Experiment is the directory of the configuration.
func (o Options) Experiment() string { return filepath.Dir(o.Config) }

// Results is the results directory.
func (o Options) Results() string { return filepath.Join(o.Experiment(), ResultsDir) }

// inputsView is the part of inputs.json the verb reads. Only order is required;
// the others feed the provenance and are read leniently.
type inputsView struct {
	Order        []string        `json:"order"`
	Commits      json.RawMessage `json:"commits"`
	Tasks        json.RawMessage `json:"tasks"`
	Construction json.RawMessage `json:"construction"`
}

func readInputs(path string) (inputsView, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return inputsView{}, "", fmt.Errorf("%w: %v", ErrInputs, err)
	}
	var view inputsView
	if err := json.Unmarshal(data, &view); err != nil {
		return inputsView{}, "", fmt.Errorf("%w: %v", ErrInputs, err)
	}
	if len(view.Order) == 0 {
		return inputsView{}, "", fmt.Errorf("%w: it holds no order", ErrInputs)
	}
	digest := sha256.Sum256(data)
	return view, hex.EncodeToString(digest[:]), nil
}

// provenance reads the tags' commits, the tasks and the dropped domains from
// inputs.json, and the times of the freeze tokens.
func provenance(ctx context.Context, o Options, view inputsView, digest string) Provenance {
	var commits struct {
		Prereg  string            `json:"prereg"`
		Inputs  string            `json:"inputs"`
		Between []json.RawMessage `json:"between"`
	}
	_ = json.Unmarshal(view.Commits, &commits)
	var tasks []struct {
		Name string `json:"name"`
	}
	_ = json.Unmarshal(view.Tasks, &tasks)
	var construction struct {
		Dropped []Dropped `json:"dropped"`
	}
	_ = json.Unmarshal(view.Construction, &construction)
	found := Provenance{InputsDigest: digest, Tasks: []string{}, Dropped: construction.Dropped}
	for _, task := range tasks {
		found.Tasks = append(found.Tasks, task.Name)
	}
	if commits.Between != nil {
		found.Between = []string{}
	}
	for _, raw := range commits.Between {
		found.Between = append(found.Between, commitOf(raw))
	}
	for _, tag := range []struct{ name, commit, token string }{
		{TagPrereg, commits.Prereg, preregTokenFile}, {TagInputs, commits.Inputs, inputsTokenFile},
	} {
		entry := Tag{Name: tag.name, Commit: tag.commit}
		tsr := filepath.Join(o.Experiment(), FreezeDir, tag.token)
		if _, err := os.Stat(tsr); err == nil {
			if token, err := o.Imprints.Read(ctx, tsr); err == nil {
				at := token.Time
				entry.Time = &at
			}
		}
		found.Tags = append(found.Tags, entry)
	}
	return found
}

// commitOf reads a commit recorded as its hash, or as an object whose commit
// member is the hash; anything else is kept as its JSON text.
func commitOf(raw json.RawMessage) string {
	var hash string
	if json.Unmarshal(raw, &hash) == nil {
		return hash
	}
	var object struct {
		Commit string `json:"commit"`
	}
	if json.Unmarshal(raw, &object) == nil && object.Commit != "" {
		return object.Commit
	}
	return string(raw)
}

// Analyze runs the verb.
func Analyze(ctx context.Context, o Options, out io.Writer) error {
	view, digest, err := readInputs(filepath.Join(o.Experiment(), FreezeDir, InputsFile))
	if err != nil {
		return err
	}
	layout := study.Layout{Root: o.Work}
	attempts, err := phase.Load(layout, view.Order)
	if err != nil {
		return err
	}
	judged := phase.Judge(view.Order, attempts, o.Now)
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
	digests := map[string]string{InputsFile: digest}
	all, kept, err := countedFacts(o, judged, digests)
	if err != nil {
		return err
	}
	summary := Build(BuildInput{
		Phase: judged, Facts: all, Deviations: study.ParseDeviations(string(log)),
		Provenance: provenance(ctx, o, view, digest), Digests: digests,
	})
	if err := writeResults(o, layout, summary, kept); err != nil {
		return err
	}
	fmt.Fprintf(out, "analyze: %d counted runs; H1 %s; H2 %s; results in %s\n",
		len(all), summary.Analysis.H1.Decision, summary.Analysis.H2.Decision, o.Results())
	return nil
}

// countedFacts computes and writes facts.json for each counted attempt, adding
// the digest of every record read under its path in the work directory.
func countedFacts(o Options, judged phase.Phase, digests map[string]string) (map[string]facts.Facts, []CountedRecords, error) {
	all := map[string]facts.Facts{}
	var kept []CountedRecords
	for _, state := range judged.Runs {
		attempt, ok := state.CountedAttempt()
		if !ok {
			continue
		}
		in, read, err := facts.Load(attempt.Records(), filepath.Join(attempt.Dir, plan.ProjectDir))
		if err != nil {
			return nil, nil, err
		}
		computed := facts.Compute(state.Name, in)
		data, err := json.MarshalIndent(computed, "", jsonIndent)
		if err != nil {
			return nil, nil, err
		}
		if err := writeFile(filepath.Join(attempt.Records(), study.FactsFile), append(data, '\n')); err != nil {
			return nil, nil, err
		}
		rel, _ := filepath.Rel(o.Work, attempt.Records())
		for name, sum := range read {
			digests[workDigestPrefix+filepath.ToSlash(filepath.Join(rel, name))] = sum
		}
		all[state.Name] = computed
		kept = append(kept, CountedRecords{Run: state.Name, Attempt: attempt.Dir, Records: attempt.Records()})
	}
	return all, kept, nil
}

// writeResults writes results/: summary.json, then REPORT.md rendered from what
// summary.json holds, the runs, the deviation log and SHA256SUMS.
func writeResults(o Options, layout study.Layout, summary Summary, kept []CountedRecords) error {
	results := o.Results()
	if err := os.RemoveAll(filepath.Join(results, RunsDir)); err != nil {
		return err
	}
	data, err := json.MarshalIndent(summary, "", jsonIndent)
	if err != nil {
		return err
	}
	paths := Paths{Repo: o.Repo, Home: o.Home}
	data = append(paths.redact(data, ""), '\n')
	if err := writeFile(filepath.Join(results, SummaryFile), data); err != nil {
		return err
	}
	var written Summary
	if err := json.NewDecoder(bytes.NewReader(data)).Decode(&written); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(results, ReportFile), []byte(Render(written))); err != nil {
		return err
	}
	var raw []string
	for _, counted := range kept {
		files, err := writeRun(results, counted, paths)
		if err != nil {
			return err
		}
		raw = append(raw, files...)
	}
	if err := copyDeviations(results, layout); err != nil {
		return err
	}
	return writeSums(results, raw)
}

// Main parses the verb's flags and runs it: 0 when the results are written, 1
// when the verb refuses or fails, 2 on a usage error.
func Main(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	o, status := parse(args, stderr)
	if status != exitOK {
		return status
	}
	o.Now, o.Imprints = time.Now(), stamp.OpenSSL{}
	if err := Analyze(ctx, o, stdout); err != nil {
		fmt.Fprintf(stderr, "analyze: %v\n", err)
		return exitRefused
	}
	return exitOK
}

func parse(args []string, stderr io.Writer) (Options, int) {
	home := os.Getenv(homeVariable)
	work := os.Getenv(workVariable)
	if work == "" {
		work = filepath.Join(home, defaultWork)
	}
	flags := flag.NewFlagSet("analyze", flag.ContinueOnError)
	flags.SetOutput(stderr)
	repo := flags.String("repo", ".", "repository root")
	config := flags.String("config", defaultConfig, "the study's configuration, beside freeze/ and results/")
	workDir := flags.String("work", work, "the work directory")
	if err := flags.Parse(args); err != nil {
		return Options{}, exitUsage
	}
	if flags.NArg() > 0 {
		fmt.Fprintf(stderr, "analyze: unexpected arguments %v\n", flags.Args())
		return Options{}, exitUsage
	}
	root, err := filepath.Abs(*repo)
	if err != nil {
		fmt.Fprintf(stderr, "analyze: %v\n", err)
		return Options{}, exitUsage
	}
	configPath := *config
	if !filepath.IsAbs(configPath) {
		configPath = filepath.Join(root, configPath)
	}
	return Options{Repo: root, Config: configPath, Work: *workDir, Home: home}, exitOK
}
