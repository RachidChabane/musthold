// Package session is the citation call of the loop-2 study as a construction
// model: `claude -p` for claude-opus-5-5 without tools, in a session container
// of the study's image under loop-1's live-session isolation. The container has
// a scratch HOME on tmpfs, a read-only root file system, no docker socket and no
// SSH agent, and sits on the run's internal network, whose only route out is the
// logging proxy, which forwards only to the model endpoint. The token reaches
// the session's entry on its standard input, followed by the prompt; it reaches
// no argument, file, environment the engine records, or record.
package session

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	canary "github.com/RachidChabane/musthold/tools/canary/internal/app"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/app"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/container"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/record"
	"github.com/RachidChabane/musthold/tools/canary/internal/domain/evidence"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/construct"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/freeze"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/cite"
)

// CitationModel is the model of the citation call.
const CitationModel = "claude-opus-5-5"

// Files a call leaves in its directory, beside the proxy's log.
const (
	StdoutFile = "stdout.json"
	StderrFile = "stderr.txt"
	EgressFile = "egress.jsonl"
	callPrefix = "cite-"
	dirMode    = 0o755
	// setupTimeout bounds each docker command around the session.
	setupTimeout = 2 * time.Minute
	lineBreak    = "\n"
)

// ErrTokenInOutput is returned when the session printed its token; the output
// is kept with the token redacted.
var ErrTokenInOutput = errors.New("the session printed its token")

// ClaudeArgv is the session's command: one print-mode turn of the citation
// model without tools, settings, MCP servers, slash commands or a kept session,
// answering as the schema says, its prompt read from standard input.
func ClaudeArgv(schema []byte) []string {
	return []string{
		freeze.ClaudeProgram, "-p",
		"--model", CitationModel,
		"--tools", "",
		"--setting-sources", "",
		"--strict-mcp-config",
		"--disable-slash-commands",
		"--no-session-persistence",
		"--output-format", "json",
		"--json-schema", string(schema),
	}
}

// Claude is the construction model of the citation call.
type Claude struct {
	// Docker is the docker client command that names the study's engine.
	Docker []string
	// Image is the study's image; Product the host's product directory.
	Image, Product string
	UID, GID       int
	// Calls is the directory each call's directory is made in, under the work
	// directory the engine mounts.
	Calls string
	// Processes runs the docker commands around the session, and Sessions the
	// session itself.
	Processes canary.Processes
	Sessions  canary.Sessions
	// Box runs `claude --version` in an offline container of the image.
	Box box.Runner
	// Credentials are the token's sources, in order of preference.
	Credentials []canary.ClaudeCredential
	// Env is the docker client's environment, without any token.
	Env []string
	Now func() time.Time
	// Timeout bounds the session; zero means construct.CallTimeout.
	Timeout time.Duration
}

var _ construct.Model = Claude{}

func (c Claude) timeout() time.Duration {
	if c.Timeout > 0 {
		return c.Timeout
	}
	return construct.CallTimeout
}

// Complete runs the citation session for one request and reads its answer as
// the file citations.json.
func (c Claude) Complete(ctx context.Context, request construct.Request) (construct.Reply, error) {
	var reply construct.Reply
	if err := os.MkdirAll(c.Calls, dirMode); err != nil {
		return reply, err
	}
	dir, err := os.MkdirTemp(c.Calls, callPrefix+request.Domain+"-")
	if err != nil {
		return reply, err
	}
	if reply.CLIVersion, err = c.version(ctx, dir); err != nil {
		return reply, fmt.Errorf("%w: %w", err, &construct.HarnessFailure{Kind: construct.HarnessUnavailable, Detail: err.Error()})
	}
	source, err := canary.FirstCredential(ctx, c.Credentials)
	if err != nil {
		return reply, &construct.HarnessFailure{Kind: construct.HarnessAuthentication, Detail: err.Error()}
	}
	token, err := source.Token(ctx)
	if err != nil {
		return reply, &construct.HarnessFailure{Kind: construct.HarnessAuthentication, Detail: "the " + source.Source() + " source gave no token"}
	}
	b := c.box(dir)
	defer c.teardown(ctx, b)
	for _, argv := range c.setup(b) {
		if err := c.docker(ctx, argv); err != nil {
			return reply, &construct.HarnessFailure{Kind: construct.HarnessUnavailable, Detail: err.Error()}
		}
	}
	reply.Start = c.Now().UTC()
	outcome, runErr := c.Sessions.Run(c.Spec(b, request, token))
	reply.End = c.Now().UTC()
	return c.read(dir, token, outcome, runErr, reply)
}

// box is the containers of one call.
func (c Claude) box(dir string) container.Box {
	return container.Box{
		Image: c.Image, RunDir: dir, Product: c.Product, UID: c.UID, GID: c.GID,
		Run: filepath.Base(dir), ForwardStdin: true,
	}
}

