package pendingfile_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/RachidChabane/musthold/alignment/internal/adapters/pendingfile"
	"github.com/RachidChabane/musthold/format"
)

const module = "alignment"

func pending(entries ...format.PendingEntry) format.Pending {
	return format.Pending{Module: module, Pending: entries}
}

func TestDirectory(t *testing.T) {
	t.Run("the pending file is written in the form the format decodes [verifies SP-333~1]", func(t *testing.T) {
		directory := pendingfile.Directory{Root: t.TempDir()}
		want := pending(
			format.PendingEntry{Criterion: "PAR-1~1", Reason: format.PendingAwaitingHost},
			format.PendingEntry{Criterion: "PAR-2~1", Reason: format.PendingAwaitingProof},
		)
		if err := directory.Write(want); err != nil {
			t.Fatal(err)
		}
		content, err := os.ReadFile(filepath.Join(directory.Root, ".musthold", "pending", "alignment.json"))
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := format.DecodePending(content)
		if err != nil || len(decoded.Pending) != 2 || decoded.Module != module {
			t.Fatalf("decoded %+v: %v", decoded, err)
		}
		read, err := directory.Read(module)
		if err != nil || len(read.Pending) != 2 {
			t.Fatalf("read %+v: %v", read, err)
		}
	})

	t.Run("a module that waits for nothing has no file [verifies SP-333~1]", func(t *testing.T) {
		directory := pendingfile.Directory{Root: t.TempDir()}
		if err := directory.Write(pending(format.PendingEntry{Criterion: "PAR-1~1", Reason: format.PendingAwaitingHost})); err != nil {
			t.Fatal(err)
		}
		if err := directory.Write(pending()); err != nil {
			t.Fatal(err)
		}
		read, err := directory.Read(module)
		if err != nil || len(read.Pending) != 0 || read.Module != module {
			t.Fatalf("read %+v: %v", read, err)
		}
		if err := directory.Write(pending()); err != nil {
			t.Fatalf("removing an absent file: %v", err)
		}
	})

	t.Run("an invalid pending file is refused before it is written [verifies SP-333~1]", func(t *testing.T) {
		directory := pendingfile.Directory{Root: t.TempDir()}
		unsorted := pending(
			format.PendingEntry{Criterion: "PAR-2~1", Reason: format.PendingAwaitingHost},
			format.PendingEntry{Criterion: "PAR-1~1", Reason: format.PendingAwaitingHost},
		)
		if err := directory.Write(unsorted); err == nil {
			t.Fatal("an unsorted pending file was written")
		}
		if err := directory.Write(pending(format.PendingEntry{Criterion: "PAR-1", Reason: format.PendingAwaitingHost})); err == nil {
			t.Fatal("a criterion without a revision was written")
		}
		if _, err := os.Stat(filepath.Join(directory.Root, ".musthold")); err == nil {
			t.Fatal("a refused file left a directory behind")
		}
	})

	t.Run("a file that is not a pending file is a read error [verifies SP-333~1]", func(t *testing.T) {
		directory := pendingfile.Directory{Root: t.TempDir()}
		path := filepath.Join(directory.Root, ".musthold", "pending", "discpower.json")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := directory.Read("discpower"); err == nil {
			t.Fatal("a corrupt file was read")
		}
	})
}
