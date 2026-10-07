package app

import (
	"fmt"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/ledger/internal/domain/ledger"
	"github.com/RachidChabane/spec-proof/ledger/internal/domain/refusal"
)

// ExportService materializes the signed proofs and the trust snapshot of the
// ledger tip as plain files, the input of the verifier. Mutants and seeds stay in
// the ledger: the verifier has no use for them.
type ExportService struct {
	Ref   LedgerRef
	Files LedgerFiles
	Sink  FileSink
}

// ExportReport counts the files written.
type ExportReport struct {
	Proofs     int
	TrustFiles int
}

// Run writes every file below proofs/ and trust/ of the ledger tip into the sink.
func (s ExportService) Run() (ExportReport, error) {
	tip, exists, err := s.Ref.Tip()
	if err != nil {
		return ExportReport{}, fmt.Errorf("read the ledger tip: %w", err)
	}
	if !exists {
		return ExportReport{}, refusal.Newf("the repository has no %s branch", format.LedgerBranch)
	}
	files, err := s.Files.Files(tip)
	if err != nil {
		return ExportReport{}, fmt.Errorf("list the ledger: %w", err)
	}
	if err := s.Sink.Prepare(); err != nil {
		return ExportReport{}, err
	}
	var report ExportReport
	for _, file := range files {
		category := ledger.Classify(file.Path)
		if category != ledger.CategoryProof && category != ledger.CategoryTrust {
			continue
		}
		content, err := s.Files.ReadBlob(file.ID)
		if err != nil {
			return ExportReport{}, fmt.Errorf("read %s: %w", file.Path, err)
		}
		if err := s.Sink.WriteFile(file.Path, content); err != nil {
			return ExportReport{}, fmt.Errorf("write %s: %w", file.Path, err)
		}
		if category == ledger.CategoryProof {
			report.Proofs++
		} else {
			report.TrustFiles++
		}
	}
	return report, nil
}
