package construct_test

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/accept"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/accept/acceptest"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/construct"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/construct/records"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study/studytest"
)

const (
	repoFromHere  = "../../../../.."
	committedRun  = "experiments/loop-1/loop-1.json"
	fakeVersion   = "codex-cli 9.9.9"
	sampleProject = "sample"
)

// scripted is a Model that answers each request with a handler and keeps them.
type scripted struct {
	mu       sync.Mutex
	requests []construct.Request
	handler  func(construct.Request) ([]construct.File, error)
}

func (s *scripted) Complete(_ context.Context, request construct.Request) (construct.Reply, error) {
	s.mu.Lock()
	s.requests = append(s.requests, request)
	s.mu.Unlock()
	files, err := s.handler(request)
	at := time.Date(2026, time.October, 27, 12, 0, 0, 0, time.UTC)
	output := "raw output"
	if err == nil {
		output = filesJSON(files)
	}
	return construct.Reply{Files: files, Output: []byte(output), CodexVersion: fakeVersion, Start: at, End: at.Add(time.Second)}, err
}

func (s *scripted) callsOf(role records.Role) []construct.Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	var found []construct.Request
	for _, request := range s.requests {
		if request.Role == role {
			found = append(found, request)
		}
	}
	return found
}

// authorFiles is a complete set of author files for the sample task.
func authorFiles(sample acceptest.Sample, edit func(files map[string]string)) []construct.File {
	files := map[string]string{study.TaskFile: sample.TaskJSON}
	for name, content := range sample.Author {
		files[name] = content
	}
	if edit != nil {
		edit(files)
	}
	return sortedFiles(files)
}

func oracleFiles(sample acceptest.Sample) []construct.File { return sortedFiles(sample.Oracle) }

func sortedFiles(files map[string]string) []construct.File {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]construct.File, 0, len(names))
	for _, name := range names {
		result = append(result, construct.File{Path: name, Content: files[name]})
	}
	return result
}

func isTestsPrompt(request construct.Request) bool {
	return request.Role == records.RoleOracle
}

// world is a repository, a work directory and a constructor over the simulated
// box and a scripted model.
type world struct {
	t           *testing.T
	repo        string
	work        string
	model       *scripted
	box         *acceptest.Box
	constructor *construct.Constructor
	sample      acceptest.Sample
}

func newWorld(t *testing.T, domains []construct.Domain, handler func(construct.Request) ([]construct.File, error)) *world {
	t.Helper()
	repo, work := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(repo, committedRun), string(studytest.BoxofficeConfig()))
	copyTree(t, filepath.Join(repoFromHere, construct.BoxofficeFixture), filepath.Join(repo, construct.BoxofficeFixture))
	copyTree(t, filepath.Join(repoFromHere, construct.BoxofficeReference), filepath.Join(repo, construct.BoxofficeReference))
	layout := study.Layout{Root: work}
	prompts, err := construct.NewPrompts()
	if err != nil {
		t.Fatal(err)
	}
	w := &world{t: t, repo: repo, work: work, model: &scripted{handler: handler}, box: acceptest.NewBox(t, work), sample: acceptest.NewSample(sampleProject)}
	writer := records.NewFileWriter(layout)
	acceptor := &accept.Acceptor{
		Repo: repo, Config: filepath.Join(repo, committedRun), Layout: layout, Runner: w.box, Equipper: &acceptest.Equipper{}, Records: writer,
	}
	w.constructor = &construct.Constructor{
		Repo: repo, Config: acceptor.Config, Store: construct.Store{Repo: repo}, Model: w.model, Acceptor: acceptor,
		Records: writer, History: writer, Prompts: prompts, Domains: domains, Now: func() time.Time { return time.Date(2026, time.October, 27, 13, 0, 0, 0, time.UTC) },
	}
	return w
}

// markBoxofficeBuilt makes the fixed task count as built, so that a run
// concerns the domains alone.
func (w *world) markBoxofficeBuilt() {
	base := filepath.Join(w.repo, study.TaskDir(construct.BoxofficeName))
	writeFile(w.t, filepath.Join(base, study.TaskFile), "{}\n")
	writeFile(w.t, filepath.Join(base, accept.OracleSubdir, accept.OracleConfigFile), "[pytest]\n")
}

func (w *world) log() records.Log {
	w.t.Helper()
	log, err := records.Read(records.Path(study.Layout{Root: w.work}))
	if err != nil {
		w.t.Fatal(err)
	}
	return log
}

func writeFile(t *testing.T, location, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(location), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(location, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func copyTree(t *testing.T, from, to string) {
	t.Helper()
	err := filepath.WalkDir(from, func(location string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		relative, _ := filepath.Rel(from, location)
		data, err := os.ReadFile(location)
		if err != nil {
			return err
		}
		writeFile(t, filepath.Join(to, relative), string(data))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func domains(names ...string) []construct.Domain {
	var list []construct.Domain
	for _, name := range names {
		list = append(list, construct.Domain{Name: name, Line: "the subject of " + strings.ReplaceAll(name, "-", " ")})
	}
	return list
}
