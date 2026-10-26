package shallnot

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	"github.com/RachidChabane/musthold/discpower/internal/domain/spec"
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
	reportPath string
}

func (f *fakeProcess) Run(_ context.Context, command ports.Command) (ports.CommandResult, error) {
	f.commands = append(f.commands, command)
	index := slices.Index(command.Argv, jsonOutFlag)
	if index < 0 || index+1 >= len(command.Argv) {
		f.t.Fatalf("no %s in %v", jsonOutFlag, command.Argv)
	}
	f.reportPath = command.Argv[index+1]
	if f.reportFile != "" {
		content, err := os.ReadFile(f.reportFile)
		if err != nil {
			f.t.Fatal(err)
		}
		if err := os.WriteFile(f.reportPath, content, 0o600); err != nil {
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
	t.Run("the gate runs advisory with a json-out path in the project root [verifies SP-158~1]", func(t *testing.T) {
		process := &fakeProcess{reportFile: currentReport}
		if _, err := load(t, process); err != nil {
			t.Fatal(err)
		}
		command := process.commands[0]
		if len(command.Argv) != 5 || !slices.Equal(command.Argv[:4], []string{"shallnot", "gate", "--advisory", "--json-out"}) {
			t.Fatalf("argv = %v", command.Argv)
		}
		if command.Dir != projectRoot {
			t.Fatalf("dir = %q", command.Dir)
		}
		if filepath.Dir(command.Argv[4]) != filepath.Clean(os.TempDir()) {
			t.Fatalf("report path %q is not in the temp directory", command.Argv[4])
		}
	})

	t.Run("a configured binary is run instead of the default [verifies SP-158~1]", func(t *testing.T) {
		process := &fakeProcess{t: t, reportFile: currentReport}
		_, err := Gateway{Process: process, Binary: "/opt/bin/shallnot", Root: projectRoot}.Load(context.Background())
		if err != nil || process.commands[0].Argv[0] != "/opt/bin/shallnot" {
			t.Fatalf("argv = %v, err = %v", process.commands[0].Argv, err)
		}
	})

	t.Run("the report file is removed after a successful load [verifies SP-158~1]", func(t *testing.T) {
		process := &fakeProcess{reportFile: currentReport}
		if _, err := load(t, process); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(process.reportPath); !os.IsNotExist(err) {
			t.Fatalf("the report file remains: %v", err)
		}
	})

	t.Run("the report file is removed after a failed gate [verifies SP-158~1]", func(t *testing.T) {
		process := &fakeProcess{result: ports.CommandResult{ExitCode: 2, Stderr: []byte("spec not found")}}
		_, err := load(t, process)
		if !errors.Is(err, ErrGateFailed) || !strings.Contains(err.Error(), "spec not found") {
			t.Fatalf("error = %v", err)
		}
		if _, statErr := os.Stat(process.reportPath); !os.IsNotExist(statErr) {
			t.Fatalf("the report file remains: %v", statErr)
		}
	})

	t.Run("a gate that times out is an error [verifies SP-158~1]", func(t *testing.T) {
		process := &fakeProcess{result: ports.CommandResult{TimedOut: true, ExitCode: -1}}
		if _, err := load(t, process); !errors.Is(err, ErrGateFailed) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("a gate that wrote nothing is a malformed report [verifies SP-158~1]", func(t *testing.T) {
		if _, err := load(t, &fakeProcess{}); !errors.Is(err, ErrMalformedReport) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("a gateway without a runner is refused [verifies SP-158~1]", func(t *testing.T) {
		if _, err := (Gateway{}).Load(context.Background()); !errors.Is(err, ErrNoRunner) {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestReportContent(t *testing.T) {
	process := &fakeProcess{reportFile: currentReport}
	report, err := load(t, process)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("only the active requirements are given, sorted by reference [verifies SP-157~1]", func(t *testing.T) {
		refs := make([]string, len(report.Criteria))
		for index, criterion := range report.Criteria {
			refs[index] = criterion.Ref.String()
		}
		if report.SchemaVersion != "1.2" || !slices.Equal(refs, []string{"PAR-1~1", "PAR-2~2", "PAR-3~1"}) {
			t.Fatalf("version %q, refs %v", report.SchemaVersion, refs)
		}
	})

	t.Run("a criterion carries its statement, digest and examples sorted by index [verifies SP-157~1]", func(t *testing.T) {
		first := report.Criteria[0]
		if !strings.HasPrefix(first.Statement, "WHEN a line is priced") || first.StatementDigest == "" {
			t.Fatalf("criterion = %+v", first)
		}
		if len(first.Examples) != 2 || first.Examples[0].Index != 0 || first.Examples[1].Index != 1 {
			t.Fatalf("examples = %+v", first.Examples)
		}
		example := first.Examples[0]
		wantArgs := map[string]spec.Literal{
			"unit_price": {Type: spec.TypeInteger, Value: "4"},
			"quantity":   {Type: spec.TypeInteger, Value: "3"},
		}
		if example.Call.Operation != "line_total" || !mapsEqual(example.Call.Args, wantArgs) {
			t.Fatalf("call = %+v", example.Call)
		}
		if example.Expect != (spec.Observation{Kind: spec.Returns, Type: spec.TypeInteger, Value: "12"}) {
			t.Fatalf("expect = %+v", example.Expect)
		}
	})

	t.Run("a raised exception is named by the value, or by the type when the value is empty [verifies SP-157~1]", func(t *testing.T) {
		if got := report.Criteria[2].Examples[0].Expect; got != (spec.Observation{Kind: spec.Raises, Value: "ValueError"}) {
			t.Fatalf("expect = %+v", got)
		}
		report, err := ParseReport([]byte(requirementJSON(`"examples":[{"index":0,"operation":"f","args":{},"expect":{"kind":"raises","type":"KeyError","value":""}}]`)))
		if err != nil {
			t.Fatal(err)
		}
		if got := report.Criteria[0].Examples[0].Expect; got != (spec.Observation{Kind: spec.Raises, Value: "KeyError"}) {
			t.Fatalf("expect = %+v", got)
		}
	})

	t.Run("a decimal and a string literal keep their type and text [verifies SP-157~1]", func(t *testing.T) {
		example := report.Criteria[1].Examples[0]
		if example.Call.Args["zone"] != (spec.Literal{Type: spec.TypeString, Value: "remote"}) ||
			example.Expect != (spec.Observation{Kind: spec.Returns, Type: spec.TypeDecimal, Value: "14.50"}) {
			t.Fatalf("example = %+v", example)
		}
	})

	t.Run("tests are identified the way the JUnit reporter names them [verifies SP-157~1]", func(t *testing.T) {
		tests := report.Criteria[0].Tests
		if len(tests) != 2 {
			t.Fatalf("tests = %+v", tests)
		}
		bound := tests[0]
		if bound.Name != "test_line_total_multiplies" || bound.Classname != "tests.test_parcel" ||
			bound.File != "tests/test_parcel.py" || bound.Line != 7 || bound.ID == "" {
			t.Fatalf("test = %+v", bound)
		}
	})

	t.Run("a scalar literal value is read as its text [verifies SP-157~1]", func(t *testing.T) {
		report, err := ParseReport([]byte(requirementJSON(`"examples":[{"index":0,"operation":"f","args":{"n":{"type":"integer","value":4},"b":{"type":"boolean","value":true}},"expect":{"kind":"returns","type":"null","value":null}}]`)))
		if err != nil {
			t.Fatal(err)
		}
		call := report.Criteria[0].Examples[0]
		if call.Call.Args["n"].Value != "4" || call.Call.Args["b"].Value != "true" || call.Expect.Value != "" {
			t.Fatalf("example = %+v", call)
		}
	})
}

func mapsEqual(a, b map[string]spec.Literal) bool {
	if len(a) != len(b) {
		return false
	}
	for key, value := range a {
		if b[key] != value {
			return false
		}
	}
	return true
}

func requirementJSON(members string) string {
	return `{"schema_version":"1.2","requirements":[{"id":"PAR-9","revision":1,"statement":"s","status":"active","statement_digest":"ab",` +
		members + `,"tests":[]}]}`
}

func TestRefusals(t *testing.T) {
	t.Run("a report of schema 1.1 is refused [verifies SP-157~1]", func(t *testing.T) {
		_, err := load(t, &fakeProcess{reportFile: legacyReport})
		if !errors.Is(err, ErrReportTooOld) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("a schema version that is below 1.2 or unreadable is refused, a later one accepted [verifies SP-157~1]", func(t *testing.T) {
		refused := []string{"", "1", "1.1", "1.0", "0.9", "one.two", "1.x", "1.2extra"}
		for _, version := range refused {
			_, err := ParseReport([]byte(`{"schema_version":"` + version + `","requirements":[]}`))
			if !errors.Is(err, ErrReportTooOld) {
				t.Errorf("version %q: error = %v", version, err)
			}
		}
		for _, version := range []string{"1.2", "1.10", "1.2.1", "2.0"} {
			if _, err := ParseReport([]byte(`{"schema_version":"` + version + `","requirements":[]}`)); err != nil {
				t.Errorf("version %q: %v", version, err)
			}
		}
	})

	t.Run("an active requirement without a statement digest is refused [verifies SP-157~1]", func(t *testing.T) {
		_, err := ParseReport([]byte(`{"schema_version":"1.2","requirements":[{"id":"PAR-1","revision":1,"statement":"s","status":"active","examples":[],"tests":[]}]}`))
		if !errors.Is(err, ErrNoStatementDigest) || !strings.Contains(err.Error(), "PAR-1~1") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("a requirement that is not active needs no digest [verifies SP-157~1]", func(t *testing.T) {
		report, err := ParseReport([]byte(`{"schema_version":"1.2","requirements":[{"id":"PAR-1","revision":1,"statement":"s","status":"superseded"}]}`))
		if err != nil || len(report.Criteria) != 0 {
			t.Fatalf("report = %+v, %v", report, err)
		}
	})

	t.Run("a report that is not JSON, an unknown literal type or an unknown kind is malformed [verifies SP-157~1]", func(t *testing.T) {
		if _, err := ParseReport([]byte("not json")); !errors.Is(err, ErrMalformedReport) {
			t.Fatalf("error = %v", err)
		}
		unknownType := requirementJSON(`"examples":[{"index":0,"operation":"f","args":{"a":{"type":"float","value":"1"}},"expect":{"kind":"raises","value":"E"}}]`)
		if _, err := ParseReport([]byte(unknownType)); !errors.Is(err, spec.ErrUnknownLiteralType) {
			t.Fatalf("error = %v", err)
		}
		unknownKind := requirementJSON(`"examples":[{"index":0,"operation":"f","args":{},"expect":{"kind":"hangs","value":"E"}}]`)
		if _, err := ParseReport([]byte(unknownKind)); !errors.Is(err, ErrMalformedReport) {
			t.Fatalf("error = %v", err)
		}
	})
}
