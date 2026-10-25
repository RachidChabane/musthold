package domain

import (
	"slices"
	"strings"
	"time"
)

// Modes and ownership of every archive entry.
const (
	ExecutableMode int64 = 0o755
	FileMode       int64 = 0o644
	ArchiveOwnerID       = 0
)

// ArchiveEntry is one file of a release archive.
type ArchiveEntry struct {
	Name string
	Mode int64
	Data []byte
}

// ArchiveContent is what goes into the archive of a target.
type ArchiveContent struct {
	Binary  []byte
	License []byte
	Readme  []byte
	// Notice is the NOTICE file of the work, shipped in every archive
	// (Apache-2.0 section 4(d)).
	Notice []byte
	// ThirdPartyNotices is the third-party notices file, shipped in every archive.
	ThirdPartyNotices []byte
}

// ArchiveEntries lists the entries of the archive of a target, sorted by name:
// the binary, the licence, the README, the notice and the
// third-party notices, with fixed modes.
func ArchiveEntries(target Target, binary string, content ArchiveContent) []ArchiveEntry {
	entries := []ArchiveEntry{
		{Name: target.ExecutableName(binary), Mode: ExecutableMode, Data: content.Binary},
		{Name: LicenseFile, Mode: FileMode, Data: content.License},
		{Name: ReadmeFile, Mode: FileMode, Data: content.Readme},
		{Name: NoticeFile, Mode: FileMode, Data: content.Notice},
		{Name: ThirdPartyNoticesFile, Mode: FileMode, Data: content.ThirdPartyNotices},
	}
	slices.SortFunc(entries, func(a, b ArchiveEntry) int { return strings.Compare(a.Name, b.Name) })
	return entries
}

// ExpectedEntryNames are the names an archive of a target holds, sorted. The
// archive holds the licence, the README, the NOTICE and the third-party notices.
func ExpectedEntryNames(target Target, binary string) []string {
	names := []string{target.ExecutableName(binary), LicenseFile, ReadmeFile, NoticeFile, ThirdPartyNoticesFile}
	slices.Sort(names)
	return names
}

// CommitTime converts a commit's Unix time, the SOURCE_DATE_EPOCH of the build,
// to the instant every archive entry and SBOM carries.
func CommitTime(epochSeconds int64) time.Time { return time.Unix(epochSeconds, 0).UTC() }
