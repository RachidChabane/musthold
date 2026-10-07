// Package xdg resolves where the module keeps its state and its caches: the host
// store under the XDG state home and the mutant vault and fact memo under the XDG
// cache home, both addressed by a digest of the project root.
package xdg

import (
	"path/filepath"

	"github.com/RachidChabane/spec-proof/format"
)

const (
	stateHomeVariable = "XDG_STATE_HOME"
	cacheHomeVariable = "XDG_CACHE_HOME"
	defaultStateHome  = ".local/state"
	defaultCacheHome  = ".cache"
	suiteDirectory    = "spec-proof"
	moduleDirectory   = "completeness"
	hostDirectory     = "host"
)

// LookupEnv reads an environment variable the way os.LookupEnv does.
type LookupEnv func(name string) (string, bool)

// StateHome is the base state directory: $XDG_STATE_HOME when it is an absolute
// path, `$HOME/.local/state` otherwise.
func StateHome(lookup LookupEnv, home string) string {
	return resolve(lookup, stateHomeVariable, home, defaultStateHome)
}

// CacheHome is the base cache directory: $XDG_CACHE_HOME when it is an absolute
// path, `$HOME/.cache` otherwise.
func CacheHome(lookup LookupEnv, home string) string {
	return resolve(lookup, cacheHomeVariable, home, defaultCacheHome)
}

func resolve(lookup LookupEnv, variable, home, fallback string) string {
	if value, found := lookup(variable); found && filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	return filepath.Join(home, filepath.FromSlash(fallback))
}

// RepoDigest identifies a project: the SHA-256 in hex of its cleaned absolute
// root path.
func RepoDigest(absRoot string) string {
	return format.SHA256Hex([]byte(filepath.Clean(absRoot)))
}

// HostDir is where the host store of the project lives:
// `<state>/spec-proof/discpower/<repo-digest>/host`.
func HostDir(lookup LookupEnv, home, absRoot string) string {
	return filepath.Join(StateHome(lookup, home), suiteDirectory, moduleDirectory, RepoDigest(absRoot), hostDirectory)
}

// CacheDir is where the local caches of the project live:
// `<cache>/spec-proof/<repo-digest>`.
func CacheDir(lookup LookupEnv, home, absRoot string) string {
	return filepath.Join(CacheHome(lookup, home), suiteDirectory, RepoDigest(absRoot))
}
