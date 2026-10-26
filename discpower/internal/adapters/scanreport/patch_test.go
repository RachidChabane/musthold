package scanreport_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/discpower/internal/adapters/scanreport"
)

const verifiesPatch = " [verifies SP-2117~1]"

// apply applies a one-hunk unified diff to before, reading only the hunk's
// first old line and its body, as patch does.
func apply(t *rapid.T, before, diff string) string {
	lines := strings.SplitAfter(before, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	body := strings.SplitAfter(diff, "\n")
	var oldStart, oldCount, newStart, newCount int
	if _, err := fmt.Sscanf(body[2], "@@ -%d,%d +%d,%d @@", &oldStart, &oldCount, &newStart, &newCount); err != nil {
		t.Fatalf("hunk header %q: %v", body[2], err)
	}
	first := max(oldStart-1, 0)
	if oldCount == 0 {
		first = oldStart
	}
	var replaced []string
	kept := false
	for index := 3; index < len(body); index++ {
		line := body[index]
		switch {
		case line == "":
		case strings.HasPrefix(line, "\\"):
			if kept {
				replaced[len(replaced)-1] = strings.TrimSuffix(replaced[len(replaced)-1], "\n")
			}
		case strings.HasPrefix(line, " "), strings.HasPrefix(line, "+"):
			replaced = append(replaced, line[1:])
			kept = true
		default:
			kept = false
		}
	}
	result := append(append(append([]string{}, lines[:first]...), replaced...), lines[first+oldCount:]...)
	return strings.Join(result, "")
}

func TestUnifiedDiff(t *testing.T) {
	t.Run("the diff applied to the old content gives the new content [verifies SP-2117~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			line := rapid.StringMatching(`[a-c]{0,3}`)
			old := rapid.SliceOfN(line, 0, 12).Draw(t, "old")
			updated := rapid.SliceOfN(line, 0, 12).Draw(t, "new")
			ending := rapid.SampledFrom([]string{"", "\n"}).Draw(t, "ending")
			before, after := strings.Join(old, "\n")+ending, strings.Join(updated, "\n")+ending
			diff := scanreport.UnifiedDiff("m.py", []byte(before), []byte(after))
			if before == after {
				if diff != "" {
					t.Fatalf("diff of identical contents: %q", diff)
				}
				return
			}
			if got := apply(t, before, diff); got != after {
				t.Fatalf("applied %q to %q: %q, want %q", diff, before, got, after)
			}
		})
	})

	t.Run("git apply applies the diff of a mutant to the reference [verifies SP-2117~1]", func(t *testing.T) {
		dir := t.TempDir()
		before := "def shipping(total):\n    if total >= 50:\n        return 0\n    return 5\n"
		after := "def shipping(total):\n    if total > 50:\n        return 0\n    return 5\n"
		if err := os.MkdirAll(filepath.Join(dir, "invoice"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "invoice", "totals.py"), []byte(before), 0o644); err != nil {
			t.Fatal(err)
		}
		patch := filepath.Join(t.TempDir(), "mutant.patch")
		if err := os.WriteFile(patch, []byte(scanreport.UnifiedDiff("invoice/totals.py", []byte(before), []byte(after))), 0o644); err != nil {
			t.Fatal(err)
		}
		command := exec.Command("git", "apply", patch)
		command.Dir = dir
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git apply: %v\n%s", err, output)
		}
		if got, _ := os.ReadFile(filepath.Join(dir, "invoice", "totals.py")); string(got) != after {
			t.Fatalf("applied: %q", got)
		}
	})
}
