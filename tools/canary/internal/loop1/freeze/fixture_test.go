package freeze_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/config"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/construct/records"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/deviation"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/freeze"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/product"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

var (
	t0           = time.Date(2026, 10, 26, 15, 55, 27, 0, time.UTC)
	claudeDigest = strings.Repeat("c", 64)
	imageDigest  = strings.Repeat("e", 64)
)

const (
	configJSON = `{
  "tasks": [{"name": "alpha", "fixture": "fixtures/alpha", "prompt": "Do alpha.",
             "operations": {"op": {"python": "alpha:op"}}}],
  "arms": ["traceability", "four-proofs"],
  "repetitions": 2, "seed": 7, "model": "claude-sonnet-5-5",
  "caps": {"assistant_messages": 10, "seconds": 60, "budget_usd": "1.00"},
  "allowed_tools": ["Skill"]
}`
	taskJSON = `{
  "name": "alpha", "fixture": "fixtures/alpha", "spec": "specs/a.md", "source": "alpha.py", "criterion": "ALP-4",
  "prompt": "Do alpha.", "operations": {"op": {"python": "alpha:op"}},
  "interface": {"module": "alpha", "operation": "op", "signature": "op(*, n: int) -> int"},
  "truth": {"reference": "experiments/loop-1/tasks/alpha/reference", "violations": "experiments/loop-1/tasks/alpha/violations",
            "refactors": "experiments/loop-1/tasks/alpha/refactors", "oracle": "experiments/loop-1/tasks/alpha/oracle"}
}`
	policyTemplate = `package policy

import "github.com/RachidChabane/musthold/format"

const (
%s
)

func Default() Policy {
	return Policy{
%s
	}
}
`
	formatSource = `package format

type Isolation string

const (
	IsolationRestricted Isolation = "restricted"
	IsolationOther      Isolation = "other"
)

func UniformThreshold(v float64) Threshold { return Threshold{} }
`
)

// scenario says where a fixture departs from a study that passes every check.
type scenario struct {
	// tamperInputs edits inputs.json before it is committed.
	tamperInputs func(*study.Inputs)
	// preregSums and inputsSums rewrite the sums files before they are committed.
	preregSums, inputsSums func(string) string
	// policyEdit rewrites a module's Default literal member value in the source.
	policyEdit map[string]string
	// genTimes overrides the generation time of a freeze, by tag.
	genTimes map[string]time.Time
	// badImprint names the tags whose token stamps something else.
	badImprint map[string]bool
	// commitFile overrides the content of a freeze's commit file, by tag.
	commitFile map[string]string
	// skipTag names a tag that is not set.
	skipTag string
	// runs are the run starts the work directory holds.
	runs []freeze.RunStart
	// armFiles lets a preparer return per-arm files.
	armFiles func(arm config.Arm) map[string]string
	// engineImage is the id the engine reports for the built image.
	engineImage string
	// tamperProduct edits the product directory after the manifest is written.
	tamperProduct func(dir string)
	// constructionStart overrides the start of the construction calls.
	constructionStart time.Time
	// claudeOutput replaces what `claude --version` prints in the image.
	claudeOutput string
	// resolveFails stops the build at resolve and keeps its error.
	resolveFails bool
}

// changesRepository reports whether the scenario changes what is committed, so
// that the study must be built for it.
func (sc scenario) changesRepository() bool {
	return sc.tamperInputs != nil || sc.preregSums != nil || sc.inputsSums != nil || sc.policyEdit != nil ||
		sc.commitFile != nil || sc.skipTag != "" || sc.armFiles != nil || sc.claudeOutput != "" ||
		sc.resolveFails || !sc.constructionStart.IsZero()
}

// studyFor is the shared study when the scenario leaves the repository alone, and
// a study built for it otherwise.
func studyFor(t *testing.T, sc scenario) studyRepo {
	t.Helper()
	if sc.changesRepository() {
		return newStudy(t, sc)
	}
	return shared(t).with(t, sc)
}

// study is a repository with every freeze, built the way PREREG §13 builds it.
type studyRepo struct {
	resolveErr             error
	root, productDir, work string
	check                  freeze.Checker
	scenario               scenario
}

var (
	sharedOnce  sync.Once
	sharedStudy studyRepo
	sharedRoot  string
)

