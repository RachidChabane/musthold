package adapters

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/RachidChabane/spec-proof/tools/release/internal/domain"
)

const sbomExtension = ".spdx.json"

// DistDirectory reads a dist directory from the disk.
type DistDirectory struct{}

// ReadDist reads every regular file of the directory: its digest, and for an
// archive its entries and the build information of its binary, and for an SBOM its
// header. SHA256SUMS is read into Checksums and is not a file of the release.
func (DistDirectory) ReadDist(dir string, tag domain.Tag, targets []domain.Target) (domain.DistFacts, error) {
	facts := domain.DistFacts{Files: map[string]domain.DistFile{}}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return facts, err
	}
	archives := map[string]domain.Target{}
	for _, target := range targets {
		archives[target.ArchiveName(tag.Module.Binary, tag.Version)] = target
	}
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return facts, err
		}
		if entry.Name() == domain.ChecksumsFileName {
			facts.Checksums = string(data)
			continue
		}
		file := domain.DistFile{Digest: domain.DigestOf(data)}
		if target, isArchive := archives[entry.Name()]; isArchive {
			if err := readArchive(&file, target, tag.Module.Binary, data); err != nil {
				file = domain.DistFile{Digest: file.Digest, Unreadable: err.Error()}
			}
		}
		if strings.HasSuffix(entry.Name(), sbomExtension) {
			if header, err := domain.ParseSPDXHeader(data); err == nil {
				file.SBOM = &header
			}
		}
		facts.Files[entry.Name()] = file
	}
	return facts, nil
}

// readArchive fills the entries of an archive and the build information of its
// binary.
func readArchive(file *domain.DistFile, target domain.Target, binary string, archive []byte) error {
	entries, err := unpack(target.Format(), archive)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		file.Entries = append(file.Entries, entry.facts)
		if entry.facts.Name == target.ExecutableName(binary) {
			if facts, err := readBinaryFacts(entry.data); err == nil {
				file.Binary = facts
			}
		}
	}
	return nil
}
