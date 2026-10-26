// Package proofsource reads the Statements another module wrote for a criterion
// from the project: the verified set the verifier wrote, then the local cache of
// unsigned Statements. A directory is searched whatever module name it carries,
// and a file counts only when its predicate type is the kind asked for.
package proofsource

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/RachidChabane/musthold/alignment/internal/app/ports"
	"github.com/RachidChabane/musthold/alignment/internal/domain/criterion"
	"github.com/RachidChabane/musthold/format"
)

// Locations of the Statements inside a project.
const (
	stateDirectory    = ".musthold"
	verifiedDirectory = "verified"
	cacheDirectory    = "cache"
)

var _ ports.ProofSource = Source{}

// Source implements ports.ProofSource over a project root.
type Source struct {
	Root string
}

// Candidates lists the Statements of the kind written for the criterion: those of
// the verified set first, then those of the local cache, each directory the most recent first.
func (s Source) Candidates(kind format.Kind, ref criterion.Ref) ([]ports.ProofFile, error) {
	verified, err := s.read(kind, ports.OriginVerified, filepath.Join(s.Root, stateDirectory, verifiedDirectory), format.SubjectKey(ref.String()))
	if err != nil {
		return nil, err
	}
	cached, err := s.read(kind, ports.OriginLocalCache, filepath.Join(s.Root, stateDirectory, cacheDirectory), ref.String())
	if err != nil {
		return nil, err
	}
	return slices.Concat(verified, cached), nil
}

// read lists `<base>/<module>/<subject>/*.json` for every module directory.
func (s Source) read(kind format.Kind, origin ports.ProofOrigin, base, subject string) ([]ports.ProofFile, error) {
	modules, err := os.ReadDir(base)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", base, err)
	}
	var found []ports.ProofFile
	for _, module := range modules {
		if !module.IsDir() {
			continue
		}
		files, err := readKind(kind, origin, filepath.Join(base, module.Name(), subject))
		if err != nil {
			return nil, err
		}
		found = append(found, files...)
	}
	return found, nil
}

func readKind(kind format.Kind, origin ports.ProofOrigin, directory string) ([]ports.ProofFile, error) {
	entries, err := os.ReadDir(directory)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", directory, err)
	}
	var found []dated
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), format.StatementSuffix) {
			continue
		}
		content, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", entry.Name(), err)
		}
		info, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", entry.Name(), err)
		}
		if predicateKind(content) == kind {
			found = append(found, dated{name: entry.Name(), modified: info.ModTime(), file: ports.ProofFile{Origin: origin, Bytes: content}})
		}
	}
	return newestFirst(found), nil
}

// dated is a Statement file with what orders it.
type dated struct {
	name     string
	modified time.Time
	file     ports.ProofFile
}

// newestFirst orders the Statements of a directory by modification time, the most
// recent first, then by name: a cache keeps the Statements of earlier inputs, and
// the latest proof is the one that stands.
func newestFirst(found []dated) []ports.ProofFile {
	slices.SortFunc(found, func(a, b dated) int {
		return cmp.Or(b.modified.Compare(a.modified), strings.Compare(a.name, b.name))
	})
	files := make([]ports.ProofFile, len(found))
	for index, entry := range found {
		files[index] = entry.file
	}
	return files
}

// predicateKind is the kind of a Statement file, empty for a file that is not a
// Statement of the format.
func predicateKind(content []byte) format.Kind {
	var envelope struct {
		PredicateType string `json:"predicateType"`
	}
	if json.Unmarshal(content, &envelope) != nil {
		return ""
	}
	kind, _ := format.KindOfPredicateType(envelope.PredicateType)
	return kind
}
