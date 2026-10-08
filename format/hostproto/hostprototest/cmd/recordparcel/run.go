package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/RachidChabane/spec-proof/format/formattest/parcel"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

// Exit codes of the command, and of the module commands it drives.
const (
	exitOK         = 0
	exitFailure    = 1
	exitUsage      = 2
	exitAwaitsHost = 3
)

const (
	flagBinaries = "binaries"
	flagFixture  = "fixture"
	flagOut      = "out"

	recordingExtension = ".json"
	maxWaves           = 12
	hooksNone          = "none"
	stateEnv           = "XDG_STATE_HOME"
	pathEnv            = "PATH"
	shallnotEnv        = "SPEC_PROOF_SHALLNOT"
	shallnotBinary     = "shallnot"
	cacheEnv           = "XDG_CACHE_HOME"
	recordDirectory    = "record"
	treeDirectory      = "tree"
	stateDirectory     = "state"
	cacheDirectory     = "cache"
	scratchPattern     = "recordparcel-"
	honestPersona      = hostprototest.PersonaHonest
)

// moduleHost is a proof module's binary with the honest host that answers its tickets.
type moduleHost struct {
	binary string
	source hostprototest.AnswerSource
}

// moduleHosts lists the proof modules in the order they are proved in.
func moduleHosts() []moduleHost {
	return []moduleHost{
		{"spec-proof-dp", parcel.Oracle{}},
		{"spec-proof-alignment", parcel.HonestAlignment()},
		{"spec-proof-completeness", FirstOption{}},
	}
}

// processRunner runs one module binary in a tree with a fixed environment.
type processRunner struct {
	binary      string
	directory   string
	environment []string
}

// Run implements hostprototest.ModuleRunner.
func (p processRunner) Run(args []string, stdin []byte) ([]byte, error) {
	command := exec.Command(p.binary, args...)
	command.Dir = p.directory
	command.Env = p.environment
	command.Stdin = bytes.NewReader(stdin)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	stdout, err := command.Output()
	if err != nil {
		return stdout, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout, nil
}

// exitCode runs the module and returns its exit code; a failure to start it is an error.
func (p processRunner) exitCode(args ...string) (int, error) {
	_, err := p.Run(args, nil)
	var exit *exec.ExitError
	switch {
	case err == nil:
		return exitOK, nil
	case errors.As(err, &exit):
		return exit.ExitCode(), nil
	}
	return 0, err
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("recordparcel", flag.ContinueOnError)
	flags.SetOutput(stderr)
	binaries := flags.String(flagBinaries, "", "directory of the proof module binaries")
	fixture := flags.String(flagFixture, "", "directory of the parcel project")
	out := flags.String(flagOut, "", "directory the recordings are written to")
	if err := flags.Parse(args); err != nil {
		return exitUsage
	}
	if *binaries == "" || *fixture == "" || *out == "" {
		fmt.Fprintf(stderr, "recordparcel: -%s, -%s and -%s are required\n", flagBinaries, flagFixture, flagOut)
		return exitUsage
	}
	count, err := record(*binaries, *fixture, *out)
	if err != nil {
		fmt.Fprintf(stderr, "recordparcel: %v\n", err)
		return exitFailure
	}
	fmt.Fprintf(stdout, "recordparcel: wrote %d recordings to %s\n", count, *out)
	return exitOK
}

// record plays every module over the parcel project and writes the recordings.
func record(binaries, fixture, out string) (int, error) {
	binaries, err := filepath.Abs(binaries)
	if err != nil {
		return 0, err
	}
	scratch, err := os.MkdirTemp("", scratchPattern)
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(scratch)
	tree := filepath.Join(scratch, treeDirectory)
	if err := BuildParcelTree(fixture, tree); err != nil {
		return 0, err
	}
	environment := append(os.Environ(),
		stateEnv+"="+filepath.Join(scratch, stateDirectory),
		cacheEnv+"="+filepath.Join(scratch, cacheDirectory),
		shallnotEnv+"="+filepath.Join(binaries, shallnotBinary),
		pathEnv+"="+binaries+string(os.PathListSeparator)+os.Getenv(pathEnv),
	)
	gate := processRunner{binary: filepath.Join(binaries, shallnotBinary), directory: tree, environment: environment}
	if err := installGate(gate, tree); err != nil {
		return 0, err
	}
	recorded := map[hostproto.RequestID]string{}
	for _, host := range moduleHosts() {
		runner := processRunner{binary: filepath.Join(binaries, host.binary), directory: tree, environment: environment}
		recorder := NewRecorder(host.source)
		if err := playModule(runner, recorder, filepath.Join(scratch, recordDirectory, host.binary), tree); err != nil {
			return 0, fmt.Errorf("%s: %w", host.binary, err)
		}
		for id, answer := range recorder.Answers {
			recorded[id] = answer
		}
	}
	return len(recorded), writeRecordings(out, recorded)
}

// installGate gives the tree the Shallnot configuration the modules read.
func installGate(gate processRunner, tree string) error {
	_, err := gate.Run([]string{"init", "--dir", tree, "--hooks", hooksNone}, nil)
	return err
}

// playModule installs a module in the tree and answers its tickets wave by wave
// until it has proved the change.
func playModule(runner processRunner, source hostprototest.AnswerSource, recordDir, tree string) error {
	for _, arguments := range [][]string{
		{"init", "--dir", tree, "--hooks", hooksNone},
		{"enable", "--dir", tree},
	} {
		if _, err := runner.Run(arguments, nil); err != nil {
			return err
		}
	}
	prove := []string{"prove", "--changed", "--root", tree, "--record", recordDir}
	code, err := runner.exitCode(prove...)
	for wave := 0; err == nil && code == exitAwaitsHost; wave++ {
		if wave == maxWaves {
			return fmt.Errorf("still waiting for the host after %d waves", maxWaves)
		}
		if err = answerWave(runner, source); err != nil {
			return err
		}
		code, err = runner.exitCode(append(prove, "--resume")...)
	}
	if err == nil && code != exitOK && code != exitFailure {
		err = fmt.Errorf("prove exited with %d", code)
	}
	return err
}

func answerWave(runner processRunner, source hostprototest.AnswerSource) error {
	results, err := hostprototest.PlayCLI(runner, honestPersona, source, func(time.Duration) {})
	if err != nil {
		return err
	}
	for _, result := range results {
		if !result.Submitted {
			return fmt.Errorf("ticket %s was not accepted: %s", result.Ticket, result.Error)
		}
	}
	return nil
}

func writeRecordings(out string, recorded map[hostproto.RequestID]string) error {
	if err := os.MkdirAll(out, directoryMode); err != nil {
		return err
	}
	for id, answer := range recorded {
		if err := os.WriteFile(filepath.Join(out, string(id)+recordingExtension), []byte(answer), fileMode); err != nil {
			return err
		}
	}
	return nil
}
