package construct_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/accept"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/accept/acceptest"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/construct"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/construct/records"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

func TestOutputChecks(t *testing.T) {
	file := func(paths ...string) []construct.File {
		var files []construct.File
		for _, path := range paths {
			files = append(files, construct.File{Path: path, Content: "x"})
		}
		return files
	}

	t.Run("a whole task is task.json and files under fixture, reference, violations and refactors [verifies SP-2543~1]", func(t *testing.T) {
		if err := construct.CheckAuthorFiles(file("task.json", "fixture/a.py", "reference/b.py", "violations/X-1/1/b.py", "refactors/1/b.py")); err != nil {
			t.Fatal(err)
		}
		for name, files := range map[string][]construct.File{
			"no task.json":      file("fixture/a.py"),
			"another top level": file("task.json", "oracle/test_x_1.py"),
			"a loose file":      file("task.json", "notes.md"),
			"an escape":         file("task.json", "../x.py"),
			"an absolute path":  file("task.json", "/etc/x.py"),
			"an unclean path":   file("task.json", "fixture//a.py"),
			"a repeated path":   file("task.json", "fixture/a.py", "fixture/a.py"),
			"an empty path":     file("task.json", ""),
		} {
			if err := construct.CheckAuthorFiles(files); !errors.Is(err, construct.ErrBadOutput) {
				t.Errorf("%s: %v", name, err)
			}
		}
	})

	t.Run("variants are files under violations and refactors alone [verifies SP-2546~1]", func(t *testing.T) {
		if err := construct.CheckVariantFiles(file("violations/X-1/1/b.py", "refactors/1/b.py")); err != nil {
			t.Fatal(err)
		}
		for _, bad := range []string{"task.json", "fixture/a.py", "reference/b.py"} {
			if err := construct.CheckVariantFiles(file(bad)); !errors.Is(err, construct.ErrBadOutput) {
				t.Errorf("%s: %v", bad, err)
			}
		}
	})

	t.Run("the tests are pytest.ini and test_*.py files in one flat directory [verifies SP-2545~1]", func(t *testing.T) {
		if err := construct.CheckTestFiles(file("pytest.ini", "test_x_1.py")); err != nil {
			t.Fatal(err)
		}
		for _, bad := range []string{"conftest.py", "helper.py", "sub/test_x.py", "test_x.txt", "../test_x.py"} {
			if err := construct.CheckTestFiles(file(bad)); !errors.Is(err, construct.ErrBadOutput) {
				t.Errorf("%s: %v", bad, err)
			}
		}
	})

	t.Run("no set of files with an escaping path is ever accepted [verifies SP-2543~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			prefix := rapid.SampledFrom([]string{"fixture", "reference", "violations", "refactors"}).Draw(t, "prefix")
			depth := rapid.IntRange(1, 3).Draw(t, "depth")
			path := prefix + "/" + strings.Repeat("../", depth) + "x.py"
			if err := construct.CheckAuthorFiles(file("task.json", path)); err == nil {
				t.Fatalf("%s was accepted", path)
			}
		})
	})
}

func TestTheTaskDescriptionTheAuthorWrites(t *testing.T) {
	t.Run("the harness sets the name, the fixture and the truth paths whatever the author wrote [verifies SP-2543~1]", func(t *testing.T) {
		sample := acceptest.NewSample(sampleProject)
		task, err := construct.NormaliseTask(`{"name":"wrong","fixture":"elsewhere","spec":"`+sample.Task().Spec+`","source":"fare/zones.py","criterion":"FARE-4","prompt":"p","operations":{"fare_for":{"python":"fare.zones:fare_for"}},"interface":{"module":"fare.zones","operation":"fare_for","signature":"fare_for(distance)"}}`, "train-fare")
		if err != nil {
			t.Fatal(err)
		}
		want := accept.WithStandardPaths(study.Task{Name: "train-fare"})
		if task.Name != "train-fare" || task.Fixture != want.Fixture || task.Truth != want.Truth || task.Criterion != "FARE-4" {
			t.Fatalf("%+v", task)
		}
		if task.Fixture != "experiments/loop-1/tasks/train-fare/fixture" || task.Truth.Oracle != "experiments/loop-1/tasks/train-fare/oracle" {
			t.Fatalf("%+v", task)
		}
	})

	t.Run("a description that is not a JSON object is refused [verifies SP-2543~1]", func(t *testing.T) {
		if _, err := construct.NormaliseTask("not json", "x"); !errors.Is(err, construct.ErrBadOutput) {
			t.Fatal(err)
		}
	})
}

