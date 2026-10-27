package verbs

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/record"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/phase"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

func TestC3Detectors(t *testing.T) {
	runDir := filepath.Join(t.TempDir(), "c3")
	written := study.Run{EquippedCommit: "abc123", Attempt: 1, Outcome: study.OutcomeCompleted}
	written.Task = "boxoffice"
	data, err := json.Marshal(written)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(phase.Records(runDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(phase.Records(runDir), record.MetaFile), data, 0o644); err != nil {
		t.Fatal(err)
	}
	observed := []string{"api.anthropic.com"}
	var steps []string
	var measured study.Run
	detectors := c3Detectors{
		measure: func(_ context.Context, dir string, run study.Run) error {
			steps, measured = append(steps, "measure "+dir), run
			return nil
		},
		detect: func(_ context.Context, dir string, egress []string) (study.Detection, error) {
			steps = append(steps, "detect "+dir)
			if !slices.Equal(egress, observed) {
				t.Errorf("the detectors got the hosts %v", egress)
			}
			return study.Detection{Hits: []study.Hit{{Detector: study.DetectorUncited}}}, nil
		},
	}

	t.Run("C3's held-out violations are measured on its task and equipped commit before the detectors run with C1's hosts [verifies SP-2649~1]", func(t *testing.T) {
		detection, err := detectors.Detect(context.Background(), runDir, observed)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(steps, []string{"measure " + runDir, "detect " + runDir}) || measured.Task != "boxoffice" || measured.EquippedCommit != "abc123" || len(detection.Hits) != 1 {
			t.Fatalf("steps %v, measured %+v, detection %+v", steps, measured, detection)
		}
	})
}
