package domain

import "fmt"

// Operating systems and architectures the release builds for.
const (
	OSLinux   = "linux"
	OSDarwin  = "darwin"
	OSWindows = "windows"

	ArchAmd64 = "amd64"
	ArchArm64 = "arm64"
)

// ArchiveFormat is the container of a release archive.
type ArchiveFormat string

// The archive formats.
const (
	FormatTarGz ArchiveFormat = "tar.gz"
	FormatZip   ArchiveFormat = "zip"
)

const (
	windowsExecutableSuffix = ".exe"
	sbomSuffix              = ".spdx.json"
	targetSeparator         = "/"
)

// Target is one operating system and architecture.
type Target struct{ OS, Arch string }

// String spells the target as Go does: os/arch.
func (t Target) String() string { return t.OS + targetSeparator + t.Arch }

// Targets returns the build matrix: linux, darwin and windows, each on amd64 and
// arm64.
func Targets() []Target {
	var matrix []Target
	for _, system := range []string{OSLinux, OSDarwin, OSWindows} {
		for _, arch := range []string{ArchAmd64, ArchArm64} {
			matrix = append(matrix, Target{OS: system, Arch: arch})
		}
	}
	return matrix
}

// ParseTarget parses an os/arch pair of the build matrix.
func ParseTarget(text string) (Target, error) {
	for _, target := range Targets() {
		if target.String() == text {
			return target, nil
		}
	}
	return Target{}, fmt.Errorf("target %q is not one of the build matrix", text)
}

// ExecutableName is the file name of the binary on the target.
func (t Target) ExecutableName(binary string) string {
	if t.OS == OSWindows {
		return binary + windowsExecutableSuffix
	}
	return binary
}

// Format is the archive format of the target: zip for Windows, tar.gz otherwise.
func (t Target) Format() ArchiveFormat {
	if t.OS == OSWindows {
		return FormatZip
	}
	return FormatTarGz
}

// ArchiveName is `<bin>_<semver>_<os>_<arch>.tar.gz`, or `.zip` for Windows.
func (t Target) ArchiveName(binary string, version Version) string {
	return fmt.Sprintf("%s_%s_%s_%s.%s", binary, version, t.OS, t.Arch, t.Format())
}

// SBOMName is the name of the SBOM that describes an archive.
func SBOMName(archiveName string) string { return archiveName + sbomSuffix }

// ReleaseFiles lists the file names a finished dist directory holds for a tag,
// without the checksum file: each archive of the matrix and its SBOM.
func ReleaseFiles(binary string, version Version, targets []Target) []string {
	var names []string
	for _, target := range targets {
		archive := target.ArchiveName(binary, version)
		names = append(names, archive, SBOMName(archive))
	}
	return names
}
