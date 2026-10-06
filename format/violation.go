package format

import (
	"fmt"
	"slices"
	"strings"
)

// ViolationCode names one broken invariant of the format. The codes are part of
// the contract: golden vectors state the codes a statement must raise.
type ViolationCode string

// Violation codes.
const (
	CodeStatementType           ViolationCode = "statement_type"
	CodePredicateType           ViolationCode = "predicate_type"
	CodeSubject                 ViolationCode = "subject"
	CodeRequirement             ViolationCode = "requirement"
	CodeMissingField            ViolationCode = "missing_field"
	CodeUnknownValue            ViolationCode = "unknown_value"
	CodeBadDigest               ViolationCode = "bad_digest"
	CodeCacheKeyMismatch        ViolationCode = "cache_key_mismatch"
	CodeUnsorted                ViolationCode = "unsorted"
	CodeJudgedWithoutConfidence ViolationCode = "judged_without_confidence"
	CodeExecutedWithConfidence  ViolationCode = "executed_with_confidence"
	CodeConfidenceRange         ViolationCode = "confidence_range"
	CodeWrongClass              ViolationCode = "wrong_class"
	CodeSentinelNotEquivalent   ViolationCode = "sentinel_not_equivalent"
	CodeNotBlind                ViolationCode = "not_blind"
	CodeNotAdvisory             ViolationCode = "not_advisory"

	CodeUncorroboratedClaim       ViolationCode = "uncorroborated_claim"
	CodeExecutedWithCorroboration ViolationCode = "executed_with_corroboration"
	CodeExecutedWithExecutor      ViolationCode = "executed_with_executor"
	CodeInconsistentAttribution   ViolationCode = "inconsistent_attribution"
)

// Violation is one broken invariant at a JSON path of the statement.
type Violation struct {
	Code    ViolationCode `json:"code"`
	Path    string        `json:"path"`
	Message string        `json:"message"`
}

// ValidationError aggregates every violation found in one statement.
type ValidationError struct {
	Violations []Violation
}

// Error lists the violations, one per line.
func (e *ValidationError) Error() string {
	lines := make([]string, 0, len(e.Violations))
	for _, violation := range e.Violations {
		lines = append(lines, fmt.Sprintf("%s at %s: %s", violation.Code, violation.Path, violation.Message))
	}
	return strings.Join(lines, "\n")
}

// Codes returns the distinct violation codes in ascending order.
func (e *ValidationError) Codes() []ViolationCode {
	seen := map[ViolationCode]bool{}
	for _, violation := range e.Violations {
		seen[violation.Code] = true
	}
	codes := make([]ViolationCode, 0, len(seen))
	for code := range seen {
		codes = append(codes, code)
	}
	slices.Sort(codes)
	return codes
}

// collector gathers violations while a statement is checked.
type collector struct {
	violations []Violation
}

func (c *collector) add(code ViolationCode, path, format string, args ...any) {
	c.violations = append(c.violations, Violation{Code: code, Path: path, Message: fmt.Sprintf(format, args...)})
}

func (c *collector) err() error {
	if len(c.violations) == 0 {
		return nil
	}
	return &ValidationError{Violations: c.violations}
}
