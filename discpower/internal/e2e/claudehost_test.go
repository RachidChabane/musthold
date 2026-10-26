package e2e_test

import (
	"os/exec"

	"github.com/RachidChabane/musthold/discpower/internal/domain/hostharness"
	"github.com/RachidChabane/musthold/format/hostproto/hostprototest/claudecode"
)

// moduleOf is the module under test as the simulated Claude Code runs it: its
// binary in the project, its executor server and the tool names of the Claude
// harness.
type moduleOf struct {
	p       *project
	harness hostharness.Harness
}

func (m moduleOf) Run(stdin string, args ...string) claudecode.Result {
	r := m.p.dp(stdin, args...)
	return claudecode.Result{Code: r.code, Stdout: r.stdout, Stderr: r.stderr}
}

func (m moduleOf) Server() *exec.Cmd {
	command := exec.Command(m.p.binary, "mcp", "--role", "executor", "--harness", "claude")
	command.Dir = m.p.root
	command.Env = m.p.environment()
	return command
}

func (m moduleOf) ToolName(tool string) string { return m.harness.ToolName(tool) }

func (p *project) module() moduleOf { return moduleOf{p: p, harness: hostharness.Claude()} }

// claudeHost starts the executor server and returns the simulated Claude Code
// that serves it.
func (p *project) claudeHost() *claudecode.Host {
	return claudecode.NewHost(p.t, p.module(), p.tmp)
}
