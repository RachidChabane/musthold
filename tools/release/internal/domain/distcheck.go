package domain

import (
	"fmt"
	"slices"
	"time"
)

// RuleDist is the rule name of the findings of a dist check.
const RuleDist = "dist"

// InstallerFileName is the verified installer, which a release ships next to its
// archives.
const InstallerFileName = "install"

// EntryFacts is one entry of a release archive as the archive records it.
type EntryFacts struct {
	Name    string
	Mode    int64
	UID     int
	GID     int
	ModTime time.Time
}

// BinaryFacts is what the build information of a released binary says.
type BinaryFacts struct {
	MainPath string
	// Settings are the build settings by key. A build with -trimpath does not record
	// its link flags, so the check looks for the linked values in the binary.
	Settings map[string]string
	// Contains reports whether the binary holds a string.
	Contains func(needle string) bool
}

// DistFile is what is known about one file of a dist directory.
type DistFile struct {
	Digest string
	// Unreadable is why an archive could not be opened, empty when it could.
	Unreadable string
	Entries    []EntryFacts
	Binary     *BinaryFacts
	SBOM       *SPDXHeader
}

// DistFacts is a dist directory as read from the disk.
type DistFacts struct {
	Tag        Tag
	Commit     string
	CommitTime time.Time
	Targets    []Target
	// Checksums is the content of SHA256SUMS, empty when the file is missing.
	Checksums string
	// Files are the files of the directory by name, SHA256SUMS aside.
	Files map[string]DistFile
}

// Build settings the check reads.
const (
	settingTrimpath = "-trimpath"
	settingVCS      = "vcs"
	settingTrue     = "true"
)

// auxiliaryFiles are files a dist directory may hold that SHA256SUMS does not
// list: they are made from it after the build.
func auxiliaryFiles(binary string) []string {
	return []string{SignatureBundleName, FormulaFileName(binary)}
}

// VerifyDist checks a finished dist directory against the release it must hold:
// every file of the matrix is listed in SHA256SUMS with its digest, nothing else
// is listed or present, each archive holds the expected entries with fixed modes,
// owner and time, each binary was built as the release builds it, and each SBOM
// names its archive.
func VerifyDist(facts DistFacts) []Finding {
	var findings []Finding
	report := func(format string, arguments ...any) {
		findings = append(findings, Finding{RuleDist, ScopeAlways, fmt.Sprintf(format, arguments...)})
	}
	module := facts.Tag.Module
	sums, err := ParseChecksums(facts.Checksums)
	if err != nil || facts.Checksums == "" {
		if err == nil {
			err = fmt.Errorf("%s is missing or empty", ChecksumsFileName)
		}
		return []Finding{{RuleDist, ScopeAlways, err.Error()}}
	}
	expected := append(ReleaseFiles(module.Binary, facts.Tag.Version, facts.Targets), InstallerFileName)
	for _, name := range expected {
		file, present := facts.Files[name]
		listed, isListed := ChecksumFor(sums, name)
		switch {
		case !present:
			report("%s is missing from the dist directory", name)
		case !isListed:
			report("%s does not list %s", ChecksumsFileName, name)
		case file.Digest != listed:
			report("%s has digest %s, %s lists %s", name, file.Digest, ChecksumsFileName, listed)
		}
	}
	for _, sum := range sums {
		if !slices.Contains(expected, sum.Name) {
			report("%s lists %s, which a release of %s does not ship", ChecksumsFileName, sum.Name, facts.Tag)
		}
	}
	for name := range facts.Files {
		if !slices.Contains(expected, name) && !slices.Contains(auxiliaryFiles(module.Binary), name) {
			report("%s is in the dist directory and is not part of the release", name)
		}
	}
	for _, target := range facts.Targets {
		archive := target.ArchiveName(module.Binary, facts.Tag.Version)
		if file, present := facts.Files[archive]; present {
			findings = append(findings, verifyArchive(facts, target, archive, file)...)
		}
		sbom := SBOMName(archive)
		if file, present := facts.Files[sbom]; present {
			findings = append(findings, verifySBOM(facts, archive, sbom, file)...)
		}
	}
	return findings
}