func TestStore(t *testing.T) {
	t.Run("a whole task is written under its directory, replacing what was there, and variants are replaced alone [verifies SP-2543~1]", func(t *testing.T) {
		repo := t.TempDir()
		store := construct.Store{Repo: repo}
		task := accept.WithStandardPaths(study.Task{Name: "d1"})
		first := []construct.File{{Path: "task.json", Content: "{}"}, {Path: "fixture/a.py", Content: "1"}, {Path: "violations/X-1/1/a.py", Content: "v"}}
		if err := store.WriteAuthor("d1", first, task); err != nil {
			t.Fatal(err)
		}
		if err := store.WriteTests("d1", []construct.File{{Path: "pytest.ini", Content: "[pytest]"}}); err != nil {
			t.Fatal(err)
		}
		second := []construct.File{{Path: "task.json", Content: "{}"}, {Path: "fixture/b.py", Content: "2"}}
		if err := store.WriteAuthor("d1", second, task); err != nil {
			t.Fatal(err)
		}
		base := filepath.Join(repo, accept.TaskDir("d1"))
		for name, want := range map[string]bool{"fixture/b.py": true, "fixture/a.py": false, "violations/X-1/1/a.py": false, "oracle/pytest.ini": true} {
			_, err := os.Stat(filepath.Join(base, name))
			if (err == nil) != want {
				t.Errorf("%s present: %v", name, err == nil)
			}
		}
		if err := store.WriteVariants("d1", []construct.File{{Path: "refactors/1/b.py", Content: "r"}}); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(base, "fixture/b.py")); err != nil {
			t.Error("the variants' replacement touched the fixture")
		}
		if !store.Complete("d1") || store.Complete("d2") {
			t.Error("Complete")
		}
		if err := store.Remove("d1"); err != nil || store.Exists("d1") {
			t.Errorf("remove: %v", err)
		}
	})
}

func TestRecords(t *testing.T) {
	t.Run("entries appended to the records file are read back whole and in order, a missing file is an empty log [verifies SP-2549~1]", func(t *testing.T) {
		layout := study.Layout{Root: t.TempDir()}
		if log, err := records.Read(records.Path(layout)); err != nil || len(log.Calls) != 0 {
			t.Fatalf("%v %+v", err, log)
		}
		writer := records.NewFileWriter(layout)
		entries := []records.Entry{
			{Call: &records.Call{Role: records.RoleAuthor, Domain: "d1", Attempt: 1, PromptSHA256: "ab", CodexVersion: fakeVersion}},
			{Check: &records.Check{Domain: "d1", Round: 1, ID: "A1", Passed: true, Agreed: true}},
			{Drop: &records.Drop{Domain: "d2", Check: "A4 base", Reason: "r"}},
			{Call: &records.Call{Role: records.RoleOracle, Domain: "d1", Attempt: 1}},
		}
		for _, entry := range entries {
			if err := writer.Append(entry); err != nil {
				t.Fatal(err)
			}
		}
		log, err := records.Read(records.Path(layout))
		if err != nil || len(log.Calls) != 2 || len(log.Checks) != 1 || len(log.Drops) != 1 || log.Calls[1].Role != records.RoleOracle || log.Calls[0].CodexVersion != fakeVersion {
			t.Fatalf("%v %+v", err, log)
		}
	})

	t.Run("a line that holds no record, or a field the records do not have, is refused [verifies SP-2549~1]", func(t *testing.T) {
		for _, bad := range []string{"{}\n", `{"call":{"unknown":1}}` + "\n", "not json\n"} {
			if _, err := records.Parse([]byte(bad)); !errors.Is(err, records.ErrMalformedRecords) {
				t.Errorf("%q: %v", bad, err)
			}
		}
	})
}
