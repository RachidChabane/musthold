package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

// Exit codes of the command.
const (
	exitOK      = 0
	exitFailure = 1
	exitUsage   = 2
)

const (
	recordingExtension = ".json"
	flagPersona        = "persona"
	flagModule         = "module-bin"
	flagRecordings     = "recordings"
	flagWait           = "wait"
	flagList           = "list"
)

// runnerFactory builds the runner of a module binary.
type runnerFactory func(binary string) hostprototest.ModuleRunner

// execRunner runs the module binary as a subprocess.
func execRunner(binary string) hostprototest.ModuleRunner { return processRunner{binary: binary} }

type processRunner struct{ binary string }

// Run implements hostprototest.ModuleRunner.
func (p processRunner) Run(args []string, stdin []byte) ([]byte, error) {
	command := exec.Command(p.binary, args...)
	command.Stdin = bytes.NewReader(stdin)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	stdout, err := command.Output()
	if err != nil {
		return stdout, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout, nil
}

func run(args []string, stdout, stderr io.Writer, newRunner runnerFactory, sleep func(time.Duration)) int {
	flags := flag.NewFlagSet("fakehost", flag.ContinueOnError)
	flags.SetOutput(stderr)
	persona := flags.String(flagPersona, hostprototest.PersonaHonest, "persona to play: "+strings.Join(hostprototest.CLIPersonaNames(), ", "))
	binary := flags.String(flagModule, "", "module binary to drive")
	recordings := flags.String(flagRecordings, "", "directory of recorded answers named <requestId>.json; without it answers derive from the response schema")
	wait := flags.Bool(flagWait, false, "really wait where the persona waits (the slow persona waits past the ticket time to live)")
	list := flags.Bool(flagList, false, "list the personas and stop")
	if err := flags.Parse(args); err != nil {
		return exitUsage
	}
	if *list {
		fmt.Fprintln(stdout, strings.Join(hostprototest.CLIPersonaNames(), "\n"))
		return exitOK
	}
	if *binary == "" {
		fmt.Fprintf(stderr, "fakehost: -%s is required\n", flagModule)
		return exitUsage
	}
	answers, err := answerSource(*recordings)
	if err != nil {
		fmt.Fprintf(stderr, "fakehost: %v\n", err)
		return exitFailure
	}
	pause := func(time.Duration) {}
	if *wait {
		pause = sleep
	}
	results, err := hostprototest.PlayCLI(newRunner(*binary), *persona, answers, pause)
	encoder := json.NewEncoder(stdout)
	for _, result := range results {
		if encodeErr := encoder.Encode(result); encodeErr != nil {
			fmt.Fprintf(stderr, "fakehost: %v\n", encodeErr)
			return exitFailure
		}
	}
	if err != nil {
		fmt.Fprintf(stderr, "fakehost: %v\n", err)
		if errors.Is(err, hostprototest.ErrUnsupported) {
			return exitUsage
		}
		return exitFailure
	}
	return exitOK
}

// answerSource reads the recordings of a directory, or derives answers from
// schemas when none is given.
func answerSource(directory string) (hostprototest.AnswerSource, error) {
	if directory == "" {
		return hostprototest.SchemaAnswers{}, nil
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	recordings := hostprototest.Recordings{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, recordingExtension) {
			continue
		}
		content, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil {
			return nil, err
		}
		recordings[hostproto.RequestID(strings.TrimSuffix(name, recordingExtension))] = string(content)
	}
	return recordings, nil
}
