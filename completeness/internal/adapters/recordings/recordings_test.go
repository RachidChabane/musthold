package recordings

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

const (
	firstOutput  = `{"validity":"VALID"}`
	secondOutput = `{"validity":"EQUIVALENT"}`
)

func requestID(label string) string { return hostprototest.Digest(label) }

func TestRecordings(t *testing.T) {
	t.Run("a recorded output and executor load back unchanged [verifies SP-540~1]", func(t *testing.T) {
		directory := Directory{Dir: filepath.Join(t.TempDir(), "recordings")}
		id := requestID("round trip")
		executor := hostprototest.Profile()
		if err := directory.Record(id, []byte(firstOutput), executor); err != nil {
			t.Fatal(err)
		}
		recording, err := directory.Load(id)
		if err != nil {
			t.Fatal(err)
		}
		if string(recording.Output) != firstOutput || recording.Executor == nil || *recording.Executor != executor {
			t.Fatalf("recording %+v", recording)
		}
	})

	t.Run("an output without a recorded executor loads with a nil executor [verifies SP-540~1]", func(t *testing.T) {
		dir := t.TempDir()
		id := requestID("no executor")
		if err := os.WriteFile(filepath.Join(dir, id+outputSuffix), []byte(firstOutput), fileMode); err != nil {
			t.Fatal(err)
		}
		recording, err := Directory{Dir: dir}.Load(id)
		if err != nil || recording.Executor != nil || string(recording.Output) != firstOutput {
			t.Fatalf("recording %+v, error %v", recording, err)
		}
	})

	t.Run("a request without a recording is an error [verifies SP-540~1]", func(t *testing.T) {
		if _, err := (Directory{Dir: t.TempDir()}).Load(requestID("absent")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("error %v", err)
		}
	})

	t.Run("a corrupt recorded executor is an error [verifies SP-540~1]", func(t *testing.T) {
		dir := t.TempDir()
		id := requestID("corrupt")
		for suffix, content := range map[string]string{outputSuffix: firstOutput, executorSuffix: "{"} {
			if err := os.WriteFile(filepath.Join(dir, id+suffix), []byte(content), fileMode); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := (Directory{Dir: dir}).Load(id); err == nil {
			t.Fatal("a corrupt executor was accepted")
		}
	})

	invalidIDs := []string{"", "../escape", "abc", requestID("upper")[:format.Sha256HexLength-1], "../" + requestID("traversal")}
	for _, id := range invalidIDs {
		t.Run("the request id "+id+" is refused by both operations [verifies SP-540~1]", func(t *testing.T) {
			directory := Directory{Dir: t.TempDir()}
			if _, err := directory.Load(id); !errors.Is(err, ErrInvalidRequestID) {
				t.Fatalf("load error %v", err)
			}
			if err := directory.Record(id, []byte(firstOutput), hostprototest.Profile()); !errors.Is(err, ErrInvalidRequestID) {
				t.Fatalf("record error %v", err)
			}
			if entries, _ := os.ReadDir(directory.Dir); len(entries) != 0 {
				t.Fatalf("a refused id left %d files", len(entries))
			}
		})
	}

	t.Run("recording again replaces the earlier recording and leaves no temporary file [verifies SP-540~1]", func(t *testing.T) {
		directory := Directory{Dir: t.TempDir()}
		id := requestID("overwrite")
		other := hostprototest.Profile()
		other.Harness.Name = "other-harness"
		for _, step := range []struct {
			output   string
			executor format.ExecutorProfile
		}{{firstOutput, hostprototest.Profile()}, {secondOutput, other}} {
			if err := directory.Record(id, []byte(step.output), step.executor); err != nil {
				t.Fatal(err)
			}
		}
		recording, err := directory.Load(id)
		if err != nil || string(recording.Output) != secondOutput || recording.Executor.Harness.Name != other.Harness.Name {
			t.Fatalf("recording %+v, error %v", recording, err)
		}
		entries, err := os.ReadDir(directory.Dir)
		if err != nil || len(entries) != 2 {
			t.Fatalf("%d files in the directory, error %v", len(entries), err)
		}
	})
}
