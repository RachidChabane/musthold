package pytest

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	"github.com/RachidChabane/musthold/discpower/internal/app/suiterun"
	"github.com/RachidChabane/musthold/discpower/internal/domain/differential"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
)

var _ ports.Compiling = (*Strategy)(nil)

//go:embed scripts/forms.py
var formsScript []byte

// Compiler compiles the units of a workspace with the interpreter of the
// project and digests each code object with its positions and file name left
// out (E0).
func (s *Strategy) Compiler() ports.FormCompiler { return compiler{deps: s.deps} }

type compiler struct {
	deps Dependencies
}

type formsRequest struct {
	Sites []string `json:"sites"`
}

type formsResponse struct {
	Error string            `json:"error"`
	Forms map[string]string `json:"forms"`
}

// Forms returns the compiled form of every site the interpreter compiles.
func (c compiler) Forms(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, sites []string) (differential.CompiledForms, error) {
	script := suiterun.ResultsFile(workspace, probeScriptExtension)
	requestFile := suiterun.ResultsFile(workspace, probeFileExtension)
	responseFile := suiterun.ResultsFile(workspace, probeFileExtension)
	encoded, err := json.Marshal(formsRequest{Sites: sites})
	if err != nil {
		return nil, fmt.Errorf("encode the forms request: %w", err)
	}
	if err := writeFiles(map[string][]byte{script: formsScript, requestFile: encoded}); err != nil {
		return nil, err
	}
	result, err := c.deps.Process.Run(ctx, ports.Command{
		Argv:    append(append([]string{}, toolchain.Command...), script, requestFile, responseFile),
		Dir:     workspace.Dir,
		Env:     []string{noBytecodeVariable},
		Timeout: ProbeTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("run the forms script: %w", err)
	}
	content, err := os.ReadFile(responseFile)
	if err != nil {
		return nil, fmt.Errorf("%w: exit %d: %s", ErrProbeFailed, result.ExitCode, strings.TrimSpace(string(result.Stderr)))
	}
	var response formsResponse
	if err := json.Unmarshal(content, &response); err != nil || response.Error != "" {
		return nil, fmt.Errorf("%w: forms: %v %s", ErrProbeFailed, err, response.Error)
	}
	return differential.CompiledForms(response.Forms), nil
}
