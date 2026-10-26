package dispatch_test

import (
	"bytes"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/umbrella/internal/dispatch"
)

const verifiesDispatch = " [verifies SP-2121~1]"

// recorded is a process that records what it was asked to run and exits with
// a fixed code.
type recorded struct {
	path string
	args []string
	code int
	err  error
}

func (r *recorded) Run(path string, args []string, _ io.Reader, stdout, _ io.Writer) (int, error) {
	r.path, r.args = path, args
	_, _ = io.WriteString(stdout, "scanned\n")
	return r.code, r.err
}

type world struct {
	process        *recorded
	stdout, stderr bytes.Buffer
	beside         bool
	onPath         bool
}

func (w *world) env() dispatch.Env {
	return dispatch.Env{
		Stdin: strings.NewReader(""), Stdout: &w.stdout, Stderr: &w.stderr,
		Executable:   "/opt/musthold/bin/musthold",
		IsExecutable: func(path string) bool { return w.beside && path == "/opt/musthold/bin/musthold-dp" },
		LookPath: func(name string) (string, error) {
			if w.onPath {
				return "/usr/local/bin/" + name, nil
			}
			return "", errors.New("not found")
		},
		Process: w.process, Version: "0.1.0",
	}
}

func newWorld(code int) *world {
	return &world{process: &recorded{code: code}, beside: true, onPath: true}
}

func TestDispatch(t *testing.T) {
	t.Run("scan runs musthold-dp scan beside the dispatcher with the same arguments, streams and exit code"+verifiesDispatch, func(t *testing.T) {
		w := newWorld(0)
		code := dispatch.Run(w.env(), []string{"scan", "--format", "md", "--base", "main"})
		if code != 0 || w.process.path != "/opt/musthold/bin/musthold-dp" || !slices.Equal(w.process.args, []string{"scan", "--format", "md", "--base", "main"}) || w.stdout.String() != "scanned\n" {
			t.Fatalf("exit %d, ran %s %v, stdout %q", code, w.process.path, w.process.args, w.stdout.String())
		}
	})
	t.Run("the exit code of the module binary is the dispatcher's"+verifiesDispatch, func(t *testing.T) {
		w := newWorld(2)
		if code := dispatch.Run(w.env(), []string{"scan", "replay", ".musthold/scan/3f2a9c1"}); code != 2 {
			t.Fatalf("exit %d", code)
		}
	})
	t.Run("without a binary beside the dispatcher, the one on the PATH runs"+verifiesDispatch, func(t *testing.T) {
		w := newWorld(0)
		w.beside = false
		if code := dispatch.Run(w.env(), []string{"scan"}); code != 0 || w.process.path != "/usr/local/bin/musthold-dp" {
			t.Fatalf("exit %d, ran %s", code, w.process.path)
		}
	})
	t.Run("a missing module binary is a failure that names it"+verifiesDispatch, func(t *testing.T) {
		w := newWorld(0)
		w.beside, w.onPath = false, false
		code := dispatch.Run(w.env(), []string{"scan"})
		if code != dispatch.ExitNotInstalled || w.process.path != "" || !strings.Contains(w.stderr.String(), "musthold-dp is not installed") {
			t.Fatalf("exit %d, stderr %q", code, w.stderr.String())
		}
	})
	t.Run("help lists the commands on standard output, and help of a command asks its binary"+verifiesDispatch, func(t *testing.T) {
		w := newWorld(0)
		if code := dispatch.Run(w.env(), []string{"--help"}); code != 0 || !strings.HasPrefix(w.stdout.String(), "usage: musthold <scan|version|help>") {
			t.Fatalf("exit %d, stdout %q", code, w.stdout.String())
		}
		if code := dispatch.Run(w.env(), []string{"help", "scan"}); code != 0 || !slices.Equal(w.process.args, []string{"scan", "-h"}) {
			t.Fatalf("exit %d, args %v", code, w.process.args)
		}
	})
	t.Run("no command or an unknown one is a usage error on standard error"+verifiesDispatch, func(t *testing.T) {
		for _, args := range [][]string{nil, {"prove"}} {
			w := newWorld(0)
			if code := dispatch.Run(w.env(), args); code != 2 || w.stdout.Len() != 0 || !strings.Contains(w.stderr.String(), "usage: musthold <") || w.process.path != "" {
				t.Fatalf("%v: exit %d, stderr %q", args, code, w.stderr.String())
			}
		}
	})
	t.Run("version prints the dispatcher's version"+verifiesDispatch, func(t *testing.T) {
		for _, args := range [][]string{{"version"}, {"--version"}} {
			w := newWorld(0)
			if code := dispatch.Run(w.env(), args); code != 0 || w.stdout.String() != "musthold 0.1.0\n" {
				t.Fatalf("%v: exit %d, stdout %q", args, code, w.stdout.String())
			}
		}
	})
}
