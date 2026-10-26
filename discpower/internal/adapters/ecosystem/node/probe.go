package node

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	"github.com/RachidChabane/musthold/discpower/internal/app/suiterun"
	"github.com/RachidChabane/musthold/discpower/internal/domain/differential"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/spec"
)

var _ ports.Probing = (*Strategy)(nil)

// ErrProbeFailed is returned when the probe script cannot answer a request.
var ErrProbeFailed = errors.New("probe failed")

// ProbeTimeout bounds one run of the probe script.
const ProbeTimeout = 2 * time.Minute

const (
	probeOperationSignatures = "signatures"
	probeOperationInvoke     = "invoke"

	scriptExtension   = ".mjs"
	requestExtension  = ".json"
	responseExtension = ".jsonl"
	filePermissions   = 0o600

	// probeExitFailure is the exit status of a script that could not read its
	// request, load a target or write its response.
	probeExitFailure = 70
	// killedExitCode is the exit status the process adapter reports for a
	// process that a signal ended.
	killedExitCode = -1

	// timezoneVariable makes times render in UTC, so a repeated call observes
	// the same thing.
	timezoneVariable = "TZ=UTC"

	raisesTimeout = "Timeout"
	raisesCrash   = "Crash"
)

//go:embed scripts/probe.mjs
var probeScript []byte

// Prober calls the bound operations of a project, `path/to/file.ts:function` or
// `path/to/file.ts:Class.method`, through an embedded Node script run inside the
// project.
func (s *Strategy) Prober() ports.Prober { return prober{deps: s.deps} }

type prober struct {
	deps Dependencies
}

type probeRequest struct {
	Operation string          `json:"op"`
	Targets   []string        `json:"targets,omitempty"`
	Calls     []probeCallJSON `json:"calls,omitempty"`
}

type probeCallJSON struct {
	Target string                  `json:"target"`
	Args   map[string]spec.Literal `json:"args"`
}

type signaturesResponse struct {
	Signatures []struct {
		Target string `json:"target"`
		Params []struct {
			Name string `json:"name"`
			Hint string `json:"hint"`
		} `json:"params"`
	} `json:"signatures"`
}

// Signatures learns the parameters of each target from the types the project's
// TypeScript compiler reads in its source, else from the defaults of its
// parameters.
func (p prober) Signatures(ctx context.Context, workspace ecosystem.Workspace, _ ecosystem.Toolchain, targets []string) ([]ports.ProbeSignature, error) {
	responseFile := suiterun.ResultsFile(workspace, requestExtension)
	run, err := p.execute(ctx, workspace, probeRequest{Operation: probeOperationSignatures, Targets: targets}, responseFile)
	if err != nil {
		return nil, err
	}
	if run.exitCode != 0 || run.timedOut {
		return nil, run.failure()
	}
	content, err := os.ReadFile(responseFile)
	if err != nil {
		return nil, fmt.Errorf("%w: read the response: %v", ErrProbeFailed, err)
	}
	var response signaturesResponse
	if err := json.Unmarshal(content, &response); err != nil {
		return nil, fmt.Errorf("%w: unreadable response: %v", ErrProbeFailed, err)
	}
	signatures := make([]ports.ProbeSignature, 0, len(response.Signatures))
	for _, signature := range response.Signatures {
		params := make([]differential.Param, 0, len(signature.Params))
		for _, param := range signature.Params {
			params = append(params, differential.Param{Name: param.Name, Hint: hintOf(param.Hint)})
		}
		signatures = append(signatures, ports.ProbeSignature{Target: signature.Target, Params: params})
	}
	return signatures, nil
}

// Invoke observes what each call returns or raises, in order. A call that
// exceeds its deadline or ends the process is an observation of its own, and the
// calls after it run in a new process, so no call hides another.
func (p prober) Invoke(ctx context.Context, workspace ecosystem.Workspace, _ ecosystem.Toolchain, calls []ports.ProbeCall) ([]spec.Observation, error) {
	observed := make([]spec.Observation, 0, len(calls))
	for len(observed) < len(calls) {
		run, err := p.invoke(ctx, workspace, calls[len(observed):])
		if err != nil {
			return nil, err
		}
		if run.failed() {
			return nil, run.failure()
		}
		ended := run.endedCall(len(calls) - len(observed))
		if len(run.observations)+len(ended) == 0 {
			return nil, fmt.Errorf("%w: the probe script answered nothing", ErrProbeFailed)
		}
		observed = append(observed, run.observations...)
		if len(observed) < len(calls) {
			observed = append(observed, ended...)
		}
	}
	return observed, nil
}

