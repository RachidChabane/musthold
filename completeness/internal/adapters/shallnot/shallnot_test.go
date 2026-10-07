package shallnot

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/RachidChabane/spec-proof/completeness/internal/app/ports"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/criterion"
)

const (
	currentReport = "testdata/report-1.2.json"
	projectRoot   = "/work/parcel"
)

// fakeProcess stands for the shallnot executable: it writes a recorded report to
// the path that follows --json-out and records the command it was given.
type fakeProcess struct {
	t          *testing.T
	reportFile string
	result     ports.CommandResult
	commands   []ports.Command
}

func (f *fakeProcess) Run(_ context.Context, command ports.Command) (ports.CommandResult, error) {
	f.commands = append(f.commands, command)
	index := slices.Index(command.Argv, jsonOutFlag)
	if index < 0 || index+1 >= len(command.Argv) {
		f.t.Fatalf("no %s in %v", jsonOutFlag, command.Argv)
	}
	if f.reportFile != "" {
		content, err := os.ReadFile(f.reportFile)
		if err != nil {
			f.t.Fatal(err)
		}
		if err := os.WriteFile(command.Argv[index+1], content, 0o600); err != nil {
			f.t.Fatal(err)
		}
	}
	return f.result, nil
}

func load(t *testing.T, process *fakeProcess) (ports.Report, error) {
	t.Helper()
	process.t = t
	return Gateway{Process: process, Root: projectRoot}.Load(context.Background())
}

func TestLoad(t *testing.T) {
	t.Run("the gate runs advisory with a json-out path in the project root [verifies SP-523~1]", func(t *testing.T) {
		process := &fakeProcess{reportFile: currentReport}
		if _, err := load(t, process); err != nil {
			t.Fatal(err)
		}
		command := process.commands[0]
		if len(command.Argv) != 5 || !slices.Equal(command.Argv[:4], []string{"shallnot", "gate", "--advisory", "--json-out"}) {
			t.Fatalf("argv = %v", command.Argv)
		}
		if command.Dir != projectRoot || filepath.Dir(command.Argv[4]) != filepath.Clean(os.TempDir()) {
			t.Fatalf("dir %q report %q", command.Dir, command.Argv[4])
		}
	})

	t.Run("a configured binary is run instead of the default [verifies SP-523~1]", func(t *testing.T) {
		process := &fakeProcess{t: t, reportFile: currentReport}
		_, err := Gateway{Process: process, Binary: "/opt/bin/shallnot", Root: projectRoot}.Load(context.Background())
		if err != nil || process.commands[0].Argv[0] != "/opt/bin/shallnot" {
			t.Fatalf("argv = %v, err = %v", process.commands[0].Argv, err)
		}
	})

	t.Run("the temporary report is removed after the load [verifies SP-523~1]", func(t *testing.T) {
		process := &fakeProcess{reportFile: currentReport}
		if _, err := load(t, process); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(process.commands[0].Argv[4]); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("the report file is still there: %v", err)
		}
	})

	t.Run("a failing, timed-out or absent shallnot is an error [verifies SP-523~1]", func(t *testing.T) {
		if _, err := load(t, &fakeProcess{result: ports.CommandResult{ExitCode: 2, Stderr: []byte("boom")}}); !errors.Is(err, ErrGateFailed) {
			t.Fatalf("exit 2: %v", err)
		}
		if _, err := load(t, &fakeProcess{result: ports.CommandResult{TimedOut: true}}); !errors.Is(err, ErrGateFailed) {
			t.Fatalf("timeout: %v", err)
		}
		if _, err := (Gateway{Root: projectRoot}).Load(context.Background()); !errors.Is(err, ErrNoRunner) {
			t.Fatalf("no runner: %v", err)
		}
	})
}

