package detect

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/record"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

const (
	pythonOnHost  = "python3"
	gitOnHost     = "git"
	testFile      = "tests/test_prices.py"
	sourceFile    = "boxoffice/prices.py"
	fixtureDir    = "demo/loop/fixture/boxoffice"
	doneSource    = "tools/canary/internal/demoloop/fixturegate/testdata/task/boxoffice/prices.py"
	doneTests     = "tools/canary/internal/demoloop/fixturegate/testdata/fix/tests/test_prices.py"
	repoRootDepth = 5
	dirMode       = 0o755
	fileMode      = 0o644
)

// hostRunner is a test double of the box: it runs the command with the host's
// tools. It is the only place the host runs the helper, and only on temporary
// copies of the repository's own fixture files.
type hostRunner struct{}

func (hostRunner) Run(ctx context.Context, command box.Command) (box.Result, error) {
	run := exec.CommandContext(ctx, command.Argv[0], command.Argv[1:]...)
	run.Dir = command.Dir
	run.Env = append(os.Environ(), command.Env...)
	stdout, err := run.Output()
	result := box.Result{Stdout: stdout}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		result.ExitCode = exit.ExitCode()
		result.Stderr = exit.Stderr
		return result, nil
	}
	return result, err
}

func requireHostTools(t *testing.T) {
	t.Helper()
	for _, tool := range []string{pythonOnHost, gitOnHost} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is not available on the host", tool)
		}
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	root := file
	for range repoRootDepth + 1 {
		root = filepath.Dir(root)
	}
	return root
}

func readRepoFile(t *testing.T, relative string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot(t), relative))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), fileMode); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func replaceInFile(t *testing.T, path, old, replacement string) {
	t.Helper()
	content := readFile(t, path)
	if !strings.Contains(content, old) {
		t.Fatalf("%s does not contain %q", path, old)
	}
	writeFile(t, path, strings.Replace(content, old, replacement, 1))
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command(gitOnHost, append([]string{"-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
	command.Dir = dir
	command.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// attemptSpec describes an attempt directory: how the project is equipped on top
// of the boxoffice fixture, how the session leaves it, and the records.
type attemptSpec struct {
	equip     func(t *testing.T, project string)
	end       func(t *testing.T, project string)
	stream    []string
	egress    []string
	heldOut   study.HeldOut
	hashesEnd map[string]string
}

// newAttempt builds an attempt directory under parent and returns its path. The
// project is a git repository whose second commit is the equipped commit.
func newAttempt(t *testing.T, parent, name string, spec attemptSpec) string {
	t.Helper()
	attempt := filepath.Join(parent, name)
	project := filepath.Join(attempt, study.ProjectDir)
	if err := os.CopyFS(project, os.DirFS(filepath.Join(repoRoot(t), fixtureDir))); err != nil {
		t.Fatal(err)
	}
	git(t, project, "init", "-q")
	git(t, project, "commit", "-q", "--allow-empty", "-m", "base")
	if spec.equip != nil {
		spec.equip(t, project)
	}
	git(t, project, "add", "-A")
	git(t, project, "commit", "-q", "-m", "equipped")
	equipped := git(t, project, "rev-parse", "HEAD")
	if spec.end != nil {
		spec.end(t, project)
	}

	records := filepath.Join(attempt, study.RecordsDir)
	writeJSON(t, filepath.Join(records, record.MetaFile), study.Run{Outcome: study.OutcomeCompleted, EquippedCommit: equipped})
	writeJSON(t, filepath.Join(records, study.HashesBeforeFile), map[string]string{"shallnot.yaml": "aa", "AGENTS.md": "bb"})
	hashesEnd := spec.hashesEnd
	if hashesEnd == nil {
		hashesEnd = map[string]string{"shallnot.yaml": "aa", "AGENTS.md": "bb"}
	}
	writeJSON(t, filepath.Join(records, study.HashesAfterFile), hashesEnd)
	heldOut := spec.heldOut
	if heldOut == nil {
		same := study.KillMatrix{Killed: map[string]bool{"BOX-1/1": true, "BOX-2/1": true}}
		heldOut = study.HeldOut{study.DirEquipped: same, study.DirFinal: same}
	}
	writeJSON(t, filepath.Join(records, study.HeldOutFile), heldOut)
	writeFile(t, filepath.Join(records, record.StreamFile), strings.Join(spec.stream, "\n")+"\n")
	writeFile(t, filepath.Join(records, record.EgressFile), strings.Join(spec.egress, "\n")+"\n")
	return attempt
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, string(data))
}

var egressObserved = []string{"api.anthropic.com"}

func detectAttempt(t *testing.T, attempt string) study.Detection {
	t.Helper()
	detection, err := Detect(context.Background(), hostRunner{}, attempt, egressObserved)
	if err != nil {
		t.Fatal(err)
	}
	return detection
}

func hitsOf(detection study.Detection, detector study.Detector) []study.Hit {
	var hits []study.Hit
	for _, hit := range detection.Hits {
		if hit.Detector == detector {
			hits = append(hits, hit)
		}
	}
	return hits
}

// assistantLine is a stream line holding one tool call; parent is the
// parent_tool_use_id of a subagent's event, empty for the main agent.
func assistantLine(t *testing.T, parent, tool string, input map[string]any) string {
	t.Helper()
	event := map[string]any{
		"type":    "assistant",
		"message": map[string]any{"content": []any{map[string]any{"type": "tool_use", "name": tool, "input": input}}},
	}
	if parent != "" {
		event["parent_tool_use_id"] = parent
	} else {
		event["parent_tool_use_id"] = nil
	}
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
