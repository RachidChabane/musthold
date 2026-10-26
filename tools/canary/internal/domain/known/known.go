// Package known is the registry of checks whose INCONCLUSIVE verdict is declared
// and expected on a given version of a CLI. A declared verdict is still reported
// as INCONCLUSIVE; the registry only lets the exit code of a night say that
// nothing undeclared is open.
package known

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/domain/verdict"
)

// DeclaredLayout is the layout of the date an entry was declared on.
const DeclaredLayout = "2006-01-02"

// versionPattern finds the version number a CLI prints in its version line.
var versionPattern = regexp.MustCompile(`\d+\.\d+\.\d+`)

// Entry declares that a check is INCONCLUSIVE, for a reason, on one exact version
// of one harness's CLI.
type Entry struct {
	Harness    string `json:"harness"`
	CLIVersion string `json:"cli_version"`
	Check      string `json:"check"`
	// Reason is a substring of the summary of the verdict the entry covers.
	Reason string `json:"reason"`
	// Declared is the date the entry was declared, as YYYY-MM-DD.
	Declared  string `json:"declared"`
	Reference string `json:"reference"`
}

// Registry is the declared known INCONCLUSIVE checks. The zero value declares none.
type Registry struct {
	entries []Entry
}

type document struct {
	Entries *[]Entry `json:"entries"`
}

// ErrMalformed is wrapped by every error Parse returns.
var ErrMalformed = errors.New("malformed known-inconclusive registry")

// Parse reads and validates a registry: a JSON object with an `entries` array
// whose members carry exactly the fields of Entry, every one non-empty, the date
// in the layout YYYY-MM-DD.
func Parse(data []byte) (Registry, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var doc document
	if err := decoder.Decode(&doc); err != nil {
		return Registry{}, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	if decoder.More() {
		return Registry{}, fmt.Errorf("%w: data after the object", ErrMalformed)
	}
	if doc.Entries == nil {
		return Registry{}, fmt.Errorf("%w: no entries member", ErrMalformed)
	}
	for index, entry := range *doc.Entries {
		if err := entry.validate(); err != nil {
			return Registry{}, fmt.Errorf("%w: entry %d: %v", ErrMalformed, index, err)
		}
	}
	return Registry{entries: *doc.Entries}, nil
}

func (e Entry) validate() error {
	required := []struct{ name, value string }{
		{"harness", e.Harness}, {"cli_version", e.CLIVersion}, {"check", e.Check},
		{"reason", e.Reason}, {"declared", e.Declared}, {"reference", e.Reference},
	}
	for _, field := range required {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("%s is empty", field.name)
		}
	}
	if _, err := time.Parse(DeclaredLayout, e.Declared); err != nil {
		return fmt.Errorf("declared %q is not a %s date", e.Declared, DeclaredLayout)
	}
	if CLIVersion(e.CLIVersion) != e.CLIVersion {
		return fmt.Errorf("cli_version %q is not an exact version number", e.CLIVersion)
	}
	return nil
}

// Note is the line that tells a reader the entry covers a verdict.
func (e Entry) Note() string {
	return fmt.Sprintf("known: declared %s, %s", e.Declared, e.Reference)
}

// CLIVersion extracts the version number from the version line a CLI printed.
func CLIVersion(line string) string { return versionPattern.FindString(line) }

// Annotate marks each INCONCLUSIVE verdict of the report that an entry matches
// exactly on harness, recorded CLI version, check name and reason. It changes no
// status. A verdict of another status, or with no matching entry, stays as it is.
func (r Registry) Annotate(report verdict.Report, versionLine string) verdict.Report {
	version := CLIVersion(versionLine)
	annotated := make([]verdict.Verdict, len(report.Verdicts))
	for index, v := range report.Verdicts {
		if entry, matched := r.match(report.Harness, version, v); matched {
			v.Known = entry.Note()
		}
		annotated[index] = v
	}
	report.Verdicts = annotated
	return report
}

func (r Registry) match(harness, version string, v verdict.Verdict) (Entry, bool) {
	if v.Status != verdict.Inconclusive || version == "" {
		return Entry{}, false
	}
	for _, entry := range r.entries {
		if entry.Harness == harness && entry.CLIVersion == version && entry.Check == v.Check && strings.Contains(v.Summary, entry.Reason) {
			return entry, true
		}
	}
	return Entry{}, false
}
