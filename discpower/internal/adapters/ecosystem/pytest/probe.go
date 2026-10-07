package pytest

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/suiterun"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/differential"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
)

var _ ports.Probing = (*Strategy)(nil)

// ErrProbeFailed is returned when the probe script cannot answer a request.
var ErrProbeFailed = errors.New("probe failed")

// ProbeTimeout bounds one probe script run.
const ProbeTimeout = 2 * time.Minute

const (
	probeOperationSignatures = "signatures"
	probeOperationInvoke     = "invoke"

	probeScriptExtension = ".py"
	probeFileExtension   = ".json"
	probeFilePermissions = 0o600
)

//go:embed scripts/probe.py
var probeScript []byte

// Prober runs calls of bound operations through an embedded Python script.
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

type probeResponse struct {
	Error        string             `json:"error"`
	Signatures   []signatureJSON    `json:"signatures"`
	Observations []spec.Observation `json:"observations"`
}

type signatureJSON struct {
	Target string      `json:"target"`
	Params []paramJSON `json:"params"`
}

type paramJSON struct {
	Name string `json:"name"`
	Hint string `json:"hint"`
}

// Signatures learns the parameters of each target from its annotations and
// defaults.
func (p prober) Signatures(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, targets []string) ([]ports.ProbeSignature, error) {
	response, err := p.exchange(ctx, workspace, toolchain, probeRequest{Operation: probeOperationSignatures, Targets: targets})
	if err != nil {
		return nil, err
	}
	signatures := make([]ports.ProbeSignature, 0, len(response.Signatures))
	for _, signature := range response.Signatures {
		signatures = append(signatures, ports.ProbeSignature{Target: signature.Target, Params: paramsOf(signature.Params)})
	}
	return signatures, nil
}

// Invoke observes what each call returns or raises, in order.
func (p prober) Invoke(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, calls []ports.ProbeCall) ([]spec.Observation, error) {
	request := probeRequest{Operation: probeOperationInvoke, Calls: make([]probeCallJSON, 0, len(calls))}
	for _, call := range calls {
		request.Calls = append(request.Calls, probeCallJSON{Target: call.Target, Args: nonNilArgs(call.Args)})
	}
	response, err := p.exchange(ctx, workspace, toolchain, request)
	if err != nil {
		return nil, err
	}
	if len(response.Observations) != len(calls) {
		return nil, fmt.Errorf("%w: %d observations for %d calls", ErrProbeFailed, len(response.Observations), len(calls))
	}
	return response.Observations, nil
}

func (p prober) exchange(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, request probeRequest) (probeResponse, error) {
	script := suiterun.ResultsFile(workspace, probeScriptExtension)
	requestFile := suiterun.ResultsFile(workspace, probeFileExtension)
	responseFile := suiterun.ResultsFile(workspace, probeFileExtension)
	encoded, err := json.Marshal(request)
	if err != nil {
		return probeResponse{}, fmt.Errorf("encode the probe request: %w", err)
	}
	if err := writeFiles(map[string][]byte{script: probeScript, requestFile: encoded}); err != nil {
		return probeResponse{}, err
	}
	argv := append(append([]string{}, toolchain.Command...), script, requestFile, responseFile)
	result, err := p.deps.Process.Run(ctx, ports.Command{
		Argv:    argv,
		Dir:     workspace.Dir,
		Env:     []string{noBytecodeVariable, pythonPathVariable + importPath(workspace)},
		Timeout: ProbeTimeout,
	})
	if err != nil {
		return probeResponse{}, fmt.Errorf("run the probe script: %w", err)
	}
	return readProbeResponse(responseFile, result)
}

func readProbeResponse(responseFile string, result ports.CommandResult) (probeResponse, error) {
	content, err := os.ReadFile(responseFile)
	if err != nil {
		return probeResponse{}, fmt.Errorf("%w: exit %d, timed out %t: %s", ErrProbeFailed, result.ExitCode, result.TimedOut, strings.TrimSpace(string(result.Stderr)))
	}
	var response probeResponse
	if err := json.Unmarshal(content, &response); err != nil {
		return probeResponse{}, fmt.Errorf("%w: unreadable response: %v", ErrProbeFailed, err)
	}
	if response.Error != "" {
		return probeResponse{}, fmt.Errorf("%w: %s", ErrProbeFailed, response.Error)
	}
	return response, nil
}

func writeFiles(files map[string][]byte) error {
	for name, content := range files {
		if err := os.WriteFile(name, content, probeFilePermissions); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}
	return nil
}

func nonNilArgs(args map[string]spec.Literal) map[string]spec.Literal {
	if args == nil {
		return map[string]spec.Literal{}
	}
	return args
}

func paramsOf(params []paramJSON) []differential.Param {
	result := make([]differential.Param, 0, len(params))
	for _, param := range params {
		result = append(result, differential.Param{Name: param.Name, Hint: hintOf(param.Hint)})
	}
	return result
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
