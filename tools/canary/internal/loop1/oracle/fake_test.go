package oracle

import (
	"archive/tar"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box"
)

// fakeCase is one test case a fake pytest reports.
type fakeCase struct {
	Classname string
	Name      string
	Failure   bool
	Error     bool
	Skipped   bool
}

// fakeOutcome is how a fake pytest run ends.
type fakeOutcome struct {
	Cases    []fakeCase
	Exit     int
	TimedOut bool
	OOM      bool
	// NoReport leaves the JUnit file unwritten.
	NoReport bool
}

// pytestCall is what a fake pytest run sees: the tree it runs in, as relative
// path to content, and which of the three runs it is.
type pytestCall struct {
	Kind string
	Run  int
	Dir  string
	Tree map[string]string
}

const (
	kindOracle  = "oracle"
	kindHeldOut = "heldout"
)

// fakeBox executes pytest-like commands deterministically and serves git
// archives from tables.
type fakeBox struct {
	t        *testing.T
	mu       sync.Mutex
	commands []box.Command
	pytest   func(pytestCall) fakeOutcome
	// archives maps a tree-ish to the files of its archive, by path in the
	// repository.
	archives map[string]map[string]string
}

func (f *fakeBox) Run(_ context.Context, command box.Command) (box.Result, error) {
	f.mu.Lock()
	f.commands = append(f.commands, command)
	f.mu.Unlock()
	switch command.Argv[0] {
	case "python":
		return f.runPytest(command)
	case "git":
		return f.runGit(command)
	}
	return box.Result{}, fmt.Errorf("fake box: unknown command %v", command.Argv)
}

func (f *fakeBox) calls(binary string) []box.Command {
	f.mu.Lock()
	defer f.mu.Unlock()
	var found []box.Command
	for _, command := range f.commands {
		if command.Argv[0] == binary {
			found = append(found, command)
		}
	}
	return found
}

func (f *fakeBox) runPytest(command box.Command) (box.Result, error) {
	out := ""
	for _, arg := range command.Argv {
		if value, ok := strings.CutPrefix(arg, "--junitxml="); ok {
			out = value
		}
	}
	base := filepath.Base(out)
	kind := kindHeldOut
	if strings.HasPrefix(base, "oracle-") {
		kind = kindOracle
	}
	number := strings.TrimSuffix(base[strings.LastIndex(base, "-")+1:], ".xml")
	run, _ := strconv.Atoi(number)
	outcome := f.pytest(pytestCall{Kind: kind, Run: run, Dir: command.Dir, Tree: readTree(f.t, command.Dir)})
	if !outcome.NoReport && !outcome.TimedOut {
		if err := os.WriteFile(out, junitOf(outcome.Cases), 0o644); err != nil {
			return box.Result{}, err
		}
	}
	return box.Result{ExitCode: outcome.Exit, TimedOut: outcome.TimedOut, OOMKilled: outcome.OOM}, nil
}

func (f *fakeBox) runGit(command box.Command) (box.Result, error) {
	argv := command.Argv
	var dest string
	for i, arg := range argv {
		if arg == "-o" {
			dest = argv[i+1]
		}
	}
	treeish, pathspec := argv[len(argv)-2], argv[len(argv)-1]
	var names []string
	for name := range f.archives[treeish] {
		if name == pathspec || strings.HasPrefix(name, pathspec+"/") {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return box.Result{ExitCode: 128, Stderr: []byte("fatal: pathspec '" + pathspec + "' did not match any files")}, nil
	}
	sort.Strings(names)
	file, err := os.Create(dest)
	if err != nil {
		return box.Result{}, err
	}
	defer file.Close()
	writer := tar.NewWriter(file)
	for _, name := range names {
		body := f.archives[treeish][name]
		if err := writer.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			return box.Result{}, err
		}
		if _, err := writer.Write([]byte(body)); err != nil {
			return box.Result{}, err
		}
	}
	return box.Result{}, writer.Close()
}

func junitOf(cases []fakeCase) []byte {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?><testsuites><testsuite name="pytest">`)
	for _, c := range cases {
		fmt.Fprintf(&b, `<testcase classname=%q name=%q>`, c.Classname, c.Name)
		if c.Failure {
			b.WriteString(`<failure message="assert">x</failure>`)
		}
		if c.Error {
			b.WriteString(`<error message="boom">x</error>`)
		}
		if c.Skipped {
			b.WriteString(`<skipped/>`)
		}
		b.WriteString(`</testcase>`)
	}
	b.WriteString(`</testsuite></testsuites>`)
	return []byte(b.String())
}

// readTree reads every regular file under dir.
func readTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	tree := map[string]string{}
	err := walkFiles(dir, skipNothing, func(rel, abs string, _ os.FileMode) error {
		data, err := os.ReadFile(abs)
		tree[rel] = string(data)
		return err
	})
	if err != nil {
		t.Fatalf("read tree %s: %v", dir, err)
	}
	return tree
}

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
