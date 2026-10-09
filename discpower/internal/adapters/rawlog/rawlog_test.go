package rawlog_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/rawlog"
	"github.com/RachidChabane/spec-proof/format/hostproto"
)

func TestDirectory(t *testing.T) {
	t.Run("a file inside the log is read, and the log is read through the format's file log [verifies SP-1917~1]", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, hostproto.WitnessIndexFile), []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		log := hostproto.FileLog{Files: rawlog.Directory{Root: root}, Root: root}
		index, err := log.Index()
		if err != nil || string(index) != "{}\n" {
			t.Fatalf("index %q (%v)", index, err)
		}
		if _, err := log.File(filepath.Join(root, hostproto.WitnessIndexFile)); err != nil {
			t.Fatalf("a file named by its absolute path: %v", err)
		}
	})

	t.Run("a file over the limit, a missing file and a file outside the log are refused [verifies SP-1917~1]", func(t *testing.T) {
		root, outside := t.TempDir(), t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "big.json"), make([]byte, 16), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(outside, "secret.json"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(outside, "secret.json"), filepath.Join(root, "link.json")); err != nil {
			t.Fatal(err)
		}
		directory := rawlog.Directory{Root: root}
		if _, err := directory.ReadFile("big.json", 8); err == nil {
			t.Error("a file over the limit was read")
		}
		if _, err := directory.ReadFile("missing.json", 8); err == nil {
			t.Error("a missing file was read")
		}
		if _, err := directory.ReadFile("link.json", 8); err == nil {
			t.Error("a link out of the log was followed")
		}
		if _, err := directory.ReadFile("../"+filepath.Base(outside)+"/secret.json", 8); err == nil {
			t.Error("a path out of the log was read")
		}
		if _, err := (rawlog.Directory{Root: filepath.Join(root, "absent")}).ReadFile("big.json", 8); err == nil {
			t.Error("a log that does not exist was read")
		}
	})
}
