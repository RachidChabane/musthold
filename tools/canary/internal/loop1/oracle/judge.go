package oracle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// Truth holds the host paths of one task's files. They are read, never written.
type Truth struct {
	Fixture    string
	Reference  string
	Violations string
	Oracle     string
	// Base is the directory of the fixture, slash-separated, that the files of
	// the reference and of each violation are relative to.
	Base string
}

// TruthOf resolves a task's truth paths against the repository root.
func TruthOf(repo string, task study.Task) Truth {
	return Truth{
		Fixture:    filepath.Join(repo, filepath.FromSlash(task.Fixture)),
		Reference:  filepath.Join(repo, filepath.FromSlash(task.Truth.Reference)),
		Violations: filepath.Join(repo, filepath.FromSlash(task.Truth.Violations)),
		Oracle:     filepath.Join(repo, filepath.FromSlash(task.Truth.Oracle)),
		Base:       task.TruthBase(),
	}
}

// Violation is one held-out violation: its name <criterion>/<k> and its
// directory, which holds full edited files mirroring the reference's paths
// relative to the truth base.
type Violation struct {
	Name string
	Dir  string
}

// ListViolations lists the violations under a task's violations directory in
// lexical order.
func ListViolations(dir string) ([]Violation, error) {
	criteria, err := listDirs(dir)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTruth, err)
	}
	var violations []Violation
	for _, criterion := range criteria {
		ks, err := listDirs(filepath.Join(dir, criterion))
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrTruth, err)
		}
		for _, k := range ks {
			violations = append(violations, Violation{Name: criterion + "/" + k, Dir: filepath.Join(dir, criterion, k)})
		}
	}
	return violations, nil
}

// Stop is one end-of-turn snapshot: its number and the id of the project's tree.
type Stop struct {
	Number int
	Tree   string
}

// Judge runs the oracle and measures test directories through a box.
type Judge struct {
	Runner box.Runner
	// JobsDir is a directory under the work directory, mounted read-write in the
	// containers; every job gets a directory of its own in it, removed at the end.
	JobsDir string
	// Memo remembers the kill matrix of each test directory measured, so that a
	// directory the runs of a task share is measured once.
	Memo map[string]study.KillMatrix
}

// NewJudge builds a judge that keeps its work under jobsDir.
func NewJudge(runner box.Runner, jobsDir string) *Judge {
	return &Judge{Runner: runner, JobsDir: jobsDir, Memo: map[string]study.KillMatrix{}}
}

// job creates a job directory and returns it with a function that removes it.
func (j *Judge) job() (string, func(), error) {
	if err := os.MkdirAll(j.JobsDir, dirMode); err != nil {
		return "", nil, err
	}
	dir, err := os.MkdirTemp(j.JobsDir, "job-")
	if err != nil {
		return "", nil, err
	}
	return dir, func() { os.RemoveAll(dir) }, nil
}

// OracleTreeSkips are the top-level entries of the project that the oracle tree
// leaves out: the session's tests and records, and any directory that would mix
// with the task's oracle.
var oracleTreeSkips = []string{testsDirName, configDir, testResultsDir, repositoryDir, oracleDirName}

// OracleTreeDigest digests what the oracle depends on: the project the session
// left, without the entries the oracle tree leaves out, and the task's oracle.
func OracleTreeDigest(runDir string, truth Truth) (string, error) {
	project, err := digestTree(projectPath(runDir), skipTopLevel(oracleTreeSkips...))
	if err != nil {
		return "", err
	}
	oracle, err := digestTree(truth.Oracle, skipNothing)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrTruth, err)
	}
	return digestParts(project, oracle), nil
}

