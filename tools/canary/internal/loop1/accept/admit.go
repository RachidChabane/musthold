package accept

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/config"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

const (
	tasksMember   = "tasks"
	entryPrefix   = "    "
	entryIndent   = "  "
	closingPrefix = "  "
	configMode    = 0o644
)

// ErrNoTasksMember is returned for a run configuration with no `tasks` array.
var ErrNoTasksMember = errors.New("the run configuration has no tasks array")

// admit appends a task that passed every check to the run configuration, unless
// it is already listed. Every other byte of the file stays as it was.
func (a *Acceptor) admit(task study.Task) error {
	data, err := os.ReadFile(a.Config)
	if err != nil {
		return fmt.Errorf("read the run configuration: %w", err)
	}
	updated, err := AppendTask(data, ConfigTask(task))
	if err != nil {
		return err
	}
	if bytes.Equal(updated, data) {
		return nil
	}
	if err := os.WriteFile(a.Config, updated, configMode); err != nil {
		return fmt.Errorf("write the run configuration: %w", err)
	}
	return nil
}

// ConfigTask is the entry of a task in the run configuration.
func ConfigTask(task study.Task) config.Task {
	return config.Task{Name: task.Name, Fixture: task.Fixture, Prompt: task.Prompt, Operations: task.Operations}
}

// AppendTask adds a task at the end of the `tasks` array of a run configuration,
// formatted as its other entries, and returns the configuration unchanged when a
// task of that name is already listed. The result must still read as a list of tasks.
func AppendTask(data []byte, task config.Task) ([]byte, error) {
	array, end, err := locateTasks(data)
	if err != nil {
		return nil, err
	}
	var listed []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(array, &listed); err != nil {
		return nil, fmt.Errorf("read the tasks of the run configuration: %w", err)
	}
	for _, entry := range listed {
		if entry.Name == task.Name {
			return data, nil
		}
	}
	entry, err := json.MarshalIndent(task, entryPrefix, entryIndent)
	if err != nil {
		return nil, fmt.Errorf("encode the task %s: %w", task.Name, err)
	}
	insertion, at := insertionPoint(data, end, entry, len(listed) == 0)
	updated := append(append(append([]byte(nil), data[:at]...), insertion...), data[at:]...)
	if err := checkTasks(updated); err != nil {
		return nil, fmt.Errorf("the run configuration with %s added is refused: %w", task.Name, err)
	}
	return updated, nil
}

// checkTasks reads the tasks of a configuration as the runner reads them: no
// unknown member, no two tasks of one name.
func checkTasks(data []byte) error {
	var document struct {
		Tasks []json.RawMessage `json:"tasks"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return fmt.Errorf("not a JSON document: %w", err)
	}
	seen := map[string]bool{}
	for index, raw := range document.Tasks {
		var task config.Task
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&task); err != nil {
			return fmt.Errorf("tasks[%d]: %w", index, err)
		}
		if seen[task.Name] {
			return fmt.Errorf("tasks[%d]: %q names two tasks", index, task.Name)
		}
		seen[task.Name] = true
	}
	return nil
}

// locateTasks returns the text of the `tasks` array and the offset just after it.
func locateTasks(data []byte) ([]byte, int, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return nil, 0, fmt.Errorf("%w: not a JSON object", ErrNoTasksMember)
	}
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return nil, 0, fmt.Errorf("%w: %v", ErrNoTasksMember, err)
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, 0, fmt.Errorf("%w: %v", ErrNoTasksMember, err)
		}
		if key == tasksMember {
			return value, int(decoder.InputOffset()), nil
		}
	}
	return nil, 0, ErrNoTasksMember
}

// insertionPoint finds where the new entry goes: just after the last entry, or
// inside the brackets of an empty array.
func insertionPoint(data []byte, end int, entry []byte, empty bool) ([]byte, int) {
	at := end - 1
	for at > 0 && strings.ContainsRune(" \t\r\n", rune(data[at-1])) {
		at--
	}
	text := entryPrefix + string(entry)
	if empty {
		return []byte("\n" + text + "\n" + closingPrefix), at
	}
	return []byte(",\n" + text), at
}
