package domain_test

import (
	"slices"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/tools/release/internal/domain"
)

const (
	verifiesMatrix   = " [verifies SP-1803~1]"
	verifiesBuild    = " [verifies SP-1804~1]"
	verifiesChecksum = " [verifies SP-1806~1]"
)

func TestBuildMatrix(t *testing.T) {
	t.Run("the matrix is linux, darwin and windows on amd64 and arm64"+verifiesMatrix, func(t *testing.T) {
		var got []string
		for _, target := range domain.Targets() {
			got = append(got, target.String())
		}
		want := []string{"linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64", "windows/amd64", "windows/arm64"}
		if !slices.Equal(got, want) {
			t.Fatalf("matrix = %v", got)
		}
	})

	t.Run("an archive is a tar.gz, or a zip for windows, named after the binary, version and target"+verifiesMatrix, func(t *testing.T) {
		version := domain.Version{Major: 0, Minor: 2, Patch: 0}
		cases := map[string]string{
			"linux/amd64":   "musthold-dp_0.2.0_linux_amd64.tar.gz",
			"darwin/arm64":  "musthold-dp_0.2.0_darwin_arm64.tar.gz",
			"windows/amd64": "musthold-dp_0.2.0_windows_amd64.zip",
		}
		for text, want := range cases {
			target, err := domain.ParseTarget(text)
			if err != nil {
				t.Fatal(err)
			}
			if got := target.ArchiveName("musthold-dp", version); got != want {
				t.Errorf("%s: %q, want %q", text, got, want)
			}
		}
	})

	t.Run("the executable of windows ends in .exe and no other does"+verifiesMatrix, func(t *testing.T) {
		for _, target := range domain.Targets() {
			if got := strings.HasSuffix(target.ExecutableName("tool"), ".exe"); got != (target.OS == domain.OSWindows) {
				t.Errorf("%s: .exe = %v", target, got)
			}
		}
	})

	t.Run("a target outside the matrix is refused"+verifiesMatrix, func(t *testing.T) {
		for _, text := range []string{"", "linux", "linux/386", "plan9/amd64", "linux/amd64/x"} {
			if _, err := domain.ParseTarget(text); err == nil {
				t.Errorf("%q parsed", text)
			}
		}
	})

	t.Run("no two targets, versions or binaries share an archive name"+verifiesMatrix, func(t *testing.T) {
		binaries := []string{}
		for _, module := range domain.Modules() {
			if module.Binary != "" {
				binaries = append(binaries, module.Binary)
			}
		}
		rapid.Check(t, func(t *rapid.T) {
			draw := func(label string) (string, domain.Target, domain.Version) {
				binary := rapid.SampledFrom(binaries).Draw(t, label+" binary")
				target := rapid.SampledFrom(domain.Targets()).Draw(t, label+" target")
				version := domain.Version{Major: rapid.IntRange(0, 4).Draw(t, label+" major"), Minor: rapid.IntRange(0, 4).Draw(t, label+" minor"), Patch: rapid.IntRange(0, 4).Draw(t, label+" patch")}
				return binary, target, version
			}
			binaryA, targetA, versionA := draw("a")
			binaryB, targetB, versionB := draw("b")
			sameInputs := binaryA == binaryB && targetA == targetB && versionA == versionB
			sameName := targetA.ArchiveName(binaryA, versionA) == targetB.ArchiveName(binaryB, versionB)
			if sameInputs != sameName {
				t.Fatalf("inputs equal %v, names equal %v", sameInputs, sameName)
			}
		})
	})

	t.Run("the files of a release are an archive and its SBOM per target"+verifiesMatrix, func(t *testing.T) {
		files := domain.ReleaseFiles("tool", domain.Version{Major: 1}, domain.Targets())
		if len(files) != 2*len(domain.Targets()) || files[1] != files[0]+".spdx.json" {
			t.Fatalf("files = %v", files)
		}
	})
}

