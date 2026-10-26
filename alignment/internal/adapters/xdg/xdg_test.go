package xdg

import (
	"path/filepath"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/format"
)

const homeDir = "/home/user"

func env(values map[string]string) LookupEnv {
	return func(name string) (string, bool) {
		value, found := values[name]
		return value, found
	}
}

func TestHomes(t *testing.T) {
	t.Run("an absolute XDG_STATE_HOME is used [verifies SP-344~1]", func(t *testing.T) {
		got := StateHome(env(map[string]string{stateHomeVariable: "/var/state"}), homeDir)
		if got != "/var/state" {
			t.Fatalf("StateHome = %q", got)
		}
	})

	t.Run("an absolute XDG_CACHE_HOME is used [verifies SP-344~1]", func(t *testing.T) {
		got := CacheHome(env(map[string]string{cacheHomeVariable: "/var/cache"}), homeDir)
		if got != "/var/cache" {
			t.Fatalf("CacheHome = %q", got)
		}
	})

	t.Run("an unset variable falls back to the default under the home [verifies SP-344~1]", func(t *testing.T) {
		if got := StateHome(env(nil), homeDir); got != filepath.Join(homeDir, ".local", "state") {
			t.Fatalf("StateHome = %q", got)
		}
		if got := CacheHome(env(nil), homeDir); got != filepath.Join(homeDir, ".cache") {
			t.Fatalf("CacheHome = %q", got)
		}
	})

	t.Run("an empty or relative variable is ignored [verifies SP-344~1]", func(t *testing.T) {
		for _, value := range []string{"", "relative/state", "./state"} {
			got := StateHome(env(map[string]string{stateHomeVariable: value}), homeDir)
			if got != filepath.Join(homeDir, ".local", "state") {
				t.Fatalf("StateHome(%q) = %q", value, got)
			}
		}
	})
}

func TestRepoDigest(t *testing.T) {
	t.Run("the digest is the SHA-256 of the cleaned root [verifies SP-340~1]", func(t *testing.T) {
		want := format.SHA256Hex([]byte("/work/project"))
		if got := RepoDigest("/work/project/"); got != want {
			t.Fatalf("RepoDigest = %q, want %q", got, want)
		}
	})

	t.Run("two roots have two digests [verifies SP-340~1]", func(t *testing.T) {
		if RepoDigest("/work/a") == RepoDigest("/work/b") {
			t.Fatal("digests collide")
		}
	})

	t.Run("a digest is always SHA-256 hex and ignores a trailing separator [verifies SP-340~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			root := "/" + rapid.StringMatching(`[a-z0-9]{1,8}(/[a-z0-9]{1,8}){0,4}`).Draw(t, "root")
			digest := RepoDigest(root)
			if !format.IsSHA256Hex(digest) || digest != RepoDigest(root+"/") {
				t.Fatalf("digest %q of %q", digest, root)
			}
		})
	})
}

func TestDirectories(t *testing.T) {
	root := "/work/project"
	digest := RepoDigest(root)

	t.Run("the host directory sits under the state home [verifies SP-340~1]", func(t *testing.T) {
		got := HostDir(env(map[string]string{stateHomeVariable: "/var/state"}), homeDir, root)
		want := filepath.Join("/var/state", "musthold", "alignment", digest, "host")
		if got != want {
			t.Fatalf("HostDir = %q, want %q", got, want)
		}
	})

	t.Run("the cache directory sits under the cache home [verifies SP-346~1]", func(t *testing.T) {
		got := CacheDir(env(nil), homeDir, root)
		if !strings.HasSuffix(got, filepath.Join(".cache", "musthold", digest)) {
			t.Fatalf("CacheDir = %q", got)
		}
	})
}
