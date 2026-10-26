package golang

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/RachidChabane/musthold/discpower/internal/adapters/ecosystem/golang/probeharness"
	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	"github.com/RachidChabane/musthold/discpower/internal/app/suiterun"
	"github.com/RachidChabane/musthold/discpower/internal/domain/differential"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/spec"
)

var _ ports.Probing = (*Strategy)(nil)

// ErrProbeFailed is returned when the probe program cannot be built or cannot
// answer a request.
var ErrProbeFailed = errors.New("probe failed")

// ProbeTimeout bounds one run of the probe program.
const ProbeTimeout = 2 * time.Minute

const (
	buildSubcommand  = "build"
	trimpathFlag     = "-trimpath"
	noVCSFlag        = "-buildvcs=false"
	overlayFlag      = "-overlay"
	outputFlag       = "-o"
	currentDirectory = "./"

	requestExtension  = ".json"
	responseExtension = ".jsonl"
	overlayExtension  = ".json"
	sourceExtension   = ".go.txt"
	programExtension  = ".probe"
	filePermissions   = 0o600

	workingDirectoryVariable = "PWD="
	// timezoneVariable and randomSeedVariable make a probe run repeat: times
	// render in UTC and the default random source is not seeded from the clock.
	timezoneVariable   = "TZ=UTC"
	randomSeedVariable = "GODEBUG=randautoseed=0"
)

// Prober calls the bound operations of a project through a program generated for
// them. Each program is a `main` package that the build overlay places inside
// the module, so that it imports the packages of the project, internal ones
// included, while nothing is written into the workspace.
func (s *Strategy) Prober() ports.Prober { return prober{deps: s.deps} }

type prober struct {
	deps Dependencies
}

// Signatures reads the parameters of each target from the types of its package.
func (p prober) Signatures(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, targets []string) ([]ports.ProbeSignature, error) {
	parsed, err := parseTargets(targets)
	if err != nil {
		return nil, err
	}
	workspace, err = resolved(workspace)
	if err != nil {
		return nil, err
	}
	operations, err := p.loadOperations(ctx, workspace, toolchain, parsed)
	if err != nil {
		return nil, err
	}
	signatures := make([]ports.ProbeSignature, 0, len(operations))
	for _, operation := range operations {
		params := make([]differential.Param, 0, len(operation.Params))
		for _, param := range operation.Params {
			params = append(params, differential.Param{Name: param.Name, Hint: hintOf(param)})
		}
		signatures = append(signatures, ports.ProbeSignature{Target: operation.Target.Text, Params: params})
	}
	return signatures, nil
}

// Invoke observes what each call returns or raises, in order.
func (p prober) Invoke(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, calls []ports.ProbeCall) ([]spec.Observation, error) {
	if len(calls) == 0 {
		return nil, nil
	}
	var texts []string
	for _, call := range calls {
		if !slices.Contains(texts, call.Target) {
			texts = append(texts, call.Target)
		}
	}
	parsed, err := parseTargets(texts)
	if err != nil {
		return nil, err
	}
	workspace, err = resolved(workspace)
	if err != nil {
		return nil, err
	}
	operations, err := p.loadOperations(ctx, workspace, toolchain, parsed)
	if err != nil {
		return nil, err
	}
	observations := make([]spec.Observation, len(calls))
	for _, group := range groupOperations(workspace, operations) {
		served := servedBy(group, calls)
		answers, err := p.runGroup(ctx, workspace, toolchain, group, pick(calls, served))
		if err != nil {
			return nil, err
		}
		for position, index := range served {
			observations[index] = answers[position]
		}
	}
	return observations, nil
}

func parseTargets(texts []string) ([]probeTarget, error) {
	parsed := make([]probeTarget, 0, len(texts))
	for _, text := range texts {
		target, err := parseProbeTarget(text)
		if err != nil {
			return nil, err
		}
		parsed = append(parsed, target)
	}
	return parsed, nil
}

// resolved returns the workspace with its directory spelled as the go command
// spells it, symbolic links followed, so that the paths of the build overlay are
// the ones the build looks up.
func resolved(workspace ecosystem.Workspace) (ecosystem.Workspace, error) {
	dir, err := filepath.EvalSymlinks(workspace.Dir)
	if err != nil {
		return ecosystem.Workspace{}, fmt.Errorf("resolve the workspace: %w", err)
	}
	workspace.Dir = dir
	return workspace, nil
}

// servedBy lists the indexes of the calls whose target the group calls.
func servedBy(group programGroup, calls []ports.ProbeCall) []int {
	var indexes []int
	for index, call := range calls {
		if slices.ContainsFunc(group.Operations, func(operation operation) bool { return operation.Target.Text == call.Target }) {
			indexes = append(indexes, index)
		}
	}
	return indexes
}

func pick(calls []ports.ProbeCall, indexes []int) []ports.ProbeCall {
	picked := make([]ports.ProbeCall, len(indexes))
	for position, index := range indexes {
		picked[position] = calls[index]
	}
	return picked
}

// runGroup builds the program of the group and has it make the calls. A call
// that kills the program or outlives its deadline is an observation of its own
// and the calls after it run in a fresh process, so no call hides another.
func (p prober) runGroup(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, group programGroup, calls []ports.ProbeCall) ([]spec.Observation, error) {
	program, err := p.build(ctx, workspace, toolchain, group)
	if err != nil {
		return nil, err
	}
	observed := make([]spec.Observation, 0, len(calls))
	for len(observed) < len(calls) {
		run, err := p.execute(ctx, workspace, program, calls[len(observed):])
		if err != nil {
			return nil, err
		}
		ended := run.endedCall()
		if len(run.observations)+len(ended) == 0 {
			return nil, fmt.Errorf("%w: the probe program answered nothing", ErrProbeFailed)
		}
		observed = append(observed, run.observations...)
		if len(observed) < len(calls) {
			observed = append(observed, ended...)
		}
	}
	return observed, nil
}

