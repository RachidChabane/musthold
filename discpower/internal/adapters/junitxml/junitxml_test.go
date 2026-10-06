package junitxml_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/junitxml"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
)

const testdata = "testdata"

var sampleFormats = map[string]ecosystem.ReportFormat{
	"surefire":   ecosystem.ReportSurefire,
	"gradle":     ecosystem.ReportGradle,
	"jest-junit": ecosystem.ReportJestJUnit,
	"vitest":     ecosystem.ReportVitest,
	"pytest":     ecosystem.ReportPytest,
}

func counts(outcomes map[ecosystem.TestID]ecosystem.TestOutcome) map[string]int {
	got := map[string]int{}
	for _, outcome := range outcomes {
		got[string(outcome)]++
	}
	return got
}

func TestParseRealReporterOutput(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(testdata, "expected-counts.json"))
	if err != nil {
		t.Fatal(err)
	}
	var expected map[string]map[string]int
	if err := json.Unmarshal(raw, &expected); err != nil {
		t.Fatal(err)
	}
	for name, format := range sampleFormats {
		t.Run(name+" outcomes match an independent count [verifies SP-71~1]", func(t *testing.T) {
			file, err := os.Open(filepath.Join(testdata, name+".xml"))
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			outcomes, err := junitxml.Parse(file, format)
			if err != nil {
				t.Fatal(err)
			}
			got, want := counts(outcomes), expected[name]
			for outcome, n := range want {
				if got[outcome] != n {
					t.Fatalf("%s: counts = %v, want %v", name, got, want)
				}
			}
		})
	}
}

const (
	errorXML = `<testsuite><testcase classname="c" name="body"><error message="boom"/></testcase></testsuite>`
	nested   = `<testsuites><testsuite name="a"><testsuite name="b"><testcase classname="c" name="n"/></testsuite></testsuite></testsuites>`
	repeated = `<testsuite><testcase classname="c" name="t"/><testcase classname="c" name="t"><failure/></testcase><testcase classname="c" name="t"><skipped/></testcase></testsuite>`
)

func TestParseDialects(t *testing.T) {
	id := ecosystem.NewTestID("c", "body")

	t.Run("an error element is a failure where the test body raises it [verifies SP-71~1]", func(t *testing.T) {
		for _, format := range []ecosystem.ReportFormat{ecosystem.ReportSurefire, ecosystem.ReportGradle, ecosystem.ReportVitest} {
			outcomes, err := junitxml.Parse(strings.NewReader(errorXML), format)
			if err != nil || outcomes[id] != ecosystem.TestFailed {
				t.Fatalf("%s: %v %v", format, outcomes, err)
			}
		}
	})

	t.Run("an error element is an error where the runner raises it [verifies SP-71~1]", func(t *testing.T) {
		for _, format := range []ecosystem.ReportFormat{ecosystem.ReportPytest, ecosystem.ReportJestJUnit, ecosystem.ReportGotestsum} {
			outcomes, err := junitxml.Parse(strings.NewReader(errorXML), format)
			if err != nil || outcomes[id] != ecosystem.TestErrored {
				t.Fatalf("%s: %v %v", format, outcomes, err)
			}
		}
	})

	t.Run("test cases of nested suites are found [verifies SP-71~1]", func(t *testing.T) {
		outcomes, err := junitxml.Parse(strings.NewReader(nested), ecosystem.ReportGotestsum)
		if err != nil || outcomes[ecosystem.NewTestID("c", "n")] != ecosystem.TestPassed {
			t.Fatalf("%v %v", outcomes, err)
		}
	})

	t.Run("a test reported several times keeps its most severe outcome [verifies SP-71~1]", func(t *testing.T) {
		outcomes, err := junitxml.Parse(strings.NewReader(repeated), ecosystem.ReportSurefire)
		if err != nil || outcomes[ecosystem.NewTestID("c", "t")] != ecosystem.TestFailed {
			t.Fatalf("%v %v", outcomes, err)
		}
	})

	t.Run("malformed XML and an unknown format are refused [verifies SP-71~1]", func(t *testing.T) {
		if _, err := junitxml.Parse(strings.NewReader("<testsuite><testcase"), ecosystem.ReportPytest); err == nil {
			t.Fatal("malformed XML was accepted")
		}
		if _, err := junitxml.Parse(strings.NewReader(nested), "nunit"); !errors.Is(err, junitxml.ErrUnknownFormat) {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestReaderMergesFilesAndToleratesMissingOnes(t *testing.T) {
	dir := t.TempDir()
	first := `<testsuite><testcase classname="c" name="a"/><testcase classname="c" name="b"/></testsuite>`
	second := `<testsuite><testcase classname="c" name="b"><failure/></testcase></testsuite>`
	for name, content := range map[string]string{"TEST-1.xml": first, "TEST-2.xml": second} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	reader := junitxml.NewReader()

	t.Run("files matching a glob are merged [verifies SP-71~1]", func(t *testing.T) {
		outcomes, err := reader.Read([]string{filepath.Join(dir, "TEST-*.xml")}, ecosystem.ReportSurefire)
		if err != nil {
			t.Fatal(err)
		}
		if outcomes[ecosystem.NewTestID("c", "a")] != ecosystem.TestPassed || outcomes[ecosystem.NewTestID("c", "b")] != ecosystem.TestFailed {
			t.Fatalf("outcomes = %v", outcomes)
		}
	})

	t.Run("a pattern that matches nothing contributes nothing [verifies SP-71~1]", func(t *testing.T) {
		outcomes, err := reader.Read([]string{filepath.Join(dir, "absent-*.xml")}, ecosystem.ReportSurefire)
		if err != nil || len(outcomes) != 0 {
			t.Fatalf("%v %v", outcomes, err)
		}
	})

	t.Run("an unreadable document and an unknown format fail [verifies SP-71~1]", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(dir, "bad.xml"), []byte("<testsuite><testcase"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := reader.Read([]string{filepath.Join(dir, "bad.xml")}, ecosystem.ReportPytest); err == nil {
			t.Fatal("malformed file accepted")
		}
		if _, err := reader.Read(nil, "nunit"); !errors.Is(err, junitxml.ErrUnknownFormat) {
			t.Fatalf("err = %v", err)
		}
	})
}
