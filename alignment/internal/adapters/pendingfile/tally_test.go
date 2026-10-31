package pendingfile_test

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/RachidChabane/musthold/alignment/internal/adapters/pendingfile"
	"github.com/RachidChabane/musthold/format"
)

const (
	verifiesTallyFile = " [verifies SP-2683~1]"
	tallyModule       = "alignment"
	concurrentTallies = 16
)

func tallyRead(t *testing.T, directory pendingfile.Directory) (format.Pending, bool) {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(directory.Root, ".musthold", "pending", tallyModule+".json"))
	if os.IsNotExist(err) {
		return format.Pending{}, false
	}
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := format.DecodePending(content)
	if err != nil {
		t.Fatalf("the file does not decode: %v", err)
	}
	return decoded, true
}

func TestAnswerTally(t *testing.T) {
	waiting := []format.PendingEntry{{Criterion: "PAR-1~1", Reason: format.PendingAwaitingHost}}

	t.Run("the first accepted answer makes the file, which a later empty write keeps"+verifiesTallyFile, func(t *testing.T) {
		directory := pendingfile.Directory{Root: t.TempDir(), Lock: filepath.Join(t.TempDir(), "pending.lock")}
		if _, present := tallyRead(t, directory); present {
			t.Fatal("a file before any answer")
		}
		if err := (pendingfile.Tally{Directory: directory, Module: tallyModule}).Accepted(1); err != nil {
			t.Fatal(err)
		}
		if err := directory.Write(format.Pending{Module: tallyModule}); err != nil {
			t.Fatal(err)
		}
		stored, present := tallyRead(t, directory)
		if !present || stored.Answered != 1 || stored.Pending == nil || len(stored.Pending) != 0 {
			t.Fatalf("stored %+v, present %v", stored, present)
		}
	})

	t.Run("the count never decreases and a tally keeps the waiting entries"+verifiesTallyFile, func(t *testing.T) {
		directory := pendingfile.Directory{Root: t.TempDir()}
		tally := pendingfile.Tally{Directory: directory, Module: tallyModule}
		if err := directory.Write(format.Pending{Module: tallyModule, Pending: waiting, Answered: 4}); err != nil {
			t.Fatal(err)
		}
		for _, total := range []int{2, 6, 5} {
			if err := tally.Accepted(total); err != nil {
				t.Fatal(err)
			}
		}
		if err := directory.Write(format.Pending{Module: tallyModule, Pending: waiting, Answered: 3}); err != nil {
			t.Fatal(err)
		}
		if stored, _ := tallyRead(t, directory); stored.Answered != 6 || len(stored.Pending) != 1 {
			t.Fatalf("stored %+v", stored)
		}
	})

	t.Run("concurrent tallies leave a whole file with the largest count"+verifiesTallyFile, func(t *testing.T) {
		directory := pendingfile.Directory{Root: t.TempDir(), Lock: filepath.Join(t.TempDir(), "pending.lock")}
		var group sync.WaitGroup
		for total := 1; total <= concurrentTallies; total++ {
			group.Add(1)
			go func() {
				defer group.Done()
				if err := (pendingfile.Tally{Directory: directory, Module: tallyModule}).Accepted(total); err != nil {
					t.Error(err)
				}
			}()
		}
		group.Wait()
		entries, err := os.ReadDir(filepath.Join(directory.Root, ".musthold", "pending"))
		if err != nil || len(entries) != 1 {
			t.Fatalf("the pending directory holds %v, %v", entries, err)
		}
		if stored, _ := tallyRead(t, directory); stored.Answered != concurrentTallies {
			t.Fatalf("stored %+v", stored)
		}
	})
}
