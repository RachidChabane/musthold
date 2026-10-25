// Package entry is the first process of a session's container: it reads the
// model token from its standard input, starts the session's command with the
// token in that command's environment only and its standard input closed, and
// forwards the signals it receives. The token reaches no file, no argument and no
// environment the engine records.
package entry

import (
	"bufio"
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

// ReadToken reads the first line of the input, without its line break.
func ReadToken(input io.Reader) (string, error) {
	reader := bufio.NewReaderSize(io.LimitReader(input, maxTokenSize), maxTokenSize)
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	token := strings.TrimRight(line, "\r\n")
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

// Run reads the token, starts the command and returns its exit code.
func Run(argv []string, stdin io.Reader, stdout, stderr io.Writer) int {
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