// Oracle runs the task's oracle three times on the project the session left
// (PREREG §4.1) and judges the change (§4.2).
func (j *Judge) Oracle(ctx context.Context, runDir string, truth Truth) (study.Oracle, error) {
	job, cleanup, err := j.job()
	if err != nil {
		return study.Oracle{}, err
	}
	defer cleanup()
	tree := filepath.Join(job, "tree")
	if err := copyTree(projectPath(runDir), tree, skipTopLevel(oracleTreeSkips...)); err != nil {
		return study.Oracle{}, err
	}
	if err := copyTree(truth.Oracle, filepath.Join(tree, oracleDirName), skipNothing); err != nil {
		return study.Oracle{}, fmt.Errorf("%w: %v", ErrTruth, err)
	}
	files, err := testFiles(truth.Oracle, oracleDirName)
	if err != nil {
		return study.Oracle{}, fmt.Errorf("%w: %v", ErrTruth, err)
	}
	reports, err := pytestRunner{runner: j.Runner, mountRoot: job}.runThrice(ctx, tree, filepath.Join(job, "out"), oracleReportPattern, oracleArgv)
	if err != nil {
		return study.Oracle{}, err
	}
	return OracleVerdict(reports, files), nil
}

// HeldOutInput names what the strength of a run's test directories depends on.
type HeldOutInput struct {
	RunDir string
	// EquippedCommit is the hash of the equipped commit in the project's
	// repository; Stops the snapshots of §8 in order.
	EquippedCommit string
	Stops          []Stop
	Truth          Truth
	TaskName       string
}

// testDir is one test directory to measure, as a host directory that holds tests/.
type testDir struct {
	label string
	root  string
}

// HeldOut measures the equipped tests, the final tests and each stop's tests
// against the task's violations (PREREG §4.3).
func (j *Judge) HeldOut(ctx context.Context, in HeldOutInput) (study.HeldOut, error) {
	job, cleanup, err := j.job()
	if err != nil {
		return nil, err
	}
	defer cleanup()
	snapshots := snapshotReader{runner: j.Runner, runDir: in.RunDir, mountRoot: job}
	dirs, err := j.collectDirs(ctx, snapshots, in, job)
	if err != nil {
		return nil, err
	}
	ini, err := os.ReadFile(filepath.Join(job, "equipped", pytestIniFile))
	if err != nil {
		return nil, fmt.Errorf("%w: the equipped commit has no %s: %v", ErrArchive, pytestIniFile, err)
	}
	violations, err := ListViolations(in.Truth.Violations)
	if err != nil {
		return nil, err
	}
	heldOut := study.HeldOut{}
	for _, dir := range dirs {
		matrix, err := j.measure(ctx, measurement{job: job, task: in.TaskName, truth: in.Truth, ini: ini, violations: violations}, dir)
		if err != nil {
			return nil, fmt.Errorf("test directory %s: %w", dir.label, err)
		}
		heldOut[dir.label] = matrix
	}
	return heldOut, nil
}

// collectDirs materialises the three sources of test directories in the job and
// leaves the equipped pytest.ini in job/equipped.
func (j *Judge) collectDirs(ctx context.Context, snapshots snapshotReader, in HeldOutInput, job string) ([]testDir, error) {
	equipped := filepath.Join(job, "equipped")
	if _, err := snapshots.extract(ctx, fromProject, in.EquippedCommit, pytestIniFile, equipped); err != nil {
		return nil, err
	}
	dirs := []testDir{{label: study.DirEquipped, root: equipped}}
	if _, err := snapshots.extract(ctx, fromProject, in.EquippedCommit, testsDirName, equipped); err != nil {
		return nil, err
	}
	final := filepath.Join(job, "final")
	if err := copyTree(filepath.Join(projectPath(in.RunDir), testsDirName), filepath.Join(final, testsDirName), skipTopLevel()); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	dirs = append(dirs, testDir{label: study.DirFinal, root: final})
	for _, stop := range in.Stops {
		root := filepath.Join(job, fmt.Sprintf("stop-%d", stop.Number))
		if _, err := snapshots.extract(ctx, fromStopStore, stop.Tree, testsDirName, root); err != nil {
			return nil, err
		}
		dirs = append(dirs, testDir{label: study.StopDir(stop.Number), root: root})
	}
	return dirs, nil
}

type measurement struct {
	job        string
	task       string
	truth      Truth
	ini        []byte
	violations []Violation
}

