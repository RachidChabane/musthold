package app_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/tools/canary/internal/adapters"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/app"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/plan"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/record"
)

func TestKeep(t *testing.T) {
	keeper := app.Keeper{Files: adapters.OSFiles{}}
	const (
		repo = "/Users/someone/src/spec-proof"
		home = "/Users/someone"
	)
	runOn := func(t *testing.T) string {
		t.Helper()
		run, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		root := run
		records := filepath.Join(root, plan.RecordsDir)
		files := map[string]string{
			record.MetaFile:   `{"argv":["` + repo + `/.tools/claude-code/2.1.295/bin/claude","--plugin-dir","` + repo + `/discpower/plugin/claude"],"task":"boxoffice"}`,
			record.StreamFile: `{"type":"system","cwd":"` + run + `/project"}` + "\n" + `{"type":"user","text":"` + strings.TrimPrefix(run, "/private") + `/project/tests/test_prices.py"}` + "\n",
			filepath.Join(record.CounterexamplesDir, "10edb8fdacce.diff"): "--- a/boxoffice/prices.py\n+++ b/boxoffice/prices.py\n",
			record.HooksFile: `{"stdin":{"transcript_path":"` + home + `/.claude/projects/x.jsonl"}}` + "\n",
		}
		for name, content := range files {
			if err := (adapters.OSFiles{}).Write(filepath.Join(records, name), []byte(content), false); err != nil {
				t.Fatal(err)
			}
		}
		return root
	}

	t.Run("the records are kept with the run directory, the repository and the home written as placeholders [verifies SP-2409~1]", func(t *testing.T) {
		root := runOn(t)
		run := root
		to := filepath.Join(t.TempDir(), "recording")
		if err := keeper.Keep(root, repo, home, to); err != nil {
			t.Fatal(err)
		}
		for name, want := range map[string]string{
			record.MetaFile:   `"<repo>/.tools/claude-code/2.1.295/bin/claude","--plugin-dir","<repo>/discpower/plugin/claude"`,
			record.StreamFile: `"cwd":"<run>/project"`,
			filepath.Join(record.CounterexamplesDir, "10edb8fdacce.diff"): "--- a/boxoffice/prices.py",
			record.HooksFile: `"transcript_path":"~/.claude/projects/x.jsonl"`,
		} {
			content, err := os.ReadFile(filepath.Join(to, name))
			if err != nil || !strings.Contains(string(content), want) {
				t.Errorf("%s = %s (%v), want %s", name, content, err, want)
			}
			for _, leaked := range []string{run, strings.TrimPrefix(run, "/private"), home} {
				if strings.Contains(string(content), leaked) {
					t.Errorf("%s still names %s", name, leaked)
				}
			}
		}
		stream, _ := os.ReadFile(filepath.Join(to, record.StreamFile))
		if !strings.Contains(string(stream), `"text":"<run>/project/tests/test_prices.py"`) {
			t.Errorf("the run directory outside /private was not replaced: %s", stream)
		}
	})

	t.Run("a destination that holds a recording is refused [verifies SP-2409~1]", func(t *testing.T) {
		root := runOn(t)
		to := filepath.Join(t.TempDir(), "recording")
		if err := keeper.Keep(root, repo, home, to); err != nil {
			t.Fatal(err)
		}
		if err := keeper.Keep(root, repo, home, to); !errors.Is(err, app.ErrRecordingExists) {
			t.Errorf("error = %v", err)
		}
	})
}
