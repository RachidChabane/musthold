package app_test

import (
	"errors"
	"io/fs"
	"maps"
	"path"
	"sync"
	"testing"
	"time"

	"github.com/RachidChabane/musthold/tools/release/internal/app"
	"github.com/RachidChabane/musthold/tools/release/internal/domain"
)

const (
	verifiesArchiveContent = " [verifies SP-1805~3]"
	buildCommit            = "4444444444444444444444444444444444444444"
)

// memoryFiles is an in-memory file store.
type memoryFiles struct {
	mutex sync.Mutex
	files map[string][]byte
}

func newMemoryFiles(files map[string]string) *memoryFiles {
	store := &memoryFiles{files: map[string][]byte{}}
	for name, data := range files {
		store.files[name] = []byte(data)
	}
	return store
}

func (m *memoryFiles) ReadFile(name string) ([]byte, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	data, found := m.files[name]
	if !found {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return data, nil
}

func (m *memoryFiles) Exists(name string) (bool, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	_, found := m.files[name]
	return found, nil
}

func (m *memoryFiles) WriteFile(name string, data []byte, _ int64) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.files[name] = data
	return nil
}

func (m *memoryFiles) RemoveAll(name string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	for existing := range m.files {
		if existing == name || path.Dir(existing) == name {
			delete(m.files, existing)
		}
	}
	return nil
}

// stagingCompiler writes a binary where the build asks for it.
type stagingCompiler struct{ files *memoryFiles }

func (c stagingCompiler) Compile(spec domain.BuildSpec) error {
	return c.files.WriteFile(spec.Output, []byte("binary"), domain.ExecutableMode)
}

// recordingArchiver keeps the entries it is asked to pack.
type recordingArchiver struct {
	mutex   sync.Mutex
	entries [][]domain.ArchiveEntry
}

func (a *recordingArchiver) Pack(_ domain.ArchiveFormat, entries []domain.ArchiveEntry, _ time.Time) ([]byte, error) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	a.entries = append(a.entries, entries)
	return []byte("archive"), nil
}

type fixedInspector struct{}

func (fixedInspector) Inspect([]byte) (domain.BuildFacts, error) {
	return domain.BuildFacts{GoVersion: "go1.26.4", MainPath: "example.com/main"}, nil
}

func buildWith(t *testing.T, sources map[string]string) (*recordingArchiver, error) {
	t.Helper()
	tag, err := domain.ParseTag("discpower/v0.2.0")
	if err != nil {
		t.Fatal(err)
	}
	files := newMemoryFiles(sources)
	archiver := &recordingArchiver{}
	builder := app.Builder{Compiler: stagingCompiler{files}, Archiver: archiver, Inspector: fixedInspector{}, Files: files}
	_, err = builder.Build(app.BuildRequest{
		Tag: tag, Commit: buildCommit, CommitTime: 1_700_000_000, Targets: domain.Targets(), Jobs: 1, DistDir: "dist",
	})
	return archiver, err
}

func entryNames(entries []domain.ArchiveEntry) []string {
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name)
	}
	return names
}

func containsEntry(entries []domain.ArchiveEntry, name, data string) bool {
	for _, entry := range entries {
		if entry.Name == name && (data == "" || string(entry.Data) == data) {
			return true
		}
	}
	return false
}

func TestBuilderArchiveContent(t *testing.T) {
	sources := map[string]string{
		domain.LicenseFile:               "licence",
		"discpower/" + domain.ReadmeFile: "readme",
		domain.NoticeFile:                "notice",
		domain.ThirdPartyNoticesFile:     "third party",
		"script/install":                 "installer",
	}

	t.Run("every archive holds the NOTICE and the third-party notices of the work"+verifiesArchiveContent, func(t *testing.T) {
		archiver, err := buildWith(t, sources)
		if err != nil {
			t.Fatal(err)
		}
		if len(archiver.entries) != len(domain.Targets()) {
			t.Fatalf("%d archives packed", len(archiver.entries))
		}
		for _, entries := range archiver.entries {
			if !containsEntry(entries, domain.NoticeFile, "notice") || !containsEntry(entries, domain.ThirdPartyNoticesFile, "third party") {
				t.Errorf("entries = %v", entryNames(entries))
			}
		}
	})

	t.Run("a build without the NOTICE fails"+verifiesArchiveContent, func(t *testing.T) {
		without := maps.Clone(sources)
		delete(without, domain.NoticeFile)
		if _, err := buildWith(t, without); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("a build without the third-party notices fails"+verifiesArchiveContent, func(t *testing.T) {
		without := maps.Clone(sources)
		delete(without, domain.ThirdPartyNoticesFile)
		if _, err := buildWith(t, without); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("error = %v", err)
		}
	})
}
