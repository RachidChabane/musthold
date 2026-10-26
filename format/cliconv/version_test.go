package cliconv_test

import (
	"runtime/debug"
	"testing"

	"github.com/RachidChabane/musthold/format/cliconv"
	"pgregory.net/rapid"
)

const verifiesVersion = " [verifies SP-1850~1]"

func TestVersionRequest(t *testing.T) {
	t.Run("the lone argument --version asks for the version"+verifiesVersion, func(t *testing.T) {
		if !cliconv.IsVersionRequest([]string{"--version"}) {
			t.Fatal("not recognised")
		}
	})
	t.Run("no other command line asks for the version"+verifiesVersion, func(t *testing.T) {
		for _, arguments := range [][]string{nil, {}, {"version"}, {"-version"}, {"--version", "x"}, {"x", "--version"}, {"--version=1"}} {
			if cliconv.IsVersionRequest(arguments) {
				t.Errorf("%v asks for the version", arguments)
			}
		}
	})
}

func TestResolveVersion(t *testing.T) {
	installed := func(version string) *debug.BuildInfo {
		return &debug.BuildInfo{Main: debug.Module{Version: version}}
	}
	t.Run("the linked value wins over the build information"+verifiesVersion, func(t *testing.T) {
		if got := cliconv.ResolveVersion("1.2.3", installed("v9.9.9")); got != "1.2.3" {
			t.Errorf("version = %q", got)
		}
	})
	t.Run("an installed module version is reported without its v"+verifiesVersion, func(t *testing.T) {
		if got := cliconv.ResolveVersion("", installed("v0.2.0")); got != "0.2.0" {
			t.Errorf("version = %q", got)
		}
		if got := cliconv.ResolveVersion("", installed("v0.2.0-rc.1")); got != "0.2.0-rc.1" {
			t.Errorf("version = %q", got)
		}
	})
	t.Run("a build with no linked and no installed version is a development build"+verifiesVersion, func(t *testing.T) {
		for name, info := range map[string]*debug.BuildInfo{"no information": nil, "empty": installed(""), "devel": installed("(devel)")} {
			if got := cliconv.ResolveVersion("", info); got != cliconv.DevelopmentVersion {
				t.Errorf("%s: version = %q", name, got)
			}
		}
	})
	t.Run("a linked version is reported unchanged whatever it holds"+verifiesVersion, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			linked := rapid.StringMatching(`[0-9a-z.+-]{1,12}`).Draw(t, "linked")
			if got := cliconv.ResolveVersion(linked, nil); got != linked {
				t.Fatalf("version = %q, want %q", got, linked)
			}
		})
	})
}
