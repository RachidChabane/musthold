package acceptest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/accept"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// Sample is a complete, valid task for the simulated box: a train fare by
// distance zones. Its paths are the files of the author, relative to the task
// directory, and the files of the oracle, relative to its directory.
type Sample struct {
	Name string
	// Author holds the fixture, reference, violations and refactors.
	Author map[string]string
	// TaskJSON is the task description as the author writes it.
	TaskJSON string
	// Oracle holds the oracle's files.
	Oracle map[string]string
}

const (
	sampleSpec = `# Train fares

The ticket office prices a journey from its distance in whole kilometres.

## Fares

- **FARE-1~1**: WHEN the distance is 0 to 10 THE SYSTEM SHALL charge 2.
  - Example: fare_for(distance=5) => 2
- **FARE-2~1**: WHEN the distance is 11 to 50 THE SYSTEM SHALL charge 5.
  - Example: fare_for(distance=30) => 5
- **FARE-3~1**: WHEN the distance is 51 to 200 THE SYSTEM SHALL charge 9.
  - Example: fare_for(distance=100) => 9
- **FARE-4~1**: WHEN the distance is more than 200 THE SYSTEM SHALL charge 14.
  - Example: fare_for(distance=250) => 14
`
	sampleTests = `import pytest

from fare import fare_for


@pytest.mark.verifies("FARE-1~1")
def test_short_trip_costs_two():
    assert fare_for(distance=3) == 2


@pytest.mark.verifies("FARE-2~1")
def test_medium_trip_costs_five():
    assert fare_for(distance=20) == 5


@pytest.mark.verifies("FARE-3~1")
def test_long_trip_costs_nine():
    assert fare_for(distance=150) == 9
`
	sampleInit = `from fare.zones import fare_for

__all__ = ["fare_for"]
`
	sampleIni       = "[pytest]\ntestpaths = tests\npythonpath = .\n"
	sampleIgnore    = "__pycache__/\ntest-results/\n"
	sampleOracleIni = "[pytest]\ntestpaths = .\npythonpath = ..\n"
)

func zones(top string) string {
	return `SHORT_LIMIT = 10
MEDIUM_LIMIT = 50
LONG_LIMIT = 200


def fare_for(distance):
    if distance <= SHORT_LIMIT:
        return 2
    if distance <= MEDIUM_LIMIT:
        return 5
    if distance <= LONG_LIMIT:
        return 9
` + top
}

func sampleOracle(id string, a, b string) string {
	return "from fare.zones import fare_for\n\n\ndef test_" + id + "_example():\n    assert fare_for(distance=" + a + ")\n\n\ndef test_" + id + "_boundary():\n    assert fare_for(distance=" + b + ")\n"
}

// NewSample builds the sample under a task name.
func NewSample(name string) Sample {
	prompt := accept.Prompt("FARE-4", "specs/fares.md", "a fare for long distances", "fare/zones.py")
	partial := map[string]any{
		"spec": "specs/fares.md", "source": "fare/zones.py", "criterion": "FARE-4", "prompt": prompt,
		"operations": map[string]any{"fare_for": map[string]string{"python": "fare.zones:fare_for"}},
		"interface":  map[string]string{"module": "fare.zones", "operation": "fare_for", "signature": "fare_for(distance)"},
	}
	encoded, _ := json.Marshal(partial)
	author := map[string]string{
		"fixture/pytest.ini":         sampleIni,
		"fixture/.gitignore":         sampleIgnore,
		"fixture/specs/fares.md":     sampleSpec,
		"fixture/tests/test_fare.py": sampleTests,
		"fixture/fare/__init__.py":   sampleInit,
		"fixture/fare/zones.py":      zones("    return 9  # BREAKS: FARE-4\n"),
		"reference/zones.py":         zones("    return 14\n"),
		"refactors/1/zones.py":       strings.Replace(zones("    return 14\n"), "return 2", "return 1 + 1", 1),
	}
	for criterion, change := range map[string][2]string{
		"FARE-1": {"if distance <= SHORT_LIMIT:", "if distance < SHORT_LIMIT:"},
		"FARE-2": {"if distance <= MEDIUM_LIMIT:", "if distance < MEDIUM_LIMIT:"},
		"FARE-3": {"if distance <= LONG_LIMIT:", "if distance < LONG_LIMIT:"},
		"FARE-4": {"    return 14\n", "    return 15\n"},
	} {
		for index, miss := range []string{"", "  # TESTS-MISS: " + criterion + "  # INLINE-MISS: " + criterion} {
			edited := strings.Replace(zones("    return 14\n"), change[0], strings.TrimSuffix(change[1], "\n")+miss+"\n", 1)
			edited = strings.Replace(edited, "\n\n\ndef", "\n\n\n# BREAKS: "+criterion+"\ndef", 1)
			author["violations/"+criterion+"/"+string(rune('1'+index))+"/zones.py"] = edited
		}
	}
	return Sample{
		Name: name, Author: author, TaskJSON: string(encoded),
		Oracle: map[string]string{
			"pytest.ini":     sampleOracleIni,
			"test_fare_1.py": sampleOracle("one", "5", "10"),
			"test_fare_2.py": sampleOracle("two", "30", "11"),
			"test_fare_3.py": sampleOracle("three", "100", "51"),
			"test_fare_4.py": sampleOracle("four", "250", "201"),
		},
	}
}

// Task is the sample's task description with the standard paths.
func (s Sample) Task() study.Task {
	var task study.Task
	_ = json.Unmarshal([]byte(s.TaskJSON), &task)
	task.Name = s.Name
	return accept.WithStandardPaths(task)
}

// Write lays the sample out under a repository root, task.json included.
func (s Sample) Write(t testing.TB, repo string) {
	t.Helper()
	base := filepath.Join(repo, filepath.FromSlash(accept.TaskDir(s.Name)))
	for name, content := range s.Author {
		write(t, filepath.Join(base, filepath.FromSlash(name)), content)
	}
	for name, content := range s.Oracle {
		write(t, filepath.Join(base, accept.OracleSubdir, name), content)
	}
	encoded, err := json.MarshalIndent(s.Task(), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(base, study.TaskFile), string(encoded))
}

func write(t testing.TB, location, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(location), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(location, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Remove deletes one file of the sample from a written repository.
func Remove(t testing.TB, repo, name, file string) {
	t.Helper()
	if err := os.Remove(filepath.Join(repo, filepath.FromSlash(accept.TaskDir(name)), filepath.FromSlash(file))); err != nil {
		t.Fatal(err)
	}
}

// Replace edits a written file of a task.
func Replace(t testing.TB, repo, name, file, from, to string) {
	t.Helper()
	location := filepath.Join(repo, filepath.FromSlash(accept.TaskDir(name)), filepath.FromSlash(file))
	data, err := os.ReadFile(location)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), from) {
		t.Fatalf("%s does not hold %q", file, from)
	}
	write(t, location, strings.Replace(string(data), from, to, 1))
}