// setup creates the call's internal network and starts the logging proxy on it.
func (c Claude) setup(b container.Box) [][]string {
	return [][]string{b.NetworkArgv(), b.ProxyArgv(filepath.Join(b.RunDir, EgressFile)), b.ConnectArgv()}
}

// Variables are the session's whole environment, which `env -i` sets: the
// scratch HOME, the image's PATH, a locale, the proxy, and no call but the
// model's.
func Variables() []string {
	base := []string{"HOME=" + container.ScratchHome, "PATH=" + container.SystemPath, "LANG=C.UTF-8"}
	return append(append(base, container.ProxyVariables()...), app.NonessentialTraffic)
}

// Spec is the session of one call: the docker command of its container, with
// the token's line and then the prompt on its standard input.
func (c Claude) Spec(b container.Box, request construct.Request, token string) canary.SessionSpec {
	argv := append(append([]string{}, c.Docker...), b.SessionArgv(b.RunDir, Variables(), ClaudeArgv(request.Schema.JSON))...)
	return canary.SessionSpec{
		Argv: argv, Dir: b.RunDir, Env: c.Env,
		Stdin:      []byte(token + lineBreak + request.Prompt),
		StdoutPath: filepath.Join(b.RunDir, StdoutFile), StderrPath: filepath.Join(b.RunDir, StderrFile),
		Counts:  func([]byte) bool { return false },
		Timeout: c.timeout(),
	}
}

// read classes a finished session by loop-1's harness rules and reads its
// answer, with the token redacted from what the reply keeps.
func (c Claude) read(dir, token string, outcome canary.SessionOutcome, runErr error, reply construct.Reply) (construct.Reply, error) {
	stdout, _ := os.ReadFile(filepath.Join(dir, StdoutFile))
	stderr, _ := os.ReadFile(filepath.Join(dir, StderrFile))
	stdout, inStdout := record.Redact(stdout, token)
	stderr, inStderr := record.Redact(stderr, token)
	reply.Output, reply.Stderr, reply.Exit = stdout, stderr, outcome.ExitCode
	switch {
	case runErr != nil:
		return reply, &construct.HarnessFailure{Kind: construct.HarnessUnavailable, Detail: runErr.Error()}
	case outcome.Killed == evidence.KilledTimeout:
		return reply, &construct.HarnessFailure{Kind: construct.HarnessTimeout, Detail: fmt.Sprintf("no answer within %s", c.timeout())}
	case outcome.Killed != "":
		return reply, &construct.HarnessFailure{Kind: construct.HarnessUnavailable, Detail: "the session was ended: " + outcome.Killed}
	}
	if failure := construct.ClassifyOutcome(construct.ProcessResult{Stdout: stdout, Stderr: stderr, ExitCode: outcome.ExitCode}, stdout); failure != nil {
		return reply, failure
	}
	if inStdout+inStderr > 0 {
		return reply, ErrTokenInOutput
	}
	if outcome.ExitCode != 0 {
		return reply, fmt.Errorf("%w: exit %d", construct.ErrCodexExit, outcome.ExitCode)
	}
	files, err := cite.AnswerFiles(stdout)
	reply.Files = files
	return reply, err
}

// version reads the CLI's version in an offline container of the image.
func (c Claude) version(ctx context.Context, dir string) (string, error) {
	result, err := c.Box.Run(ctx, box.Command{
		Argv: []string{freeze.ClaudeProgram, "--version"}, Env: []string{box.SystemPath, "HOME=" + dir},
		Dir: dir, Mounts: []box.Mount{{Path: dir}}, Limit: setupTimeout,
	})
	if err != nil {
		return "", err
	}
	if result.ExitCode != 0 {
		return "", fmt.Errorf("%s --version exited %d", freeze.ClaudeProgram, result.ExitCode)
	}
	return strings.TrimSpace(string(result.Stdout)), nil
}

// teardown removes the call's containers and network, whatever happened.
func (c Claude) teardown(ctx context.Context, b container.Box) {
	_ = c.docker(context.WithoutCancel(ctx), b.RemoveArgv())
	_ = c.docker(context.WithoutCancel(ctx), b.NetworkRemoveArgv())
}

// docker runs one docker command against the engine.
func (c Claude) docker(ctx context.Context, argv []string) error {
	output, err := c.Processes.Run(ctx, canary.CommandSpec{Argv: append(append([]string{}, c.Docker...), argv...), Env: c.Env, Timeout: setupTimeout})
	if err != nil {
		return err
	}
	if output.ExitCode != 0 {
		return fmt.Errorf("docker %s exited %d: %s", strings.Join(argv[:min(2, len(argv))], " "), output.ExitCode, strings.TrimSpace(output.Stderr))
	}
	return nil
}