func TestParseReport(t *testing.T) {
	read := func(t *testing.T, name string) []byte {
		t.Helper()
		content, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		return content
	}

	t.Run("every spec file is read with its active requirements, statement, digest and examples, sorted [verifies SP-522~1]", func(t *testing.T) {
		report, err := ParseReport(read(t, currentReport))
		if err != nil {
			t.Fatal(err)
		}
		var paths []string
		for _, spec := range report.Project.Specs {
			paths = append(paths, spec.Path)
		}
		if !slices.Equal(paths, []string{"specs/archive.md", "specs/invoicing.md", "specs/shipping.yaml"}) {
			t.Fatalf("specs %v (a spec file without requirements is still a spec)", paths)
		}
		invoicing := report.Project.Specs[1]
		var refs []string
		for _, c := range invoicing.Criteria {
			refs = append(refs, c.Ref.String())
		}
		if !slices.Equal(refs, []string{"INV-1~2", "INV-2~1"}) {
			t.Fatalf("refs %v (a non-testable requirement is excluded)", refs)
		}
		first, _ := report.Project.Find(criterion.Ref{ID: "INV-1", Revision: 2})
		if first.StatementDigest == "" || len(first.Examples) != 2 || first.Examples[0].Operation != "line_amount" || first.Examples[1].Index != 1 {
			t.Fatalf("criterion %+v (examples are sorted by index)", first)
		}
		if len(report.Project.Specs[0].Criteria) != 0 {
			t.Fatalf("the archive has criteria %+v", report.Project.Specs[0].Criteria)
		}
	})

	t.Run("the test roots are read and cleaned [verifies SP-522~1]", func(t *testing.T) {
		report, err := ParseReport(read(t, currentReport))
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(report.Project.TestRoots, []string{"checks", "tests"}) {
			t.Fatalf("test roots %v", report.Project.TestRoots)
		}
	})

	t.Run("the out-of-scope declarations are attached to their spec file, sorted [verifies SP-522~1]", func(t *testing.T) {
		report, err := ParseReport(read(t, currentReport))
		if err != nil {
			t.Fatal(err)
		}
		var declared []string
		for _, declaration := range report.Project.Specs[1].OutOfScope {
			declared = append(declared, declaration.Dimension)
		}
		if !slices.Equal(declared, []string{"concurrency", "permissions", "telepathy"}) {
			t.Fatalf("declarations %v", declared)
		}
		if got := report.Project.Specs[2].DeclaredDimensions(); !slices.Equal(got, []string{"time_and_locale"}) {
			t.Fatalf("declarations of the shipping spec %v", got)
		}
	})

	t.Run("a report without out_of_scope declares nothing out of scope [verifies SP-522~1]", func(t *testing.T) {
		report, err := ParseReport([]byte(`{"schema_version":"1.2","run":{"spec_files":["specs/a.md"]},"requirements":[]}`))
		if err != nil || len(report.Project.Specs) != 1 || len(report.Project.Specs[0].OutOfScope) != 0 {
			t.Fatalf("report %+v: %v", report, err)
		}
	})

	t.Run("a report older than 1.2, a requirement without a digest or a spec file and a malformed report are refused [verifies SP-522~1]", func(t *testing.T) {
		if _, err := ParseReport([]byte(`{"schema_version":"1.1","requirements":[]}`)); !errors.Is(err, ErrReportTooOld) {
			t.Fatalf("legacy: %v", err)
		}
		if _, err := ParseReport([]byte(`{"schema_version":"1.2","requirements":[{"id":"A-1","revision":1,"status":"active","location":{"file":"specs/a.md"}}]}`)); !errors.Is(err, ErrNoStatementDigest) {
			t.Fatalf("no digest: %v", err)
		}
		if _, err := ParseReport([]byte(`{"schema_version":"1.2","requirements":[{"id":"A-1","revision":1,"status":"active","statement_digest":"d"}]}`)); !errors.Is(err, ErrNoSpecFile) {
			t.Fatalf("no spec file: %v", err)
		}
		if _, err := ParseReport([]byte(`not json`)); !errors.Is(err, ErrMalformedReport) {
			t.Fatalf("malformed: %v", err)
		}
		for _, version := range []string{"", "1", "x.y", "0.9"} {
			if _, err := ParseReport([]byte(`{"schema_version":"` + version + `","requirements":[]}`)); !errors.Is(err, ErrReportTooOld) {
				t.Errorf("version %q: %v", version, err)
			}
		}
	})
}
