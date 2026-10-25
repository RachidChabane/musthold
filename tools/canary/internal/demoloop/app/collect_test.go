package app_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/tools/canary/internal/adapters"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/app"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/config"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/plan"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/record"
)

const (
	drawnMutant = "10edb8fdacce0000000000000000000000000000000000000000000000000000"
	staleMutant = "a2ac7905b3d20000000000000000000000000000000000000000000000000000"
	pricesFile  = "boxoffice/prices.py"
)

// plantStatement writes a discriminating-power statement that holds the mutant
// and names the code file with the digest.
func plantStatement(t *testing.T, project, name, mutant, digest string) {
	t.Helper()
	statement := map[string]any{"predicate": map[string]any{
		"inputs":  []map[string]any{{"name": pricesFile, "role": "code_tree", "digest": map[string]string{"sha256": digest}}},
		"mutants": []map[string]any{{"digest": mutant}},
	}}
	encoded, err := json.Marshal(statement)
	if err != nil {
		t.Fatal(err)
	}
	write(t, mustDir(t, filepath.Join(project, ".spec-proof", "cache", "discriminating-power", "BOX-2~1", name+".json")), string(encoded))
}

func mustDir(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCollectCounterexamples(t *testing.T) {
	w := newWorld(t, night, true)
	requireProduct(t, w.options.RepoRoot)
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required")
	}
	run, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	l := plan.Layout{Repo: w.options.RepoRoot, Run: run}
	task := w.options.Config.Tasks[0]
	preparer := app.Preparer{Processes: adapters.Exec{}, Files: adapters.OSFiles{}, Env: os.Environ()}
	prepared, err := preparer.Prepare(context.Background(), l, config.Run{Task: task, Arm: config.ArmTraceability, Repetition: 1}, []string{filepath.Join(l.Repo, plan.ToolsBinRelative)})
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(l.Project(), pricesFile))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	plantStatement(t, l.Project(), "current", drawnMutant, hex.EncodeToString(sum[:]))
	plantStatement(t, l.Project(), "older", staleMutant, strings.Repeat("0", 64))
	for id, source := range map[string]string{
		drawnMutant: "def ticket_price(age):\n    if age > SENIOR_FROM_AGE:\n        return SENIOR_PRICE\n    if age >= ADULT_FROM_AGE:\n        return ADULT_PRICE\n    return CHILD_PRICE",
		staleMutant: "def ticket_price(age):\n    return 0",
	} {
		encoded, _ := json.Marshal(map[string]any{"edits": []map[string]string{{"file": pricesFile, "qualname": "ticket_price", "new_source": source}}})
		write(t, mustDir(t, filepath.Join(l.Path(plan.CacheDir), "spec-proof", "digest", record.MutantsDir, id+record.MutantExtension)), string(encoded))
	}
	block := "requirement BOX-2~1: mutant " + drawnMutant[:12] + " violates it\\nrequirement BOX-2~1: mutant " + staleMutant[:12] + " violates it"
	write(t, mustDir(t, l.Record(record.HooksFile)), `{"module":"shallnot","args":["hook","claude-stop"],"stderr":"`+block+`","exitCode":2}`+"\n")

	collector := app.Collector{Processes: adapters.Exec{}, Files: adapters.OSFiles{}, Preparer: preparer}
	if err := collector.Collect(context.Background(), &prepared, record.Meta{Task: task.Name}); err != nil {
		t.Fatal(err)
	}

	t.Run("a named mutant gets the diff git writes against the file version it was drawn on [verifies SP-2404~1]", func(t *testing.T) {
		diff, err := os.ReadFile(filepath.Join(l.Records(), record.CounterexamplesDir, drawnMutant[:12]+record.DiffExtension))
		if err != nil {
			t.Fatal(err)
		}
		for _, wanted := range []string{"--- a/boxoffice/prices.py", "+++ b/boxoffice/prices.py", "-    if age >= SENIOR_FROM_AGE:", "+    if age > SENIOR_FROM_AGE:"} {
			if !strings.Contains(string(diff), wanted) {
				t.Errorf("the diff lacks %q:\n%s", wanted, diff)
			}
		}
		if strings.Count(string(diff), "\n+") != 2 {
			t.Errorf("the diff changes more than the mutated line:\n%s", diff)
		}
	})

	t.Run("a mutant drawn on another version of the file gets no diff, and every mutant is kept [verifies SP-2404~1]", func(t *testing.T) {
		if _, err := os.Stat(filepath.Join(l.Records(), record.CounterexamplesDir, staleMutant[:12]+record.DiffExtension)); !os.IsNotExist(err) {
			t.Errorf("a stale mutant was diffed: %v", err)
		}
		for _, id := range []string{drawnMutant, staleMutant} {
			if _, err := os.Stat(filepath.Join(l.Records(), record.MutantsDir, id+record.MutantExtension)); err != nil {
				t.Error(err)
			}
		}
		if _, err := os.Stat(l.Record(record.MetaFile)); err != nil {
			t.Error(err)
		}
	})
}
