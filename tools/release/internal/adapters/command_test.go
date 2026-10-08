package adapters

import (
	"slices"
	"testing"

	"github.com/RachidChabane/spec-proof/tools/release/internal/domain"
)

func TestEnvironmentWith(t *testing.T) {
	t.Run("the caller's own values of the variables a build sets are dropped and the build's are used [verifies SP-1804~1]", func(t *testing.T) {
		caller := []string{"PATH=/usr/bin", "GOFLAGS=-mod=mod", "GOWORK=/tmp/go.work", "CGO_ENABLED=1", "GOOS=plan9", "HOME=/home/x"}
		spec := domain.BuildSpec{Target: domain.Target{OS: domain.OSLinux, Arch: domain.ArchArm64}}
		got := environmentWith(caller, spec.Environment())
		for _, kept := range []string{"PATH=/usr/bin", "HOME=/home/x"} {
			if !slices.Contains(got, kept) {
				t.Errorf("%s was dropped: %v", kept, got)
			}
		}
		for _, dropped := range []string{"GOFLAGS=-mod=mod", "GOWORK=/tmp/go.work", "CGO_ENABLED=1", "GOOS=plan9"} {
			if slices.Contains(got, dropped) {
				t.Errorf("%s survived: %v", dropped, got)
			}
		}
		for _, set := range spec.Environment() {
			if !slices.Contains(got, set) {
				t.Errorf("%s is missing: %v", set, got)
			}
		}
	})
}
