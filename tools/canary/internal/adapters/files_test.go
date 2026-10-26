package adapters_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/tools/canary/internal/adapters"
)

func TestReplace(t *testing.T) {
	files := adapters.OSFiles{}

	t.Run("a file is overwritten in place and keeps the mode it had [verifies SP-1512~1]", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.toml")
		if err := os.WriteFile(path, []byte("long old content\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := files.Replace(path, []byte("new\n")); err != nil {
			t.Fatal(err)
		}
		content, _ := os.ReadFile(path)
		info, err := os.Stat(path)
		if err != nil || string(content) != "new\n" || info.Mode().Perm() != 0o600 {
			t.Fatalf("content %q, mode %v, %v", content, info.Mode(), err)
		}
	})

	t.Run("a symbolic link stays a link and the file it names is the one that changes [verifies SP-1512~1]", func(t *testing.T) {
		dir := t.TempDir()
		target, link := filepath.Join(dir, "dotfiles", "config.toml"), filepath.Join(dir, "config.toml")
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte("old\n"), 0o640); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		if err := files.Replace(link, []byte("new\n")); err != nil {
			t.Fatal(err)
		}
		if stat, err := os.Lstat(link); err != nil || stat.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("the link was replaced: %v, %v", stat, err)
		}
		content, _ := os.ReadFile(target)
		if info, err := os.Stat(target); err != nil || string(content) != "new\n" || info.Mode().Perm() != 0o640 {
			t.Fatalf("target %q, %v, %v", content, info, err)
		}
	})

	t.Run("a file that does not exist is not created [verifies SP-1512~1]", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "absent.toml")
		if err := files.Replace(path, []byte("x")); err == nil {
			t.Fatal("an absent file was replaced")
		}
		if _, err := os.Stat(path); err == nil {
			t.Error("the file was created")
		}
	})
}

func TestFirstLine(t *testing.T) {
	files := adapters.OSFiles{}

	t.Run("the first line is returned without its ending, whatever follows [verifies SP-1510~1]", func(t *testing.T) {
		long := strings.Repeat("x", 3<<20)
		for content, want := range map[string]string{"one\ntwo\n": "one", "one\r\ntwo\r\n": "one", "only": "only", "": "", long + "\nrest": long} {
			path := filepath.Join(t.TempDir(), "rollout.jsonl")
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			got, err := files.FirstLine(path)
			if err != nil || string(got) != want {
				t.Errorf("FirstLine of %.20q = %.20q, %v; want %.20q", content, got, err, want)
			}
		}
	})

	t.Run("a file that does not exist has no first line [verifies SP-1510~1]", func(t *testing.T) {
		got, err := files.FirstLine(filepath.Join(t.TempDir(), "absent.jsonl"))
		if err != nil || got != nil {
			t.Fatalf("got %q, %v", got, err)
		}
	})
}
