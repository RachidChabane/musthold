package analyze

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/construct/records"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

func TestInputsAsResolveWritesThem(t *testing.T) {
	log := records.Log{
		Calls: []records.Call{{Role: records.RoleAuthor, Domain: "water bill", CodexVersion: "codex-cli 0.9.0"}},
		Drops: []records.Drop{{Domain: "water bill", Check: "A5", Reason: "the diagonal kills no violation"}},
	}
	construction, err := json.Marshal(log)
	if err != nil {
		t.Fatal(err)
	}
	written := study.Inputs{
		Commits:      study.Commits{Prereg: "9058fee", Inputs: "abc123", Between: []string{"def456"}},
		Tasks:        []study.TaskInputs{{Name: "boxoffice"}, {Name: "parcel"}},
		Construction: construction,
		Order:        []string{"boxoffice-control-1"},
	}
	document, err := json.MarshalIndent(written, "", jsonIndent)
	if err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(t.TempDir(), "loop-1.json")
	path := filepath.Join(study.FreezeBeside(config), study.InputsFile)
	mustWrite(t, path, document)

	t.Run("the order, the tags' commits, the tasks and the dropped domains come from the freeze resolve writes [verifies SP-2646~1]", func(t *testing.T) {
		inputs, digest, err := readInputs(path)
		if err != nil {
			t.Fatal(err)
		}
		found, err := provenance(context.Background(), Options{Config: config, Imprints: fakeImprints{}}, inputs, digest)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(found.Tasks, []string{"boxoffice", "parcel"}) || !slices.Equal(found.Between, []string{"def456"}) ||
			len(found.Dropped) != 1 || found.Dropped[0] != log.Drops[0] || found.Tags[0].Commit != "9058fee" || found.Tags[1].Commit != "abc123" {
			t.Fatalf("provenance %+v", found)
		}
	})

	t.Run("a member the freeze does not define is refused [verifies SP-2646~1]", func(t *testing.T) {
		var members map[string]any
		if err := json.Unmarshal(document, &members); err != nil {
			t.Fatal(err)
		}
		members["dropped"] = []string{"water bill"}
		stray, _ := json.Marshal(members)
		mustWrite(t, path, stray)
		if _, _, err := readInputs(path); !errors.Is(err, ErrInputs) {
			t.Fatalf("an unknown member: %v", err)
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	})
}
