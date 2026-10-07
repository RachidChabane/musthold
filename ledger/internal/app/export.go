package app

import (
	"fmt"
	"slices"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/ledger/internal/domain/ledger"
	"github.com/RachidChabane/spec-proof/ledger/internal/domain/refusal"
)

// ExportService materializes the signed proofs and the newest trust snapshot of the
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

// Run writes every file below proofs/ of the ledger tip into the sink, and the files
// of the newest trust snapshot flat below trust/, as a TUF mirror serves them.
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
	snapshot, err := s.newestSnapshot(files)
	if err != nil {
		return ExportReport{}, err
	}
	if err := s.Sink.Prepare(); err != nil {
		return ExportReport{}, err
	}
	var report ExportReport
	for _, file := range files {
		target, selected, err := exportedPath(file.Path, snapshot)
		if err != nil {
			return ExportReport{}, err
		}
		if !selected {
			continue
		}
		content, err := s.Files.ReadBlob(file.ID)
		if err != nil {
			return ExportReport{}, fmt.Errorf("read %s: %w", file.Path, err)
		}
		if err := s.Sink.WriteFile(target, content); err != nil {
			return ExportReport{}, fmt.Errorf("write %s: %w", file.Path, err)
		}
		if ledger.Classify(file.Path) == ledger.CategoryProof {
			report.Proofs++
		} else {
			report.TrustFiles++
		}
	}
	return report, nil
}

// exportedPath says whether a ledger file is exported and where: proofs keep their
// path, the files of the selected snapshot lose their snapshot id, everything else
// stays in the ledger.
func exportedPath(path, snapshot string) (target string, selected bool, err error) {
	switch ledger.Classify(path) {
	case ledger.CategoryProof:
		return path, true, nil
	case ledger.CategoryTrust:
		id, relative, ok := ledger.ParseTrustPath(path)
		if !ok {
			return "", false, refusal.Newf("%s is not a file of a trust snapshot", path)
		}
		return format.TrustDirectory + "/" + relative, id == snapshot, nil
	default:
		return "", false, nil
	}
}

// newestSnapshot returns the id of the trust snapshot to export, or an empty id when
// the ledger holds none.
func (s ExportService) newestSnapshot(files []ledger.FileEntry) (string, error) {
	timestamps := map[string][]byte{}
	for _, file := range files {
		id, relative, ok := ledger.ParseTrustPath(file.Path)
		if !ok || relative != format.TrustTimestampFile {
			continue
		}
		content, err := s.Files.ReadBlob(file.ID)
		if err != nil {
			return "", fmt.Errorf("read %s: %w", file.Path, err)
		}
		timestamps[id] = content
	}
	id, found, err := ledger.NewestSnapshot(timestamps)
	if err != nil {
		return "", err
	}
	if !found && slices.ContainsFunc(files, func(file ledger.FileEntry) bool { return ledger.Classify(file.Path) == ledger.CategoryTrust }) {
		return "", refusal.Newf("the ledger holds trust files but no snapshot with a %s", format.TrustTimestampFile)
	}
	return id, nil
}