// measure computes the kill matrix of one test directory, or recalls it.
func (j *Judge) measure(ctx context.Context, m measurement, dir testDir) (study.KillMatrix, error) {
	tests := filepath.Join(dir.root, testsDirName)
	digest, err := digestTree(tests, skipTopLevel())
	if err != nil && !os.IsNotExist(err) {
		return study.KillMatrix{}, err
	}
	key := digestParts(m.task, string(m.ini), digest)
	if matrix, known := j.Memo[key]; known {
		return matrix, nil
	}
	files, err := testFiles(tests, testsDirName)
	if err != nil {
		return study.KillMatrix{}, err
	}
	reference := filepath.Join(m.job, "reference-"+dir.label)
	if err := j.buildReferenceTree(m, tests, reference); err != nil {
		return study.KillMatrix{}, err
	}
	runner := pytestRunner{runner: j.Runner, mountRoot: m.job}
	referenceRuns, err := runner.runThrice(ctx, reference, filepath.Join(m.job, "out-reference-"+dir.label), heldOutReportPattern, heldOutArgv)
	if err != nil {
		return study.KillMatrix{}, err
	}
	violationRuns := map[string][study.HeldOutRuns]RunReport{}
	valid := len(KillVerdict(referenceRuns, nil, files).Valid) > 0
	for _, violation := range m.violations {
		var runs [study.HeldOutRuns]RunReport
		if valid {
			runs, err = j.runViolation(ctx, runner, m, reference, violation, dir.label)
			if err != nil {
				return study.KillMatrix{}, err
			}
		}
		violationRuns[violation.Name] = runs
	}
	matrix := KillVerdict(referenceRuns, violationRuns, files)
	j.Memo[key] = matrix
	return matrix, nil
}

// buildReferenceTree is the fixture with the reference applied at the truth
// base, the test directory in place of tests/ and the equipped commit's
// pytest.ini.
func (j *Judge) buildReferenceTree(m measurement, tests, dst string) error {
	if err := copyTree(m.truth.Fixture, dst, skipTopLevel(testsDirName)); err != nil {
		return fmt.Errorf("%w: %v", ErrTruth, err)
	}
	if err := copyTree(m.truth.Reference, m.truth.at(dst), skipNothing); err != nil {
		return fmt.Errorf("%w: %v", ErrTruth, err)
	}
	if err := os.MkdirAll(filepath.Join(dst, testsDirName), dirMode); err != nil {
		return err
	}
	if err := copyTree(tests, filepath.Join(dst, testsDirName), skipNothing); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(filepath.Join(dst, pytestIniFile), m.ini, fileMode)
}

// at is the truth base inside a tree.
func (t Truth) at(tree string) string { return filepath.Join(tree, filepath.FromSlash(t.Base)) }

// runViolation runs a test directory three times on the reference tree with one
// violation applied at the truth base, in a copy that is removed afterwards.
func (j *Judge) runViolation(ctx context.Context, runner pytestRunner, m measurement, reference string, violation Violation, label string) ([study.HeldOutRuns]RunReport, error) {
	var none [study.HeldOutRuns]RunReport
	tree := filepath.Join(m.job, "violation-"+label+"-"+safeName(violation.Name))
	defer os.RemoveAll(tree)
	if err := copyTree(reference, tree, skipNothing); err != nil {
		return none, err
	}
	if err := copyTree(violation.Dir, m.truth.at(tree), skipNothing); err != nil {
		return none, fmt.Errorf("%w: %v", ErrTruth, err)
	}
	out := filepath.Join(m.job, "out-violation-"+label+"-"+safeName(violation.Name))
	return runner.runThrice(ctx, tree, out, heldOutReportPattern, heldOutArgv)
}

// HeldOutDigest digests what the kill matrices depend on.
func HeldOutDigest(in HeldOutInput) (string, error) {
	parts := []string{in.EquippedCommit, in.TaskName}
	for _, stop := range in.Stops {
		parts = append(parts, fmt.Sprintf("%d=%s", stop.Number, stop.Tree))
	}
	for _, dir := range []string{filepath.Join(projectPath(in.RunDir), testsDirName), in.Truth.Fixture, in.Truth.Reference, in.Truth.Violations} {
		digest, err := digestTree(dir, skipTopLevel())
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
		parts = append(parts, digest)
	}
	return digestParts(parts...), nil
}

// safeName turns a violation name into one path element.
func safeName(name string) string { return strings.ReplaceAll(name, "/", "_") }

// sortStops orders snapshots by number.
func sortStops(stops []Stop) {
	slices.SortFunc(stops, func(a, b Stop) int { return a.Number - b.Number })
}
