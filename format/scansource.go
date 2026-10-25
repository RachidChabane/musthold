package format

import (
	"regexp"
	"strconv"
)

// Names of the quotable sources of a scan: the README at head, the description
// of the change and each issue it closes. The set is closed; a diff is never a
// source.
const (
	// ScanSourceReadme names the README of the scanned commit.
	ScanSourceReadme = "README.md"
	// ScanSourceDescription names the title and body of the change as the forge
	// returned them.
	ScanSourceDescription = "description"
	scanSourceIssuePrefix = "issue #"
)

var scanSourceIssuePattern = regexp.MustCompile(`^` + scanSourceIssuePrefix + `[1-9][0-9]*$`)

// ScanSourceIssue names the source of the closing issue with the given number.
func ScanSourceIssue(number int) string { return scanSourceIssuePrefix + strconv.Itoa(number) }

// IsScanSourceName reports whether name is one of the closed set of source
// names: the README, the description or a closing issue.
func IsScanSourceName(name string) bool {
	return name == ScanSourceReadme || name == ScanSourceDescription || scanSourceIssuePattern.MatchString(name)
}
