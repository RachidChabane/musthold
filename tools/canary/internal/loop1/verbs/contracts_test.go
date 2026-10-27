package verbs_test

import (
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/tools/canary/internal/adapters"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/app"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/accept"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/accept/acceptest"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box/docker/dockertest"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/oracle"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/runs"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

func TestOracleFileNaming(t *testing.T) {
	t.Run("the oracle file construct names for a criterion maps back to it, BOX-4 to test_box_4.py [verifies SP-2643~1]", func(t *testing.T) {
		if accept.OracleFileName("BOX-4") != "test_box_4.py" {
			t.Fatalf("BOX-4 is named %s", accept.OracleFileName("BOX-4"))
		}
		rapid.Check(t, func(t *rapid.T) {
			criterion := rapid.StringMatching(`[A-Z][A-Z0-9]{0,6}-[0-9]{1,4}`).Draw(t, "criterion")
			back, ok := oracle.CriterionOfFile(path.Join(accept.OracleSubdir, accept.OracleFileName(criterion)))
			if !ok || back != criterion {
				t.Fatalf("%s is named %s, which maps to %q", criterion, accept.OracleFileName(criterion), back)
			}
		})
	})
}

func TestTruthBase(t *testing.T) {
	repo := t.TempDir()
	sample := acceptest.NewSample("fare")
	sample.Write(t, repo)
	data, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(study.TaskDir(sample.Name)), study.TaskFile))
	if err != nil {
		t.Fatal(err)
	}
	task, err := study.ParseTask(data)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("the oracle applies the reference and the violations at the directory accept applies them at [verifies SP-2644~1]", func(t *testing.T) {
		truth := oracle.TruthOf(repo, task)
		if truth.Base != task.TruthBase() || task.TruthBase() != path.Dir(task.Source) {
			t.Fatalf("the oracle's base is %q, the task's %q", truth.Base, task.TruthBase())
		}
		violations, err := oracle.ListViolations(truth.Violations)
		if err != nil || len(violations) == 0 {
			t.Fatalf("violations %v, %v", violations, err)
		}
		for _, root := range append([]string{truth.Reference}, dirs(violations)...) {
			err := filepath.WalkDir(root, func(file string, entry os.DirEntry, err error) error {
				if err != nil || entry.IsDir() {
					return err
				}
				relative, _ := filepath.Rel(root, file)
				target := filepath.Join(truth.Fixture, filepath.FromSlash(truth.Base), relative)
				if _, statErr := os.Stat(target); statErr != nil {
					t.Errorf("%s lands on %s, which the fixture does not hold", file, target)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		}
	})
}

func dirs(violations []oracle.Violation) []string {
	var found []string
	for _, violation := range violations {
		found = append(found, violation.Dir)
	}
	return found
}

// imageClaude is where the image installs Claude Code: npm's global prefix of
// the Node.js image.
const imageClaude = "/usr/local/bin/claude"

func TestClaudeOfTheImage(t *testing.T) {
	t.Run("every session and probe runs the image's Claude Code CLI, never one under the product [verifies SP-2647~1]", func(t *testing.T) {
		repo := dockertest.RepoRoot(t)
		flags, err := runs.ParseFlags("contract", []string{"--repo", repo, "--work", t.TempDir(), "--product", t.TempDir()}, nil, io.Discard)
		if err != nil {
			t.Fatal(err)
		}
		setup, err := flags.Resolve(adapters.OSFiles{}, true)
		if err != nil {
			t.Fatal(err)
		}
		claude := app.LayoutOf(setup.Options, t.TempDir()).Claude()
		if claude != imageClaude || strings.HasPrefix(claude, setup.Options.Product) {
			t.Fatalf("the session runs %s", claude)
		}
	})
}
