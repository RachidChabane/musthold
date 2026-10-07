package shallnot

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/RachidChabane/spec-proof/alignment/internal/app/ports"
	"github.com/RachidChabane/spec-proof/alignment/internal/domain/criterion"
)

const (
	currentReport = "testdata/report-1.2.json"
	legacyReport  = "testdata/report-1.1.json"
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
	t.Run("the gate runs advisory with a json-out path in the project root [verifies SP-327~1]", func(t *testing.T) {
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

	t.Run("a configured binary is run instead of the default [verifies SP-327~1]", func(t *testing.T) {
		process := &fakeProcess{t: t, reportFile: currentReport}
		_, err := Gateway{Process: process, Binary: "/opt/bin/shallnot", Root: projectRoot}.Load(context.Background())
		if err != nil || process.commands[0].Argv[0] != "/opt/bin/shallnot" {
			t.Fatalf("argv = %v, err = %v", process.commands[0].Argv, err)
		}
	})

	t.Run("the temporary report is removed after the load [verifies SP-327~1]", func(t *testing.T) {
		process := &fakeProcess{reportFile: currentReport}
		if _, err := load(t, process); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(process.commands[0].Argv[4]); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("the report file is still there: %v", err)
		}
	})

	t.Run("a failing, timed-out or absent shallnot is an error [verifies SP-327~1]", func(t *testing.T) {
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

	t.Run("the active requirements are read with statement, digest and bound tests, sorted [verifies SP-326~1]", func(t *testing.T) {
		report, err := ParseReport(read(t, currentReport))
		if err != nil {
			t.Fatal(err)
		}
		var refs []string
		for _, c := range report.Project.Criteria {
			refs = append(refs, c.Ref.String())
		}
		if !slices.Equal(refs, []string{"PAR-1~1", "PAR-2~2", "PAR-3~1"}) {
			t.Fatalf("refs %v (a superseded requirement is excluded)", refs)
		}
		first, _ := report.Project.Find(criterion.Ref{ID: "PAR-1", Revision: 1})
		if first.StatementDigest == "" || len(first.Tests) != 2 || first.Tests[0].File != "tests/test_parcel.py" || first.Tests[0].Line != 7 {
			t.Fatalf("criterion %+v", first)
		}
		second, _ := report.Project.Find(criterion.Ref{ID: "PAR-2", Revision: 2})
		if second.Tests[0].Label() != "tests.test_shipping.TestSurcharge.test_remote_zone_adds_surcharge" {
			t.Fatalf("label %q", second.Tests[0].Label())
		}
	})

	t.Run("a report older than 1.2 and a requirement without a digest are refused [verifies SP-326~1]", func(t *testing.T) {
		if _, err := ParseReport(read(t, legacyReport)); !errors.Is(err, ErrReportTooOld) {
			t.Fatalf("legacy: %v", err)
		}
		if _, err := ParseReport([]byte(`{"schema_version":"1.2","requirements":[{"id":"A-1","revision":1,"status":"active"}]}`)); !errors.Is(err, ErrNoStatementDigest) {
			t.Fatalf("no digest: %v", err)
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

	t.Run("a test is identified by its name, or by its classname and name [verifies SP-326~1]", func(t *testing.T) {
		bare := criterion.BoundTest{Name: "TestMain/prints_its_version"}
		classed := criterion.BoundTest{Classname: "tests.test_parcel", Name: "test_x"}
		if bare.Label() != "TestMain/prints_its_version" || classed.Label() != "tests.test_parcel.test_x" || classed.ResultID() != "tests.test_parcel::test_x" {
			t.Fatalf("labels %q %q %q", bare.Label(), classed.Label(), classed.ResultID())
		}
	})
}