func TestBuildCommand(t *testing.T) {
	tag := mustTag(t, "discpower/v0.2.0")
	target, _ := domain.ParseTarget("windows/arm64")
	spec := domain.BuildSpec{Tag: tag, Commit: "abc123", Target: target, Output: "/dist/out.exe"}

	t.Run("the build runs outside the workspace with cgo off, the pinned toolchain and read-only modules"+verifiesBuild, func(t *testing.T) {
		want := []string{"GOWORK=off", "CGO_ENABLED=0", "GOTOOLCHAIN=go1.26.4", "GOFLAGS=-mod=readonly", "GOOS=windows", "GOARCH=arm64"}
		if got := spec.Environment(); !slices.Equal(got, want) {
			t.Fatalf("environment = %v", got)
		}
	})

	t.Run("the command trims paths, turns VCS stamping off and links the version and commit"+verifiesBuild, func(t *testing.T) {
		want := []string{
			"build", "-trimpath", "-buildvcs=false",
			"-ldflags", "-s -w -buildid= -X main.version=0.2.0 -X main.commit=abc123",
			"-o", "/dist/out.exe", "./cmd/musthold-dp",
		}
		if got := spec.Arguments(); !slices.Equal(got, want) {
			t.Fatalf("arguments = %v", got)
		}
	})

	t.Run("the checks run outside the workspace on the pinned toolchain"+verifiesBuild, func(t *testing.T) {
		want := []string{"GOWORK=off", "GOTOOLCHAIN=go1.26.4", "GOFLAGS=-mod=readonly"}
		if got := domain.CheckEnvironment(); !slices.Equal(got, want) {
			t.Fatalf("environment = %v", got)
		}
	})
}

func TestChecksums(t *testing.T) {
	t.Run("a list renders as sorted `<sha256>  <name>` lines"+verifiesChecksum, func(t *testing.T) {
		digestA, digestB := strings.Repeat("a", 64), strings.Repeat("b", 64)
		got := domain.RenderChecksums([]domain.Checksum{{Digest: digestB, Name: "b.tar.gz"}, {Digest: digestA, Name: "a.tar.gz"}})
		want := digestA + "  a.tar.gz\n" + digestB + "  b.tar.gz\n"
		if got != want {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("reading a rendered list gives the list back"+verifiesChecksum, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			names := rapid.SliceOfNDistinct(rapid.StringMatching(`[A-Za-z0-9][A-Za-z0-9._+-]{0,12}`), 0, 8, func(name string) string { return name }).Draw(t, "names")
			var sums []domain.Checksum
			for _, name := range names {
				sums = append(sums, domain.Checksum{Digest: domain.DigestOf([]byte(name)), Name: name})
			}
			parsed, err := domain.ParseChecksums(domain.RenderChecksums(sums))
			if err != nil || len(parsed) != len(sums) {
				t.Fatalf("parsed %v, error %v", parsed, err)
			}
			for _, sum := range sums {
				if digest, found := domain.ChecksumFor(parsed, sum.Name); !found || digest != sum.Digest {
					t.Fatalf("%s: %q %v", sum.Name, digest, found)
				}
			}
		})
	})

	t.Run("a malformed digest, a malformed or repeated name and a missing line break are refused"+verifiesChecksum, func(t *testing.T) {
		good := strings.Repeat("a", 64)
		for name, text := range map[string]string{
			"short digest":     strings.Repeat("a", 63) + "  x\n",
			"upper-case":       strings.Repeat("A", 64) + "  x\n",
			"one space":        good + " x\n",
			"path in the name": good + "  dir/x\n",
			"empty name":       good + "  \n",
			"repeated name":    good + "  x\n" + good + "  x\n",
			"no final newline": good + "  x",
			"leading dot":      good + "  .hidden\n",
		} {
			if _, err := domain.ParseChecksums(text); err == nil {
				t.Errorf("%s: accepted", name)
			}
		}
	})
}
