package construct_test

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/accept"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/construct"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/construct/records"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

const (
	boxofficeSourceFile = "demo/loop/fixture/boxoffice/boxoffice/prices.py"
	missingMarker       = "\n# BREAKS: BOX-4\n"
)

var boxCriteria = []string{"BOX-1", "BOX-2", "BOX-3", "BOX-4"}

// boxofficeWorld is a repository with the committed boxoffice fixture and
// reference. The simulated box learns that the fixture lacks BOX-4 from a marker
// appended to the copy of its source.
func boxofficeWorld(t *testing.T, handler func(w *world, request construct.Request) ([]construct.File, error)) *world {
	var w *world
	w = newWorld(t, nil, func(request construct.Request) ([]construct.File, error) { return handler(w, request) })
	source := filepath.Join(w.repo, boxofficeSourceFile)
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, source, string(data)+missingMarker)
	return w
}

func boxofficeTests() []construct.File {
	files := []construct.File{{Path: "pytest.ini", Content: "[pytest]\ntestpaths = .\npythonpath = ..\n"}}
	for index, criterion := range boxCriteria {
		age := 40 + index
		body := "from boxoffice.prices import ticket_price\n\n\ndef test_" + strings.ToLower(strings.ReplaceAll(criterion, "-", "_")) +
			"_first():\n    assert ticket_price(age=" + strconv.Itoa(age) + ") is not None\n\n\ndef test_" + strings.ToLower(strings.ReplaceAll(criterion, "-", "_")) +
			"_second():\n    assert ticket_price(age=" + strconv.Itoa(age+10) + ") is not None\n"
		files = append(files, construct.File{Path: accept.OracleFileName(criterion), Content: body})
	}
	return files
}

// boxofficeVariants is a set of violations and refactors of the committed
// reference, tagged for the simulated box.
func boxofficeVariants(t *testing.T, w *world, detectable bool) []construct.File {
	reference, err := os.ReadFile(filepath.Join(w.repo, construct.BoxofficeReference, "prices.py"))
	if err != nil {
		t.Fatal(err)
	}
	var files []construct.File
	for _, criterion := range boxCriteria {
		for _, k := range []string{"1", "2"} {
			marker := "# BREAKS: " + criterion + "\n"
			if !detectable {
				marker = "# harmless\n"
			}
			if k == "2" {
				marker += "# TESTS-MISS: " + criterion + "\n# INLINE-MISS: " + criterion + "\n"
			}
			files = append(files, construct.File{Path: "violations/" + criterion + "/" + k + "/prices.py", Content: string(reference) + "\n" + marker})
		}
	}
	files = append(files, construct.File{Path: "refactors/1/prices.py", Content: strings.Replace(string(reference), "return FREE", "return 0", 1)})
	return files
}

