package ciseed_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/ciseed"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostbundle"
)

const commit = "0123456789abcdef0123456789abcdef01234567"

func TestResolve(t *testing.T) {
	t.Run("the first run draws the seed from the commit and itself and records it [verifies SP-249~2]", func(t *testing.T) {
		dir := t.TempDir()
		record, err := ciseed.Resolve(dir, commit, "1001")
		if err != nil {
			t.Fatal(err)
		}
		if record.Seed != hostbundle.CISeed(commit, "1001") || record.FirstRunID != "1001" || record.HeadSHA != commit {
			t.Fatalf("record = %+v", record)
		}
		if _, err := os.Stat(filepath.Join(dir, "seeds", commit+".json")); err != nil {
			t.Fatalf("nothing was recorded: %v", err)
		}
	})

	t.Run("a later run of the commit reuses the first seed, whatever run it is [verifies SP-249~2]", func(t *testing.T) {
		dir := t.TempDir()
		first, err := ciseed.Resolve(dir, commit, "1001")
		if err != nil {
			t.Fatal(err)
		}
		again, err := ciseed.Resolve(dir, commit, "2002")
		if err != nil || again != first {
			t.Fatalf("again = %+v, %v", again, err)
		}
	})

	t.Run("a new commit draws a new seed [verifies SP-249~2]", func(t *testing.T) {
		dir := t.TempDir()
		first, _ := ciseed.Resolve(dir, commit, "1001")
		other, err := ciseed.Resolve(dir, "fedcba9876543210fedcba9876543210fedcba98", "1001")
		if err != nil || other.Seed == first.Seed {
			t.Fatalf("other = %+v, %v", other, err)
		}
	})

	t.Run("a head that is not a commit id and a damaged record are refused [verifies SP-249~2]", func(t *testing.T) {
		dir := t.TempDir()
		for _, head := range []string{"", "main", "../../etc/passwd", "ABC"} {
			if _, err := ciseed.Resolve(dir, head, "1"); err == nil {
				t.Errorf("%q was accepted", head)
			}
		}
		if err := os.MkdirAll(filepath.Join(dir, "seeds"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "seeds", commit+".json"), []byte("{"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := ciseed.Resolve(dir, commit, "1"); err == nil {
			t.Error("a damaged record was accepted")
		}
	})
}