func (p prober) build(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, group programGroup) (string, error) {
	rendered, err := renderTable(group.Operations)
	if err != nil {
		return "", err
	}
	tableFile := suiterun.ResultsFile(workspace, sourceExtension)
	harnessFile := suiterun.ResultsFile(workspace, sourceExtension)
	overlayFile := suiterun.ResultsFile(workspace, overlayExtension)
	program := suiterun.ResultsFile(workspace, programExtension)
	overlay, err := overlayOf(group.Dir, tableFile, harnessFile)
	if err != nil {
		return "", err
	}
	if err := writeFiles(map[string][]byte{tableFile: rendered, harnessFile: probeharness.ProgramSource(), overlayFile: overlay}); err != nil {
		return "", err
	}
	relative, err := filepath.Rel(workspace.Dir, filepath.Join(group.Dir, probeDirectory))
	if err != nil {
		return "", fmt.Errorf("place the probe program: %w", err)
	}
	argv := append(append([]string{}, toolchain.Command...), buildSubcommand, trimpathFlag, noVCSFlag, overlayFlag, overlayFile, outputFlag, program, currentDirectory+filepath.ToSlash(relative))
	result, err := p.deps.Process.Run(ctx, ports.Command{Argv: argv, Dir: workspace.Dir, Env: []string{workingDirectoryVariable + workspace.Dir}, Timeout: p.deps.Timeout})
	if err != nil {
		return "", fmt.Errorf("build the probe program: %w", err)
	}
	if result.TimedOut || result.ExitCode != 0 {
		return "", fmt.Errorf("%w: build: %s", ErrProbeFailed, strings.TrimSpace(string(result.Stdout)+string(result.Stderr)))
	}
	return program, nil
}

// probeRun is what one process of the probe program left behind.
type probeRun struct {
	observations []spec.Observation
	exitCode     int
	timedOut     bool
}

// endedCall is the observation of the call a process died or was killed in,
// which is the one after the calls it answered. A process that stopped by itself
// ended its last call by a timeout the program already reported.
func (r probeRun) endedCall() []spec.Observation {
	switch {
	case r.timedOut:
		return []spec.Observation{{Kind: spec.Raises, Value: probeharness.RaisesTimeout}}
	case r.exitCode != probeharness.ExitClean:
		return []spec.Observation{{Kind: spec.Raises, Value: probeharness.RaisesCrash}}
	}
	return nil
}

type probeRequest struct {
	Calls []probeCallJSON `json:"calls"`
}

type probeCallJSON struct {
	Target string                  `json:"target"`
	Args   map[string]spec.Literal `json:"args"`
}

func (p prober) execute(ctx context.Context, workspace ecosystem.Workspace, program string, calls []ports.ProbeCall) (probeRun, error) {
	request := probeRequest{Calls: make([]probeCallJSON, 0, len(calls))}
	for _, call := range calls {
		args := call.Args
		if args == nil {
			args = map[string]spec.Literal{}
		}
		request.Calls = append(request.Calls, probeCallJSON{Target: call.Target, Args: args})
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		return probeRun{}, fmt.Errorf("encode the probe request: %w", err)
	}
	requestFile := suiterun.ResultsFile(workspace, requestExtension)
	responseFile := suiterun.ResultsFile(workspace, responseExtension)
	if err := writeFiles(map[string][]byte{requestFile: encoded}); err != nil {
		return probeRun{}, err
	}
	result, err := p.deps.Process.Run(ctx, ports.Command{
		Argv: []string{program, requestFile, responseFile}, Dir: workspace.Dir,
		Env: []string{timezoneVariable, randomSeedVariable}, Timeout: ProbeTimeout,
	})
	if err != nil {
		return probeRun{}, fmt.Errorf("run the probe program: %w", err)
	}
	if result.ExitCode == probeharness.ExitFailure {
		return probeRun{}, fmt.Errorf("%w: %s", ErrProbeFailed, strings.TrimSpace(string(result.Stderr)))
	}
	observations, err := readObservations(responseFile)
	if err != nil {
		return probeRun{}, err
	}
	if len(observations) > len(calls) {
		return probeRun{}, fmt.Errorf("%w: %d observations for %d calls", ErrProbeFailed, len(observations), len(calls))
	}
	return probeRun{observations: observations, exitCode: result.ExitCode, timedOut: result.TimedOut}, nil
}

// readObservations reads the complete lines of the response file; a missing file
// holds none, and a line the process was killed in the middle of is dropped.
func readObservations(file string) ([]spec.Observation, error) {
	content, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read the probe response: %w", err)
	}
	complete := content[:bytes.LastIndexByte(content, '\n')+1]
	var observations []spec.Observation
	for _, line := range bytes.Split(bytes.TrimSuffix(complete, []byte{'\n'}), []byte{'\n'}) {
		if len(line) == 0 {
			continue
		}
		var observation spec.Observation
		if err := json.Unmarshal(line, &observation); err != nil {
			return nil, fmt.Errorf("%w: unreadable response line: %v", ErrProbeFailed, err)
		}
		observations = append(observations, observation)
	}
	return observations, nil
}

func writeFiles(files map[string][]byte) error {
	for name, content := range files {
		if err := os.WriteFile(name, content, filePermissions); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}
	return nil
}
