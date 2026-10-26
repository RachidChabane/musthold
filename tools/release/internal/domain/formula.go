package domain

import (
	"fmt"
	"strings"
)

const formulaSuffix = ".rb"

// FormulaFileName is the name of the Homebrew formula of a binary.
func FormulaFileName(binary string) string { return binary + formulaSuffix }

// formulaPlatforms are the targets a formula installs: Homebrew runs on macOS and
// Linux.
var formulaPlatforms = []struct {
	Target Target
	Block  string
	Arch   string
}{
	{Target{OSDarwin, ArchArm64}, "on_macos", "on_arm"},
	{Target{OSDarwin, ArchAmd64}, "on_macos", "on_intel"},
	{Target{OSLinux, ArchArm64}, "on_linux", "on_arm"},
	{Target{OSLinux, ArchAmd64}, "on_linux", "on_intel"},
}

// DownloadURL is where the release of a tag serves an asset. The tag keeps its
// slash: GitHub resolves it.
func DownloadURL(tag Tag, assetName string) string {
	return RepositoryURL + "/releases/download/" + tag.String() + "/" + assetName
}

// RenderFormula renders the Homebrew formula of a binary release: one url and
// sha256 per operating system and architecture, read from the checksum list.
func RenderFormula(tag Tag, sums []Checksum) (string, error) {
	module := tag.Module
	if !module.ShipsBinary() {
		return "", fmt.Errorf("%s ships no binary: it has no formula", module.Name)
	}
	var out strings.Builder
	out.WriteString("class " + formulaClassName(module.Binary) + " < Formula\n")
	fmt.Fprintf(&out, "  desc %q\n", module.Summary)
	fmt.Fprintf(&out, "  homepage %q\n", RepositoryURL)
	fmt.Fprintf(&out, "  version %q\n", tag.Semver())
	fmt.Fprintf(&out, "  license %q\n", LicenseIdentifier)
	for _, block := range []string{"on_macos", "on_linux"} {
		out.WriteString("\n  " + block + " do\n")
		for _, platform := range formulaPlatforms {
			if platform.Block != block {
				continue
			}
			archive := platform.Target.ArchiveName(module.Binary, tag.Version)
			digest, found := ChecksumFor(sums, archive)
			if !found {
				return "", fmt.Errorf("%s holds no checksum for %s", ChecksumsFileName, archive)
			}
			fmt.Fprintf(&out, "    %s do\n      url %q\n      sha256 %q\n    end\n", platform.Arch, DownloadURL(tag, archive), digest)
		}
		out.WriteString("  end\n")
	}
	out.WriteString("\n  def install\n")
	fmt.Fprintf(&out, "    bin.install %q\n  end\n", module.Binary)
	out.WriteString("\n  test do\n")
	fmt.Fprintf(&out, "    assert_equal version.to_s, shell_output(\"#{bin}/%s --version\").strip\n  end\nend\n", module.Binary)
	return out.String(), nil
}

// formulaClassName turns a binary name into the class name Homebrew expects:
// musthold-dp becomes MustholdDp.
func formulaClassName(binary string) string {
	var name strings.Builder
	for _, word := range strings.Split(binary, "-") {
		if word != "" {
			name.WriteString(strings.ToUpper(word[:1]) + word[1:])
		}
	}
	return name.String()
}
