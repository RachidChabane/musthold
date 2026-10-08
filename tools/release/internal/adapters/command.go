// Package adapters connects the release use cases to git, the Go toolchain, the
// file system and the archive formats.
package adapters

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
)

// outputTailBytes is how much of a failed command's output a finding keeps: the
// end, where the error is.
const outputTailBytes = 4000

// commandRunner runs commands in a directory with a controlled environment.
type commandRunner struct {
	dir string
}

// run executes a command and returns its combined output. The environment is the
// caller's, minus the variables the overrides set, plus the overrides.
func (r commandRunner) run(overrides []string, name string, arguments ...string) (string, error) {
	command := exec.Command(name, arguments...)
	command.Dir = r.dir
	command.Env = environmentWith(os.Environ(), overrides)
	var output bytes.Buffer
	command.Stdout, command.Stderr = &output, &output
	err := command.Run()
	return output.String(), err
}

// environmentWith drops the variables the overrides set from the base and appends
// the overrides.
func environmentWith(base, overrides []string) []string {
	var names []string
	for _, override := range overrides {
		name, _, _ := strings.Cut(override, "=")
		names = append(names, name)
	}
	environment := make([]string, 0, len(base)+len(overrides))
	for _, entry := range base {
		name, _, _ := strings.Cut(entry, "=")
		if !slices.Contains(names, name) {
			environment = append(environment, entry)
		}
	}
	return append(environment, overrides...)
}

// failureText renders the end of a failed command's output with its error.
func failureText(output string, err error) string {
	trimmed := strings.TrimSpace(output)
	if len(trimmed) > outputTailBytes {
		trimmed = "..." + trimmed[len(trimmed)-outputTailBytes:]
	}
	if trimmed == "" {
		return err.Error()
	}
	return fmt.Sprintf("%v\n%s", err, trimmed)
}