func verifyArchive(facts DistFacts, target Target, archive string, file DistFile) []Finding {
	var findings []Finding
	report := func(format string, arguments ...any) {
		findings = append(findings, Finding{RuleDist, ScopeAlways, archive + ": " + fmt.Sprintf(format, arguments...)})
	}
	if file.Unreadable != "" {
		report("cannot be read: %s", file.Unreadable)
		return findings
	}
	binary := facts.Tag.Module.Binary
	var names []string
	for _, entry := range file.Entries {
		names = append(names, entry.Name)
		wantMode := FileMode
		if entry.Name == target.ExecutableName(binary) {
			wantMode = ExecutableMode
		}
		if entry.Mode != wantMode {
			report("entry %s has mode %o, want %o", entry.Name, entry.Mode, wantMode)
		}
		if entry.UID != ArchiveOwnerID || entry.GID != ArchiveOwnerID {
			report("entry %s is owned by %d:%d, want %d:%d", entry.Name, entry.UID, entry.GID, ArchiveOwnerID, ArchiveOwnerID)
		}
		if !entry.ModTime.Equal(facts.CommitTime) {
			report("entry %s has time %s, want the commit time %s", entry.Name, entry.ModTime.UTC().Format(time.RFC3339), facts.CommitTime.Format(time.RFC3339))
		}
	}
	if want := ExpectedEntryNames(target, binary); !slices.Equal(names, want) {
		report("entries are %v, want %v in that order", names, want)
	}
	if file.Binary == nil {
		report("the binary has no readable build information")
		return findings
	}
	for _, problem := range binaryProblems(facts, target, *file.Binary) {
		report("%s", problem)
	}
	return findings
}

func binaryProblems(facts DistFacts, target Target, binary BinaryFacts) []string {
	var problems []string
	module := facts.Tag.Module
	if want := module.ModulePath() + "/cmd/" + module.Binary; binary.MainPath != want {
		problems = append(problems, fmt.Sprintf("the main package is %s, want %s", binary.MainPath, want))
	}
	for variable, linked := range map[string]string{LinkedVersionVariable: facts.Tag.Semver(), LinkedCommitVariable: facts.Commit} {
		if binary.Contains == nil || !binary.Contains(linked) {
			problems = append(problems, fmt.Sprintf("the binary does not hold %q, the value linked into %s", linked, variable))
		}
	}
	wanted := map[string]string{EnvOS: target.OS, EnvArch: target.Arch, EnvCgo: cgoOff, settingTrimpath: settingTrue}
	for key, want := range wanted {
		if got := binary.Settings[key]; got != want {
			problems = append(problems, fmt.Sprintf("build setting %s is %q, want %q", key, got, want))
		}
	}
	if _, stamped := binary.Settings[settingVCS]; stamped {
		problems = append(problems, "the binary carries VCS stamping, which the release build turns off")
	}
	return problems
}

func verifySBOM(facts DistFacts, archive, sbom string, file DistFile) []Finding {
	var findings []Finding
	report := func(format string, arguments ...any) {
		findings = append(findings, Finding{RuleDist, ScopeAlways, sbom + ": " + fmt.Sprintf(format, arguments...)})
	}
	if file.SBOM == nil {
		report("the SBOM cannot be read")
		return findings
	}
	if file.SBOM.Version != spdxVersion {
		report("spdxVersion is %q, want %q", file.SBOM.Version, spdxVersion)
	}
	if want := SBOMNamespace(facts.Tag, archive); file.SBOM.Namespace != want {
		report("documentNamespace is %q, want %q", file.SBOM.Namespace, want)
	}
	if want := SPDXCreated(facts.CommitTime); file.SBOM.Created != want {
		report("created is %q, want the commit time %q", file.SBOM.Created, want)
	}
	return findings
}