func TestTheFixedTask(t *testing.T) {
	good := func(detectable bool) func(w *world, request construct.Request) ([]construct.File, error) {
		return func(w *world, request construct.Request) ([]construct.File, error) {
			if request.Role == records.RoleOracle {
				return boxofficeTests(), nil
			}
			return boxofficeVariants(t, w, detectable), nil
		}
	}

	t.Run("the author writes only violations and refactors, from the spec and the reference source, and the fixture and reference stay as they are [verifies SP-2546~1]", func(t *testing.T) {
		w := boxofficeWorld(t, good(true))
		before := map[string][]byte{}
		for _, dir := range []string{construct.BoxofficeFixture, construct.BoxofficeReference} {
			_ = filepath.WalkDir(filepath.Join(w.repo, dir), func(location string, entry os.DirEntry, err error) error {
				if err == nil && !entry.IsDir() {
					before[location], _ = os.ReadFile(location)
				}
				return nil
			})
		}
		if err := w.constructor.BuildFixed(t.Context()); err != nil {
			t.Fatal(err)
		}
		for location, content := range before {
			if now, _ := os.ReadFile(location); string(now) != string(content) {
				t.Errorf("%s changed", location)
			}
		}
		base := filepath.Join(w.repo, study.TaskDir(construct.BoxofficeName))
		for _, wanted := range []string{"task.json", "violations/BOX-4/2/prices.py", "refactors/1/prices.py", "oracle/test_box_4.py", "oracle/pytest.ini"} {
			if _, err := os.Stat(filepath.Join(base, wanted)); err != nil {
				t.Errorf("%s: %v", wanted, err)
			}
		}
		if _, err := os.Stat(filepath.Join(base, "fixture")); !os.IsNotExist(err) {
			t.Error("a fixture was written for the fixed task")
		}
		authorPrompt := w.model.callsOf(records.RoleAuthor)[0].Prompt
		for _, wanted := range []string{"BOX-4~1", "ADULT_PRICE = 12", "violations/<ID>/<k>/prices.py"} {
			if !strings.Contains(authorPrompt, wanted) {
				t.Errorf("the author's prompt lacks %q", wanted)
			}
		}
		for _, absent := range []string{"test_adult_pays_the_full_price", "pytest.ini", "verifies("} {
			if strings.Contains(authorPrompt, absent) {
				t.Errorf("the author's prompt holds %q", absent)
			}
		}
		description, _ := os.ReadFile(filepath.Join(base, "task.json"))
		for _, wanted := range []string{"Implement requirement BOX-4 of specs/tickets.md, free entry for children under 3, in boxoffice/prices.py", construct.BoxofficeFixture, construct.BoxofficeReference} {
			if !strings.Contains(string(description), wanted) {
				t.Errorf("task.json lacks %q:\n%s", wanted, description)
			}
		}
	})

	t.Run("files outside violations and refactors are refused for the fixed task [verifies SP-2546~1]", func(t *testing.T) {
		w := boxofficeWorld(t, func(w *world, request construct.Request) ([]construct.File, error) {
			if request.Role == records.RoleOracle {
				return boxofficeTests(), nil
			}
			files := boxofficeVariants(t, w, true)
			if request.Purpose == records.PurposeGenerate {
				files = append(files, construct.File{Path: "fixture/boxoffice/prices.py", Content: "x = 1\n"})
			}
			return files, nil
		})
		if err := w.constructor.BuildFixed(t.Context()); err != nil {
			t.Fatal(err)
		}
		if got := w.model.callsOf(records.RoleAuthor); len(got) != 2 || !strings.Contains(got[1].Prompt, "lies outside") {
			t.Fatalf("author calls %d", len(got))
		}
	})

	t.Run("a check that only a change of the fixture could satisfy stops construction with a clear error, after the repairs that can help [verifies SP-2546~1]", func(t *testing.T) {
		w := boxofficeWorld(t, good(true))
		source := filepath.Join(w.repo, boxofficeSourceFile)
		data, _ := os.ReadFile(source)
		writeFile(t, source, strings.Replace(string(data), "return SENIOR_PRICE", "return SENIOR_PRICE  # BREAKS: BOX-2", 1))
		err := w.constructor.BuildFixed(t.Context())
		if !errors.Is(err, construct.ErrFixtureAmendment) || !strings.Contains(err.Error(), "A2") {
			t.Fatalf("error %v", err)
		}
		if got := len(w.model.callsOf(records.RoleAuthor)); got != 1 {
			t.Errorf("the author was called %d times for a fixture it cannot change", got)
		}
		if w.constructor.Store.Exists(construct.BoxofficeName) {
			t.Error("the stopped task left files")
		}
	})

	t.Run("the fixed task is dropped, with an error, when its violations never meet the checks [verifies SP-2548~1]", func(t *testing.T) {
		w := boxofficeWorld(t, good(false))
		err := w.constructor.BuildFixed(t.Context())
		if !errors.Is(err, construct.ErrFixedTaskDropped) || !strings.Contains(err.Error(), "A5") {
			t.Fatalf("error %v", err)
		}
		repairs := 0
		for _, request := range w.model.callsOf(records.RoleAuthor) {
			if request.Purpose == records.PurposeRepair {
				repairs++
			}
		}
		if repairs != construct.MaxRepairs || len(w.log().Drops) != 1 {
			t.Errorf("%d repairs, %d drops", repairs, len(w.log().Drops))
		}
	})
}
