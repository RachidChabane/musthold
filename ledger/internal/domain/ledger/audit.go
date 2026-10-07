package ledger

import "fmt"

// Rule names one property the audit checks. They stand in for the deletion and
// force-push rulesets that need a paid plan or an organisation.
type Rule string

// Audit rules.
const (
	// RuleLinearHistory: no merge commit and a single root, so the history is one
	// chain.
	RuleLinearHistory Rule = "linear_history"
	// RuleNoRewrite: no commit deletes or modifies a path.
	RuleNoRewrite Rule = "no_rewrite"
	// RuleContentAddressed: every file is named by what it contains.
	RuleContentAddressed Rule = "content_addressed"
)

// Finding is one broken property.
type Finding struct {
	Rule    Rule
	Subject string
	Detail  string
}

// String renders a finding on one line.
func (f Finding) String() string {
	return fmt.Sprintf("%s: %s: %s", f.Rule, f.Subject, f.Detail)
}

// CheckLinearHistory reports every merge commit and a history that does not have
// exactly one root.
func CheckLinearHistory(commits []CommitRecord) []Finding {
	var findings []Finding
	roots := 0
	for _, commit := range commits {
		switch len(commit.Parents) {
		case 0:
			roots++
		case 1:
		default:
			findings = append(findings, Finding{Rule: RuleLinearHistory, Subject: string(commit.ID), Detail: fmt.Sprintf("merge commit with %d parents", len(commit.Parents))})
		}
	}
	if roots != 1 {
		findings = append(findings, Finding{Rule: RuleLinearHistory, Subject: "history", Detail: fmt.Sprintf("%d root commits, want 1", roots)})
	}
	return findings
}

// CheckNoRewrite reports every path a commit of the history deleted or modified.
func CheckNoRewrite(mutatedPaths []string) []Finding {
	findings := make([]Finding, 0, len(mutatedPaths))
	for _, path := range mutatedPaths {
		findings = append(findings, Finding{Rule: RuleNoRewrite, Subject: path, Detail: "deleted or modified after it was added"})
	}
	return findings
}

// CheckFileName reports a file that is not what its name says it is.
func CheckFileName(path string, content []byte, payloads PayloadReader) []Finding {
	if err := CheckFile(path, content, payloads); err != nil {
		return []Finding{{Rule: RuleContentAddressed, Subject: path, Detail: err.Error()}}
	}
	return nil
}
