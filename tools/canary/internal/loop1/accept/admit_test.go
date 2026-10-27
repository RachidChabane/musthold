package accept_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/config"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/accept"
)

func committedConfig(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoFromHere, committedRun))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func newTask(name string) config.Task {
	return config.Task{
		Name: name, Fixture: "experiments/loop-1/tasks/" + name + "/fixture",
		Prompt:     "Implement requirement T-4 of specs/t.md, a rule, in t/t.py, and add a test that verifies it. Tell me when the task is done.",
		Operations: map[string]map[string]string{"op": {"python": "t.t:op"}},
	}
}

func TestAppendTask(t *testing.T) {
	t.Run("a task is added after the last entry and every other byte of the configuration stays [verifies SP-2558~1]", func(t *testing.T) {
		before := committedConfig(t)
		after, err := accept.AppendTask(before, newTask("fare"))
		if err != nil {
			t.Fatal(err)
		}
		var parsed struct {
			Tasks []config.Task `json:"tasks"`
		}
		if err := json.Unmarshal(after, &parsed); err != nil {
			t.Fatal(err)
		}
		if len(parsed.Tasks) != 2 || parsed.Tasks[0].Name != "boxoffice" || parsed.Tasks[1].Name != "fare" {
			t.Fatalf("tasks %+v", parsed.Tasks)
		}
		marker := "\n  ],\n  \"arms\""
		head, tail, found := strings.Cut(string(before), marker)
		if !found {
			t.Fatal("the committed configuration changed its layout")
		}
		if !strings.HasPrefix(string(after), head) || !strings.HasSuffix(string(after), marker+tail) {
			t.Fatalf("bytes outside the tasks array changed:\n%s", after)
		}
	})

	t.Run("a task already listed leaves the configuration as it is [verifies SP-2558~1]", func(t *testing.T) {
		before := committedConfig(t)
		after, err := accept.AppendTask(before, newTask("boxoffice"))
		if err != nil || string(after) != string(before) {
			t.Fatalf("%v\n%s", err, after)
		}
	})

	t.Run("an empty tasks array receives the first task inside its brackets [verifies SP-2558~1]", func(t *testing.T) {
		after, err := accept.AppendTask([]byte("{\n  \"tasks\": []\n}\n"), newTask("fare"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(after), "\"tasks\": [\n    {\n      \"name\": \"fare\"") || !strings.HasSuffix(string(after), "    }\n  ]\n}\n") {
			t.Fatalf("layout:\n%s", after)
		}
	})

	t.Run("a document without tasks is refused [verifies SP-2558~1]", func(t *testing.T) {
		if _, err := accept.AppendTask([]byte(`{"arms": []}`), newTask("fare")); err == nil {
			t.Fatal("accepted")
		}
	})
}
