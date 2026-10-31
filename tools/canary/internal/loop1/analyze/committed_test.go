package analyze

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// The committed results of the study.
const (
	committedSummary = "../../../../../experiments/loop-1/results/summary.json"
	committedReport  = "../../../../../experiments/loop-1/results/REPORT.md"
)

func TestCommittedResults(t *testing.T) {
	t.Run("the committed report renders from the committed summary up to its exploratory section [verifies SP-2604~1]", func(t *testing.T) {
		data, err := os.ReadFile(committedSummary)
		if err != nil {
			t.Fatal(err)
		}
		var summary Summary
		if err := json.Unmarshal(data, &summary); err != nil {
			t.Fatal(err)
		}
		report, err := os.ReadFile(committedReport)
		if err != nil {
			t.Fatal(err)
		}
		rendered, _, _ := strings.Cut(Render(summary), ExploratoryMarker)
		if !strings.HasPrefix(string(report), rendered) {
			t.Fatal("REPORT.md differs from what summary.json renders")
		}
	})
}
