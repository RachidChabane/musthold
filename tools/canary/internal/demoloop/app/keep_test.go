package app_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/tools/canary/internal/adapters"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/app"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/plan"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/record"
)

// dashed spells a path the way Claude Code names a directory after it.
func dashed(path string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return '-'
	}, path)
}

func TestKeep(t *testing.T) {
	keeper := app.Keeper{Files: adapters.OSFiles{}}
	const (
		repo = "/Users/someone/src/musthold"
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
			record.HooksFile: `{"stdin":{"transcript_path":"` + home + `/.claude/projects/x.jsonl"}}` + "\n" +
				`{"session_dir":"` + run + `/claude-config/projects/` + dashed(run) + `-project","repo_dir":"` + home + `/.claude/projects/` + dashed(repo) + `"}` + "\n",
		}
		for name, content := range files {
			if err := (adapters.OSFiles{}).Write(filepath.Join(records, name), []byte(content), false); err != nil {
				t.Fatal(err)
			}
		}
		return root
	}

	t.Run("the records are kept with the run directory, the repository and the home written as placeholders [verifies SP-2409~4]", func(t *testing.T) {
		root := runOn(t)
		run := root
		to := filepath.Join(t.TempDir(), "recording")
		if err := keeper.Keep(root, repo, home, to, ""); err != nil {
			t.Fatal(err)
		}
		for name, want := range map[string]string{
			record.MetaFile:   `"<repo>/.tools/claude-code/2.1.295/bin/claude","--plugin-dir","<repo>/discpower/plugin/claude"`,
			record.StreamFile: `"cwd":"<run>/project"`,
			record.HooksFile:  `"transcript_path":"~/.claude/projects/x.jsonl"`,
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
		dashed, _ := os.ReadFile(filepath.Join(to, record.HooksFile))
		if !strings.Contains(string(dashed), `"session_dir":"<run>/claude-config/projects/<run>-project"`) ||
			!strings.Contains(string(dashed), `"repo_dir":"~/.claude/projects/<repo>"`) {
			t.Errorf("a dash-encoded path was not replaced: %s", dashed)
		}
		stream, _ := os.ReadFile(filepath.Join(to, record.StreamFile))
		if !strings.Contains(string(stream), `"text":"<run>/project/tests/test_prices.py"`) {
			t.Errorf("the run directory outside /private was not replaced: %s", stream)
		}
	})

	t.Run("the kept transcript is rendered from the kept records, never copied from the run [verifies SP-2409~4]", func(t *testing.T) {
		root := runOn(t)
		records := filepath.Join(root, plan.RecordsDir)
		for _, name := range []string{record.TranscriptMarkdown, record.TranscriptHTML} {
			if err := (adapters.OSFiles{}).Write(filepath.Join(records, name), []byte("<pre>"+repo+"</pre>"), false); err != nil {
				t.Fatal(err)
			}
		}
		to := filepath.Join(t.TempDir(), "recording")
		if err := keeper.Keep(root, repo, home, to, ""); err != nil {
			t.Fatal(err)
		}
		if err := (app.Renderer{Files: adapters.OSFiles{}}).Write(to, to, true); err != nil {
			t.Fatalf("the kept transcript is not what its records render: %v", err)
		}
		html, _ := os.ReadFile(filepath.Join(to, record.TranscriptHTML))
		if strings.Contains(string(html), "<pre><repo></pre>") {
			t.Fatalf("the run's transcript was copied: %s", html)
		}
	})

	t.Run("records that hold the token, exactly or by its shape, are refused and nothing is kept [verifies SP-2409~4]", func(t *testing.T) {
		for name, leak := range map[string]struct{ content, exact string }{
			"by shape": {content: "sk-ant-oat01-" + strings.Repeat("x", 40), exact: ""},
			"exactly":  {content: "the value is correct-horse-battery-staple-42", exact: "correct-horse-battery-staple-42"},
		} {
			root := runOn(t)
			if err := (adapters.OSFiles{}).Write(filepath.Join(root, plan.RecordsDir, record.StderrFile), []byte(leak.content), false); err != nil {
				t.Fatal(err)
			}
			to := filepath.Join(t.TempDir(), "recording")
			err := keeper.Keep(root, repo, home, to, leak.exact)
			if !errors.Is(err, app.ErrTokenInRecord) || !strings.Contains(err.Error(), record.StderrFile) || strings.Contains(err.Error(), leak.content) {
				t.Errorf("%s: error %v", name, err)
			}
			if _, statErr := os.Stat(to); !os.IsNotExist(statErr) {
				t.Errorf("%s: something was kept", name)
			}
		}
	})

	t.Run("a destination that holds a recording is refused [verifies SP-2409~4]", func(t *testing.T) {
		root := runOn(t)
		to := filepath.Join(t.TempDir(), "recording")
		if err := keeper.Keep(root, repo, home, to, ""); err != nil {
			t.Fatal(err)
		}
		if err := keeper.Keep(root, repo, home, to, ""); !errors.Is(err, app.ErrRecordingExists) {
			t.Errorf("error = %v", err)
		}
	})
}