// probeRun is what one process of the probe script left behind.
type probeRun struct {
	observations []spec.Observation
	exitCode     int
	timedOut     bool
	stderr       string
}

func (r probeRun) failed() bool { return r.exitCode == probeExitFailure }

func (r probeRun) failure() error {
	return fmt.Errorf("%w: exit %d, timed out %t: %s", ErrProbeFailed, r.exitCode, r.timedOut, r.stderr)
}

// endedCall is the observation of the call a process died or was killed in,
// which is the one after the calls it answered. A process the script's own
// watchdog killed has reported that call's timeout already.
func (r probeRun) endedCall(requested int) []spec.Observation {
	switch {
	case r.timedOut:
		return []spec.Observation{{Kind: spec.Raises, Value: raisesTimeout}}
	case len(r.observations) >= requested || r.endedByWatchdog():
		return nil
	}
	return []spec.Observation{{Kind: spec.Raises, Value: raisesCrash}}
}

func (r probeRun) endedByWatchdog() bool {
	last := len(r.observations) - 1
	return r.exitCode == killedExitCode && last >= 0 && r.observations[last].Kind == spec.Raises && r.observations[last].Value == raisesTimeout
}

func (p prober) invoke(ctx context.Context, workspace ecosystem.Workspace, calls []ports.ProbeCall) (probeRun, error) {
	request := probeRequest{Operation: probeOperationInvoke, Calls: make([]probeCallJSON, 0, len(calls))}
	for _, call := range calls {
		args := call.Args
		if args == nil {
			args = map[string]spec.Literal{}
		}
		request.Calls = append(request.Calls, probeCallJSON{Target: call.Target, Args: args})
	}
	responseFile := suiterun.ResultsFile(workspace, responseExtension)
	run, err := p.execute(ctx, workspace, request, responseFile)
	if err != nil {
		return probeRun{}, err
	}
	if run.observations, err = readObservations(responseFile); err != nil {
		return probeRun{}, err
	}
	if len(run.observations) > len(calls) {
		return probeRun{}, fmt.Errorf("%w: %d observations for %d calls", ErrProbeFailed, len(run.observations), len(calls))
	}
	return run, nil
}

// execute writes the script and the request outside the workspace and runs the
// script inside it.
func (p prober) execute(ctx context.Context, workspace ecosystem.Workspace, request probeRequest, responseFile string) (probeRun, error) {
	node, err := p.deps.LookPath(nodeCommand)
	if err != nil {
		return probeRun{}, fmt.Errorf("%w: %v", ErrNoToolchain, err)
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		return probeRun{}, fmt.Errorf("encode the probe request: %w", err)
	}
	script := suiterun.ResultsFile(workspace, scriptExtension)
	requestFile := suiterun.ResultsFile(workspace, requestExtension)
	for name, content := range map[string][]byte{script: probeScript, requestFile: encoded} {
		if err := os.WriteFile(name, content, filePermissions); err != nil {
			return probeRun{}, fmt.Errorf("write %s: %w", name, err)
		}
	}
	result, err := p.deps.Process.Run(ctx, ports.Command{
		Argv: []string{node, script, requestFile, responseFile}, Dir: workspace.Dir,
		Env: []string{timezoneVariable}, Timeout: ProbeTimeout,
	})
	if err != nil {
		return probeRun{}, fmt.Errorf("run the probe script: %w", err)
	}
	return probeRun{exitCode: result.ExitCode, timedOut: result.TimedOut, stderr: strings.TrimSpace(string(result.Stderr))}, nil
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

var knownHints = map[string]differential.Hint{
	string(differential.HintInteger):  differential.HintInteger,
	string(differential.HintDecimal):  differential.HintDecimal,
	string(differential.HintString):   differential.HintString,
	string(differential.HintBoolean):  differential.HintBoolean,
	string(differential.HintDatetime): differential.HintDatetime,
}

// hintOf maps a hint name of the probe script to a domain hint; a name outside
// the vocabulary is unknown.
func hintOf(name string) differential.Hint {
	if hint, known := knownHints[name]; known {
		return hint
	}
	return differential.HintUnknown
}
