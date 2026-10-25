package app_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/tools/canary/internal/adapters"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/app"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/record"
)

const recording = "testdata/recording"

// placeholders are the texts the renderer writes where a record has nothing.
var placeholders = []string{"(no output)"}

func renderer() app.Renderer { return app.Renderer{Files: adapters.OSFiles{}} }

func rendered(t *testing.T, dir string) app.Rendered {
	t.Helper()
	out, err := renderer().Render(dir)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// copyRecording copies the test recording, so that a test can change it.
func copyRecording(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := (adapters.OSFiles{}).CopyTree(recording, dir); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestRenderedTranscript(t *testing.T) {
	markdown := string(rendered(t, recording).Markdown)

	t.Run("the steps appear in the order of time, each gate decision with the message the agent received [verifies SP-2405~1]", func(t *testing.T) {
		order := []string{
			"## Task", "· Agent", "· Edit", "· Edit", "· Agent", "· Gate: blocked", "· Skill", "· Command", "· Executors: 2 launched",
			"· Command", "· Command", "· Gate: blocked", "· Agent", "· Edit", "· Command",
			"· Agent", "· Gate: passed", "## Outcome", "## Change the session made", "## Counts and timings",
		}
		rest := markdown
		for _, wanted := range order {
			index := strings.Index(rest, wanted)
			if index < 0 {
				t.Fatalf("%q is missing or out of order in:\n%s", wanted, markdown)
			}
			rest = rest[index+len(wanted):]
		}
		for _, wanted := range []string{
			"mutant 10edb8fdacce violates it and survived every test",
			"every test passes on an operator mutant of family relational that fails the criterion's example 1: ticket_price(age=65) => 8",
			"+    assert ticket_price(age=65) == 8",
			"Gate after the session: pass, exit code 0 (gate-after.json)",
			"shallnot: gate passed: 4 requirement(s) in focus covered by passing tests",
		} {
			if !strings.Contains(markdown, wanted) {
				t.Errorf("the transcript lacks %q", wanted)
			}
		}
	})

	t.Run("every entry names the record and the line it comes from [verifies SP-2405~1]", func(t *testing.T) {
		for _, wanted := range []string{"Source: `stream.jsonl:2`", "Source: `hooks.jsonl:2`", "Source: `hooks.jsonl:4`", "Source: `stream.jsonl:24`"} {
			if !strings.Contains(markdown, wanted) {
				t.Errorf("the transcript lacks %s", wanted)
			}
		}
		if strings.Count(markdown, "\nSource: `") != strings.Count(markdown, "\n### ")+1 {
			t.Errorf("%d sources for %d entries", strings.Count(markdown, "\nSource: `"), strings.Count(markdown, "\n### "))
		}
	})

	t.Run("the counts and timings come from the records [verifies SP-2405~1]", func(t *testing.T) {
		for _, row := range []string{
			"| Assistant messages, main agent | 13 |", "| Assistant messages, subagents | 2 |", "| Executors launched | 2 |",
			"| Prove runs | 3 |", "| Gate runs, blocked | 2 |", "| Gate runs, passed | 1 |", "| Time in the gate | 6.3 s |",
			"| Session wall-clock time | 570.0 s |", "| Cost reported by the CLI | 3.2104 USD |", "| Turns reported by the CLI | 31 |",
		} {
			if !strings.Contains(markdown, row) {
				t.Errorf("the counts lack %s", row)
			}
		}
		if !strings.Contains(markdown, "### 4:00 · Gate: blocked") || !strings.Contains(markdown, "### 0:52 · Command") {
			t.Errorf("the times are not those of the records:\n%s", markdown)
		}
	})

	t.Run("every line shown in a block is a line of a record [verifies SP-2405~1]", func(t *testing.T) {
		corpus := recordedLines(t, recording)
		shown := blockLines(markdown)
		if len(shown) < 40 {
			t.Fatalf("only %d block lines were read", len(shown))
		}
		for _, line := range shown {
			edited := line
			if strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-") {
				edited = line[1:]
			}
			if corpus[line] || corpus[strings.TrimPrefix(line, "$ ")] || corpus[edited] || isPlaceholder(line) {
				continue
			}
			t.Errorf("the line %q is in no record", line)
		}
	})
}

// blockLines are the non-empty lines inside the fenced blocks of a Markdown text.
func blockLines(markdown string) []string {
	var lines []string
	inside := false
	for _, line := range strings.Split(markdown, "\n") {
		if strings.HasPrefix(line, "```") {
			inside = !inside
			continue
		}
		if inside && strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func isPlaceholder(line string) bool {
	for _, placeholder := range placeholders {
		if line == placeholder {
			return true
		}
	}
	return false
}

// recordedLines are the lines of every string a recording holds: the strings
// of its JSON documents and the lines of its other files.
func recordedLines(t *testing.T, dir string) map[string]bool {
	t.Helper()
	corpus := map[string]bool{}
	add := func(text string) {
		for _, line := range strings.Split(text, "\n") {
			corpus[line] = true
		}
	}
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		add(string(content))
		for _, line := range strings.Split(string(content), "\n") {
			var value any
			if json.Unmarshal([]byte(line), &value) == nil {
				walkStrings(value, add)
			}
		}
		var document any
		if json.Unmarshal(content, &document) == nil {
			walkStrings(document, add)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return corpus
}

func walkStrings(value any, add func(string)) {
	switch typed := value.(type) {
	case string:
		add(typed)
		var nested any
		if json.Unmarshal([]byte(typed), &nested) == nil {
			walkStrings(nested, add)
		}
	case []any:
		for _, item := range typed {
			walkStrings(item, add)
		}
	case map[string]any:
		for _, item := range typed {
			walkStrings(item, add)
		}
	}
}

func TestRenderedHTML(t *testing.T) {
	fragment := string(rendered(t, recording).HTML)

	t.Run("the fragment is self-contained and escapes every recorded text [verifies SP-2405~1]", func(t *testing.T) {
		if !strings.HasPrefix(fragment, `<section class="spec-proof-loop">`) || !strings.HasSuffix(fragment, "</section>\n") {
			t.Errorf("fragment starts %q", fragment[:40])
		}
		for _, forbidden := range []string{"<script", "<link", "src=", "href=", "<ok>", "<run>"} {
			if strings.Contains(fragment, forbidden) {
				t.Errorf("the fragment holds %q", forbidden)
			}
		}
		for _, wanted := range []string{"Done. &lt;ok&gt;", "&lt;run&gt;/project/tests/test_prices.py", `<span class="add">+    assert ticket_price(age=65) == 8</span>`, `<article class="entry entry-gate passed">`} {
			if !strings.Contains(fragment, wanted) {
				t.Errorf("the fragment lacks %s", wanted)
			}
		}
	})
}

func TestRenderedFailures(t *testing.T) {
	t.Run("a run ended on its cap, with no closing event and a gate that gave up, is rendered as it ended [verifies SP-2405~1]", func(t *testing.T) {
		dir := copyRecording(t)
		meta, err := os.ReadFile(filepath.Join(dir, record.MetaFile))
		if err != nil {
			t.Fatal(err)
		}
		killed := strings.Replace(string(meta), `"exitCode": 0`, `"exitCode": 143, "killed": "turn-cap"`, 1)
		write(t, filepath.Join(dir, record.MetaFile), killed)
		stream, err := os.ReadFile(filepath.Join(dir, record.StreamFile))
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(strings.TrimSpace(string(stream)), "\n")
		write(t, filepath.Join(dir, record.StreamFile), strings.Join(lines[:len(lines)-1], "\n")+"\n")
		hooks, err := os.ReadFile(filepath.Join(dir, record.HooksFile))
		if err != nil {
			t.Fatal(err)
		}
		gaveUp := `{"module":"shallnot","args":["hook","claude-stop"],"stderr":"shallnot: the gate is still blocked after 3 attempts; run ` + "`shallnot gate`" + ` to see why.","exitCode":1,"startedAt":"2026-10-25T21:09:25Z","durationMs":900}`
		write(t, filepath.Join(dir, record.HooksFile), string(hooks)+gaveUp+"\n")
		if err := os.Remove(filepath.Join(dir, record.FinalGateFile)); err != nil {
			t.Fatal(err)
		}
		markdown := string(rendered(t, dir).Markdown)
		for _, wanted := range []string{
			"Session: ended by the runner (turn-cap), exit code 143", "Result event: none in stream.jsonl",
			"· Gate: gave up", "shallnot: the gate is still blocked after 3 attempts", "Gate after the session: no report in gate-after.json",
			"| Gate runs, gave up | 1 |",
		} {
			if !strings.Contains(markdown, wanted) {
				t.Errorf("the transcript lacks %q", wanted)
			}
		}
	})

	t.Run("a directory without a recording is refused [verifies SP-2405~1]", func(t *testing.T) {
		if _, err := renderer().Render(t.TempDir()); !errors.Is(err, app.ErrNoRecording) {
			t.Errorf("error = %v", err)
		}
	})
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReplay(t *testing.T) {
	t.Run("rendering gives the same bytes every time [verifies SP-2406~1]", func(t *testing.T) {
		first, second := rendered(t, recording), rendered(t, recording)
		if string(first.Markdown) != string(second.Markdown) || string(first.HTML) != string(second.HTML) {
			t.Error("two renderings differ")
		}
	})

	t.Run("a written transcript passes the check, and a changed one fails it without being rewritten [verifies SP-2406~1]", func(t *testing.T) {
		out := t.TempDir()
		if err := renderer().Write(recording, out, false); err != nil {
			t.Fatal(err)
		}
		if err := renderer().Write(recording, out, true); err != nil {
			t.Fatalf("check after write: %v", err)
		}
		path := filepath.Join(out, record.TranscriptHTML)
		write(t, path, "edited")
		if err := renderer().Write(recording, out, true); !errors.Is(err, app.ErrStale) {
			t.Fatalf("error = %v", err)
		}
		if content, _ := os.ReadFile(path); string(content) != "edited" {
			t.Error("the check rewrote the transcript")
		}
	})
}
