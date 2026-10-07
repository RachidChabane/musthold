package app

import (
	"fmt"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/ledger/internal/domain/ledger"
	"github.com/RachidChabane/spec-proof/ledger/internal/domain/refusal"
)

// AuditService checks that the ledger is append-only and content-addressed.
type AuditService struct {
	Ref      LedgerRef
	Files    LedgerFiles
	History  LedgerHistory
	Payloads ledger.PayloadReader
}

// AuditReport lists every broken property, in the order the rules run.
type AuditReport struct {
	Commits  int
	Files    int
	Findings []ledger.Finding
}

// Run checks the history for merges and a single root, every commit for a
// deleted or modified path, and every file at the tip against its name. A ledger
// that breaks a rule is reported, never repaired.
func (s AuditService) Run() (AuditReport, error) {
	tip, exists, err := s.Ref.Tip()
	if err != nil {
		return AuditReport{}, fmt.Errorf("read the ledger tip: %w", err)
	}
	if !exists {
		return AuditReport{}, refusal.Newf("the repository has no %s branch", format.LedgerBranch)
	}
	commits, err := s.History.History(tip)
	if err != nil {
		return AuditReport{}, fmt.Errorf("read the history: %w", err)
	}
	rewrites, err := s.History.Rewrites(tip)
	if err != nil {
		return AuditReport{}, fmt.Errorf("read the rewrites: %w", err)
	}
	files, err := s.Files.Files(tip)
	if err != nil {
		return AuditReport{}, fmt.Errorf("list the ledger: %w", err)
	}
	report := AuditReport{Commits: len(commits), Files: len(files)}
	report.Findings = append(report.Findings, ledger.CheckLinearHistory(commits)...)
	report.Findings = append(report.Findings, ledger.CheckNoRewrite(rewrites)...)
	for _, file := range files {
		content, err := s.Files.ReadBlob(file.ID)
		if err != nil {
			return AuditReport{}, fmt.Errorf("read %s: %w", file.Path, err)
		}
		report.Findings = append(report.Findings, ledger.CheckFileName(file.Path, content, s.Payloads)...)
	}
	return report, nil
}