func TestMain(m *testing.M) {
	for key, value := range map[string]string{
		"GIT_CONFIG_GLOBAL": os.DevNull, "GIT_CONFIG_NOSYSTEM": "1",
		"GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@example.invalid", "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@example.invalid",
		"GIT_AUTHOR_DATE": "2026-01-01T00:00:00Z", "GIT_COMMITTER_DATE": "2026-01-01T00:00:00Z",
	} {
		os.Setenv(key, value)
	}
	code := m.Run()
	if sharedRoot != "" {
		os.RemoveAll(sharedRoot)
	}
	os.Exit(code)
}

// shared is the study that passes every condition, built once for the tests whose
// scenario only changes what the checker is given.
func shared(t *testing.T) studyRepo {
	t.Helper()
	sharedOnce.Do(func() {
		var err error
		if sharedRoot, err = os.MkdirTemp("", "freeze-shared-"); err != nil {
			t.Fatal(err)
		}
		sharedStudy = buildStudy(t, sharedRoot, scenario{})
	})
	if sharedRoot == "" {
		t.Fatal("the shared study could not be built")
	}
	return sharedStudy
}

// with returns the study checked under a scenario that changes the checker's
// collaborators and the product directory, never the repository.
func (s studyRepo) with(t *testing.T, sc scenario) studyRepo {
	t.Helper()
	productDir := s.productDir
	if sc.tamperProduct != nil {
		productDir = filepath.Join(t.TempDir(), "product")
		if err := os.CopyFS(productDir, os.DirFS(s.productDir)); err != nil {
			t.Fatal(err)
		}
		sc.tamperProduct(productDir)
	}
	s.productDir = productDir
	s.check = freeze.Checker{
		Root: s.root, ProductDir: productDir, Repo: freeze.Git{Dir: s.root, Exec: freeze.OSExec{}},
		Tokens: tokens{scenario: sc}, Runs: fakeRuns{starts: sc.runs}, Engine: fakeEngine{image: sc.engineImage},
	}
	return s
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func write(t *testing.T, root, name, content string) {
	t.Helper()
	target := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func commit(t *testing.T, root, message string) string {
	t.Helper()
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", message)
	return git(t, root, "rev-parse", "HEAD")
}

func policySource(module string, edits map[string]string) string {
	var constants, fields []string
	for member, value := range freeze.ExpectedPolicy[module] {
		field := strings.ReplaceAll(strings.Title(strings.ReplaceAll(member, "_", " ")), " ", "")
		if edited, ok := edits[module+"."+member]; ok {
			value = edited
		}
		switch member {
		case "min_isolation":
			fields = append(fields, fmt.Sprintf("\t\t%s: format.Isolation%s,", field, strings.Title(value)))
		case "tau_judge", "tau_screen", "tau_route", "modal_min", "tau_adv":
			constants = append(constants, fmt.Sprintf("\tDefault%s = %s0", field, value))
			fields = append(fields, fmt.Sprintf("\t\t%s: format.UniformThreshold(Default%s),", field, field))
		default:
			constants = append(constants, fmt.Sprintf("\tDefault%s = %s", field, value))
			fields = append(fields, fmt.Sprintf("\t\t%s: Default%s,", field, field))
		}
	}
	return fmt.Sprintf(policyTemplate, strings.Join(constants, "\n"), strings.Join(fields, "\n"))
}

// tokens answers for the timestamp reader: an honest token stamps the commit file
// next to the reply, at the freeze's generation time.
type tokens struct{ scenario scenario }

func (k tokens) Read(_ context.Context, reply string) (deviation.Token, error) {
	stem := strings.TrimSuffix(reply, ".tsr")
	content, err := os.ReadFile(stem + ".txt")
	if err != nil {
		return deviation.Token{}, err
	}
	var tag string
	var stamped time.Time
	switch filepath.Base(stem) {
	case "prereg-commit":
		tag, stamped = freeze.PreregTag, t0
	case "a1-commit":
		tag, stamped = freeze.AmendmentTag, t0.Add(time.Hour)
	default:
		tag, stamped = freeze.InputsTag, t0.Add(4*time.Hour)
	}
	if override, ok := k.scenario.genTimes[tag]; ok {
		stamped = override
	}
	imprint := deviation.Imprint(content)
	if k.scenario.badImprint[tag] {
		imprint = strings.Repeat("0", 64)
	}
	return deviation.Token{Imprint: imprint, GenTime: stamped}, nil
}

type fakeEngine struct{ image string }

func (fakeEngine) DockerVersion(context.Context) (string, error) { return "27.5.1", nil }
func (fakeEngine) ColimaVersion(context.Context) (string, error) { return "0.8.1", nil }
func (fakeEngine) Settings(context.Context, string) (freeze.ColimaSettings, error) {
	return freeze.ColimaSettings{CPUs: freeze.ExpectedColimaCPUs, MemoryBytes: freeze.ExpectedColimaMemory, DiskBytes: freeze.ExpectedColimaDisk}, nil
}
func (e fakeEngine) ImageID(_ context.Context, image string) (string, error) {
	if e.image != "" {
		return e.image, nil
	}
	return image, nil
}

type fakeHost struct{}

func (fakeHost) Info(context.Context) (freeze.HostInfo, error) {
	return freeze.HostInfo{OS: "Darwin", Version: "25.6.0", Arch: "arm64"}, nil
}

type fakeReleases struct{}

func (fakeReleases) Tags(context.Context) ([]string, error) {
	return []string{"v0.5.1", "v0.4.0", "v0.5.0", "nightly", "v0.6.0"}, nil
}

func (fakeReleases) Contains(_ context.Context, tag, _ string) (bool, error) {
	return tag == "v0.5.1" || tag == "v0.6.0", nil
}

type fakeCanary struct{ records []study.CanaryRecord }

func (f fakeCanary) Output(context.Context) (study.CanaryOutput, error) {
	held := study.CanaryCheck{Held: true}
	return study.CanaryOutput{
		C1: held, C2: held, C3: held,
		CLIAuxiliaryModels: []string{"claude-haiku-4-5"}, EgressObserved: []string{"api.anthropic.com", "statsig.anthropic.com"},
		EgressAllowlist: []string{"api.anthropic.com"}, QuotaPatterns: []string{"usage limit reached"}, Records: f.records,
	}, nil
}

type fakeConstruction struct {
	start time.Time
	// output is a check's output, which names the machine's paths.
	output string
}

func (f fakeConstruction) Records(context.Context) (records.Log, error) {
	call := func(version string, offset time.Duration) records.Call {
		start := f.start.Add(offset)
		return records.Call{Role: records.RoleAuthor, CodexVersion: version, Start: start, End: start.Add(time.Minute)}
	}
	return records.Log{
		Calls:  []records.Call{call("codex-cli 0.9.0", 0), call("codex-cli 0.9.0", time.Hour)},
		Checks: []records.Check{{Domain: "x", Round: 1, ID: "A2", Output: f.output}},
		Drops:  []records.Drop{{Domain: "x"}},
	}, nil
}

// machineOutput is a check output that names the work directory, the repository
// and the home directory.
func machineOutput(work, root, home string) string {
	return work + "/construction/accept/x/round-1/junit.xml; " + root + "/experiments; " + home + "/.cache"
}

// fixtureHome is the home directory the fixture's resolver redacts.
const fixtureHome = "/home/owner"

type fakePreparer struct {
	files func(arm config.Arm) map[string]string
}

func (f fakePreparer) Prepare(_ context.Context, _ config.Task, arm config.Arm) (freeze.Prepared, error) {
	files := map[string]string{"AGENTS.md": strings.Repeat("a", 64), "shallnot.yaml": strings.Repeat(string(arm[0]), 64)}
	if f.files != nil {
		files = f.files(arm)
	}
	return freeze.Prepared{
		BaseCommit: strings.Repeat("b", 40), EquippedTree: strings.Repeat(string(arm[0]), 64), Files: files, SessionSettings: strings.Repeat("5", 64),
	}, nil
}

type fakeRuns struct{ starts []freeze.RunStart }

func (f fakeRuns) Starts(context.Context) ([]freeze.RunStart, error) { return f.starts, nil }

// boxAnswers answers the commands the resolver runs in the study's image.
type boxAnswers struct{ claude string }

func (b boxAnswers) Run(_ context.Context, command box.Command) (box.Result, error) {
	answers := map[string]string{
		"/usr/local/bin/python3": "Python 3.13.14", "/usr/local/bin/pytest": "pytest 9.1.1", "/usr/local/bin/uv": "uv 0.11.23 (abcdef 2026-01-01)",
		"/usr/local/bin/node": "v24.1.0", "/usr/bin/git": "git version 2.39.5", "/opt/musthold/bin/shallnot": "shallnot v0.5.1",
		"/usr/local/bin/claude": "2.1.295 (Claude Code)",
	}
	if command.Argv[0] == "/usr/bin/sha256sum" {
		return box.Result{Stdout: []byte(claudeDigest + "  " + command.Argv[1] + "\n")}, nil
	}
	if b.claude != "" {
		answers["/usr/local/bin/claude"] = b.claude
	}
	answer, ok := answers[command.Argv[0]]
	if !ok {
		return box.Result{ExitCode: 127, Stderr: []byte("not found")}, nil
	}
	return box.Result{Stdout: []byte(answer + "\n")}, nil
}

func resolver(t *testing.T, root, productDir, work string, sc scenario) freeze.Resolver {
	t.Helper()
	configured, err := config.Parse([]byte(configJSON))
	if err != nil {
		t.Fatal(err)
	}
	layout := study.Layout{Root: work}
	start := t0.Add(2 * time.Hour)
	if !sc.constructionStart.IsZero() {
		start = sc.constructionStart
	}
	return freeze.Resolver{
		Root: root, ConfigPath: study.ConfigFile, Config: configured, Work: layout, Home: fixtureHome, ProductDir: productDir, ProductMount: "/opt/musthold",
		Repo: freeze.Git{Dir: root, Exec: freeze.OSExec{}}, Box: boxAnswers{claude: sc.claudeOutput}, Engine: fakeEngine{}, Host: fakeHost{}, Releases: fakeReleases{},
		Canary:       fakeCanary{records: []study.CanaryRecord{{Name: "c1.jsonl", SHA256: strings.Repeat("d", 64), Start: t0.Add(3 * time.Hour)}}},
		Construction: fakeConstruction{start: start, output: machineOutput(work, root, fixtureHome)}, Preparer: fakePreparer{files: sc.armFiles},
	}
}

func writeProduct(t *testing.T, dir, head string) {
	t.Helper()
	for _, name := range product.RequiredBinaries() {
		write(t, dir, "bin/"+name, "binary "+name)
	}
	for _, plugin := range product.PluginPaths() {
		write(t, dir, "plugins/"+plugin+"/plugin.json", "plugin "+plugin)
	}
	digests, err := product.Describe(dir)
	if err != nil {
		t.Fatal(err)
	}
	manifest := product.Manifest{
		Commit: head, ShallnotPin: freeze.ExpectedShallnotPin, Arch: product.ArchARM64,
		Binaries: digests.Binaries, Plugins: digests.Plugins,
		Images: product.Images{
			Build: "golang:1.26.4-bookworm@sha256:" + strings.Repeat("1", 64), Base: "node:24-bookworm-slim@sha256:" + strings.Repeat("2", 64),
			UV: "ghcr.io/astral-sh/uv:0.11.23@sha256:" + strings.Repeat("3", 64), Built: "sha256:" + imageDigest,
		},
	}
	data, err := manifest.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	write(t, dir, product.ManifestFile, string(data))
}

// newStudy builds the repository, the product and the work directory, and runs
// resolve, in the order of PREREG §13.
func newStudy(t *testing.T, sc scenario) studyRepo {
	t.Helper()
	return buildStudy(t, t.TempDir(), sc)
}

func buildStudy(t *testing.T, base string, sc scenario) studyRepo {
	t.Helper()
	root, productDir, work := filepath.Join(base, "repo"), filepath.Join(base, "product"), filepath.Join(base, "work")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, root, "init", "-q", "-b", "main")
	git(t, root, "config", "core.hooksPath", os.DevNull)
	git(t, root, "config", "tag.gpgsign", "false")
	git(t, root, "config", "commit.gpgsign", "false")
	git(t, root, "config", "tag.forceSignAnnotated", "false")

	writePreregTree(t, root, sc)
	preregSums := preregSumsText(t, root)
	if sc.preregSums != nil {
		preregSums = sc.preregSums(preregSums)
	}
	write(t, root, freeze.PreregSumsFile, preregSums)
	preregCommit := commit(t, root, "prereg")
	setTag(t, root, sc, freeze.PreregTag)
	stampFile(t, root, freeze.Prereg, sc, preregCommit)

	write(t, root, "experiments/loop-1/AMENDMENT-a1.md", "amendment")
	amendmentCommit := commit(t, root, "amendment")
	setTag(t, root, sc, freeze.AmendmentTag)
	stampFile(t, root, freeze.Amendment, sc, amendmentCommit)

	write(t, root, "tools/canary/internal/tooling.go", "package tooling")
	head := commit(t, root, "tooling")
	writeProduct(t, productDir, head)

	r := resolver(t, root, productDir, work, sc)
	result, err := r.Resolve(t.Context())
	if sc.resolveFails {
		return studyRepo{resolveErr: err}
	}
	if err != nil {
		t.Fatal(err)
	}
	if sc.tamperInputs != nil {
		sc.tamperInputs(&result.Inputs)
		if result, err = rerender(t, r, result.Inputs); err != nil {
			t.Fatal(err)
		}
	}
	if sc.inputsSums != nil {
		result.Files[freeze.InputsSumsFile] = []byte(sc.inputsSums(string(result.Files[freeze.InputsSumsFile])))
	}
	if err := r.Write(result); err != nil {
		t.Fatal(err)
	}
	inputsCommit := commit(t, root, "inputs")
	setTag(t, root, sc, freeze.InputsTag)
	stampFile(t, root, freeze.Inputs, sc, inputsCommit)
	commit(t, root, "inputs timestamp")

	if sc.tamperProduct != nil {
		sc.tamperProduct(productDir)
	}
	return studyRepo{
		root: root, productDir: productDir, work: work, scenario: sc,
		check: freeze.Checker{
			Root: root, ProductDir: productDir, Repo: freeze.Git{Dir: root, Exec: freeze.OSExec{}},
			Tokens: tokens{scenario: sc}, Runs: fakeRuns{starts: sc.runs}, Engine: fakeEngine{image: sc.engineImage},
		},
	}
}

func writePreregTree(t *testing.T, root string, sc scenario) {
	t.Helper()
	files := map[string]string{
		"experiments/loop-1/PREREG.md":                                                 "protocol",
		"experiments/loop-1/loop-1.json":                                               configJSON,
		"experiments/loop-1/tasks/alpha/task.json":                                     taskJSON,
		"experiments/loop-1/tasks/alpha/oracle/test_a.py":                              "def test_a(): pass",
		"experiments/loop-1/tasks/alpha/reference/alpha.py":                            "def op(*, n): return n",
		"experiments/loop-1/tasks/alpha/violations/a/1/x.py":                           "x",
		"experiments/loop-1/tasks/alpha/refactors/1/x.py":                              "x",
		"experiments/loop-1/image/Dockerfile":                                          "FROM scratch",
		"experiments/loop-1/image/requirements.lock":                                   "pytest==9.1.1 \\\n    --hash=sha256:00\n",
		"fixtures/alpha/alpha.py":                                                      "def op(*, n): return n + 1",
		"demo/loop/fixture/boxoffice/prices.py":                                        "x",
		"tools/canary/internal/demoloop/fixturegate/testdata/task/boxoffice/prices.py": "x",
		freeze.ShallnotPinFile:                                                         freeze.ExpectedShallnotPin + "\n",
		freeze.ClaudeCodePinFile:                                                       freeze.ExpectedClaudeVersion + "\n",
		"format/executor.go":                                                           formatSource,
	}
	for module := range freeze.ExpectedPolicy {
		files[module+"/internal/domain/policy/policy.go"] = policySource(module, sc.policyEdit)
	}
	for name, content := range files {
		write(t, root, name, content)
	}
}

func preregSumsText(t *testing.T, root string) string {
	t.Helper()
	repo := freeze.Git{Dir: root, Exec: freeze.OSExec{}}
	git(t, root, "add", "-A")
	files, err := repo.Tracked(t.Context(), freeze.PreregRoots...)
	if err != nil {
		t.Fatal(err)
	}
	var sums []freeze.Sum
	for _, file := range files {
		if strings.HasPrefix(file, freeze.PreregExcluded) {
			continue
		}
		content, err := os.ReadFile(filepath.Join(root, file))
		if err != nil {
			t.Fatal(err)
		}
		sums = append(sums, freeze.Sum{Digest: deviation.Imprint(content), Path: file})
	}
	return freeze.FormatSums(sums)
}

func setTag(t *testing.T, root string, sc scenario, tag string) {
	t.Helper()
	if sc.skipTag != tag {
		git(t, root, "tag", "-a", "-m", tag, tag)
	}
}

// stampFile commits the freeze's commit file with a placeholder reply.
func stampFile(t *testing.T, root string, f freeze.Freeze, sc scenario, tagged string) {
	t.Helper()
	content := tagged + "\n"
	if override, ok := sc.commitFile[f.Tag]; ok {
		content = override
	}
	write(t, root, f.CommitFile(), content)
	write(t, root, strings.TrimSuffix(f.CommitFile(), ".txt")+".tsr", "reply")
	git(t, root, "add", "-A")
	if f.Tag != freeze.InputsTag {
		git(t, root, "commit", "-q", "-m", "timestamp "+f.Tag)
	}
}

func rerender(t *testing.T, r freeze.Resolver, inputs study.Inputs) (freeze.Result, error) {
	t.Helper()
	return r.Render(t.Context(), inputs)
}

func failureConditions(failures []freeze.Failure) map[string]bool {
	conditions := map[string]bool{}
	for _, failure := range failures {
		conditions[failure.Condition] = true
	}
	return conditions
}
