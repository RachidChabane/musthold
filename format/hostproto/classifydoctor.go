package hostproto

import (
	"fmt"
	"slices"
	"strings"

	"github.com/RachidChabane/spec-proof/format"
)

// ClassifyDiagnosis is what `doctor` says about the classification of a module.
type ClassifyDiagnosis struct {
	// Passed is false when an admission entry of the module was measured on another
	// version of the host than the one that made the latest classification.
	Passed bool
	Detail string
}

// DiagnoseClassification reports the switch, the admission entries of the kind and
// whether each was measured on the version of the host the latest classification in
// the journal was made by.
func DiagnoseClassification(mode ClassifyMode, admission ClassifyAdmission, kind format.Kind, records []Record) ClassifyDiagnosis {
	entries := slices.DeleteFunc(slices.Clone(admission.Entries), func(entry ClassifyAdmissionEntry) bool { return entry.Kind != kind })
	observed := observedCLIVersion(records)
	summary := fmt.Sprintf("mode %s, %d admitted", mode, len(entries))
	if observed != "" {
		summary += ", host " + observed
	}
	var mismatches []string
	for _, entry := range entries {
		if observed != "" && entry.CLIVersion != observed {
			mismatches = append(mismatches, fmt.Sprintf("%s for %s was measured on host %s", entry.Source, entry.Model, entry.CLIVersion))
		}
	}
	if len(mismatches) > 0 {
		return ClassifyDiagnosis{Detail: summary + ": host version mismatch: " + strings.Join(mismatches, "; ")}
	}
	return ClassifyDiagnosis{Passed: true, Detail: summary}
}

// observedCLIVersion is the version of the host that made the latest
// classification the journal records, empty when it records none.
func observedCLIVersion(records []Record) string {
	for _, record := range slices.Backward(records) {
		switch record.Event.Kind {
		case EventClassifyAnswered, EventClassifyShadow, EventClassifyFellThrough:
			if detail, ok := ParseClassifyDetail(record.Event.Detail); ok {
				return detail.CLIVersion
			}
		}
	}
	return ""
}
