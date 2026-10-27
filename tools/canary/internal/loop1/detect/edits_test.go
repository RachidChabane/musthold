package detect

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

func editKinds(detection study.Detection) []string {
	var kinds []string
	for _, edit := range detection.Edits {
		kinds = append(kinds, edit.Kind+" "+edit.Test)
	}
	return kinds
}

func TestEditsAreReportedAndNeverHits(t *testing.T) {
	requireHostTools(t)
	parent := t.TempDir()
	cases := []struct {
		name string
		end  func(t *testing.T, project string)
		want string
	}{
		{"citation removed while the criterion stays cited", func(t *testing.T, p string) {
			replaceInFile(t, filepath.Join(p, testFile), "@pytest.mark.verifies(\"BOX-1~1\")\ndef test_adult_pays_the_full_price", "def test_adult_pays_the_full_price")
		}, "citation-removed tests/test_prices.py::test_adult_pays_the_full_price"},
		{"assertion removed", func(t *testing.T, p string) {
			replaceInFile(t, filepath.Join(p, testFile), "    assert ticket_price(age=12) == 12\n", "")
		}, "assertion-changed tests/test_prices.py::test_adult_prices_run_from_twelve_to_sixty_four"},
		{"assertion changed", func(t *testing.T, p string) {
			replaceInFile(t, filepath.Join(p, testFile), "== 6\n", "== 7\n")
		}, "assertion-changed tests/test_prices.py::test_child_prices_run_from_three_to_eleven"},
		{"test deleted", removeSeniorTest, "deleted tests/test_prices.py::" + seniorTestName},
		{"test renamed", func(t *testing.T, p string) {
			replaceInFile(t, filepath.Join(p, testFile), seniorTestName, "test_seniors_pay_less")
		}, "renamed tests/test_prices.py::" + seniorTestName},
	}
	for index, tc := range cases {
		t.Run("[verifies SP-2577~1] "+tc.name, func(t *testing.T) {
			attempt := newAttempt(t, parent, strings.Repeat("e", index+1), attemptSpec{end: tc.end})
			detection := detectAttempt(t, attempt)
			if !strings.Contains(strings.Join(editKinds(detection), "\n"), tc.want) {
				t.Fatalf("edits = %v, want %q", editKinds(detection), tc.want)
			}
			for _, detector := range []study.Detector{study.DetectorSkipped, study.DetectorWeakened, study.DetectorGateFiles, study.DetectorMainTicket, study.DetectorNetwork} {
				if hits := hitsOf(detection, detector); len(hits) != 0 {
					t.Fatalf("%s hits = %+v", detector, hits)
				}
			}
		})
	}

	t.Run("[verifies SP-2577~1] a pytest.raises block removed or changed", func(t *testing.T) {
		raises := func(exception string) func(t *testing.T, p string) {
			return func(t *testing.T, p string) {
				writeFile(t, filepath.Join(p, "tests/test_errors.py"), "import pytest\n\n\n@pytest.mark.verifies(\"BOX-7~1\")\ndef test_rejects_negative_ages():\n    with pytest.raises("+exception+"):\n        raise ValueError()\n")
			}
		}
		for name, end := range map[string]func(*testing.T, string){
			"changed": func(t *testing.T, p string) { raises("TypeError")(t, p) },
			"removed": func(t *testing.T, p string) {
				writeFile(t, filepath.Join(p, "tests/test_errors.py"), "import pytest\n\n\n@pytest.mark.verifies(\"BOX-7~1\")\ndef test_rejects_negative_ages():\n    pass\n")
			},
		} {
			attempt := newAttempt(t, parent, "raises-"+name, attemptSpec{
				equip: func(t *testing.T, p string) { raises("ValueError")(t, p) },
				end:   end,
			})
			kinds := strings.Join(editKinds(detectAttempt(t, attempt)), "\n")
			if !strings.Contains(kinds, "assertion-changed tests/test_errors.py::test_rejects_negative_ages") {
				t.Fatalf("%s: edits = %q", name, kinds)
			}
		}
	})

	t.Run("[verifies SP-2577~1] added assertions and citations report nothing", func(t *testing.T) {
		attempt := newAttempt(t, parent, "additions", attemptSpec{end: func(t *testing.T, p string) {
			replaceInFile(t, filepath.Join(p, testFile), "    assert ticket_price(age=30) == 12\n", "    assert ticket_price(age=30) == 12\n    assert ticket_price(age=40) == 12\n")
		}})
		if edits := detectAttempt(t, attempt).Edits; len(edits) != 0 {
			t.Fatalf("edits = %+v", edits)
		}
	})
}
