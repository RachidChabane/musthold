// Package entry is the first process of a session's container: it reads the
// model token from its standard input, starts the session's command with the
// token in that command's environment only and its standard input closed, or,
// when asked, holding the rest of the entry's standard input, and forwards the
// signals it receives. The token reaches no file, no argument and no
// environment the engine records.
package entry

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
)

// TokenVariable is the variable the session's command reads its token from.
const TokenVariable = "CLAUDE_CODE_OAUTH_TOKEN"

// Exit codes of the entry itself.
const (
	ExitNoToken  = 70
	ExitNoStart  = 71
	maxTokenSize = 4096
)

// ErrNoToken is returned when standard input holds no token line.
var ErrNoToken = errors.New("standard input holds no token")

// ReadToken reads the first line of the input, without its line break, and
// nothing after it.
func ReadToken(input io.Reader) (string, error) {
	var line []byte
	octet := make([]byte, 1)
	for len(line) < maxTokenSize {
		n, err := input.Read(octet)
		if n == 1 {
			if octet[0] == '\n' {
				break
			}
			line = append(line, octet[0])
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", err
		}
	}
	token := strings.TrimRight(string(line), "\r")
	if token == "" {
		return "", ErrNoToken
	}
	return token, nil
}

// Environment is the command's environment: the entry's own, without any token
// it might carry, and the token.
func Environment(own []string, token string) []string {
	kept := make([]string, 0, len(own)+1)
	for _, pair := range own {
		if name, _, _ := strings.Cut(pair, "="); name != TokenVariable {
			kept = append(kept, pair)
		}
	}
	return append(kept, TokenVariable+"="+token)
}

// Options say what the command receives beside the token.
type Options struct {
	// ForwardStdin gives the command the rest of the entry's standard input,
	// after the token's line; otherwise its standard input is closed.
	ForwardStdin bool
}

// Run reads the token, starts the command with its standard input closed and
// returns its exit code.
func Run(argv []string, stdin io.Reader, stdout, stderr io.Writer) int {
	return RunWith(Options{}, argv, stdin, stdout, stderr)
}

// RunWith reads the token, starts the command as the options say and returns
// its exit code.
func RunWith(options Options, argv []string, stdin io.Reader, stdout, stderr io.Writer) int {
	token, err := ReadToken(stdin)
	if err != nil {
		fmt.Fprintln(stderr, "entry:", err)
		return ExitNoToken
	}
	if len(argv) == 0 {
		fmt.Fprintln(stderr, "entry: no command")
		return ExitNoStart
	}
	command := exec.Command(argv[0], argv[1:]...)
	command.Env = Environment(os.Environ(), token)
	command.Stdin, command.Stdout, command.Stderr = nil, stdout, stderr
	if options.ForwardStdin {
		command.Stdin = stdin
	}
	if err := command.Start(); err != nil {
		fmt.Fprintln(stderr, "entry: the command could not start:", err)
		return ExitNoStart
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	go func() {
		for received := range signals {
			_ = command.Process.Signal(received)
		}
	}()
	err = command.Wait()
	signal.Stop(signals)
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	if err != nil {
		return ExitNoStart
	}
	return 0
}
