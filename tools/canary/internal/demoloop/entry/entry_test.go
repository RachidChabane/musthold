package entry_test

import (
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/entry"
)

const token = "sk-ant-oat01-entry-test-token-0123456789"

func TestEntry(t *testing.T) {
	t.Run("the token is the first line of standard input, and an empty input is refused [verifies SP-2411~2]", func(t *testing.T) {
		if got, err := entry.ReadToken(strings.NewReader(token + "\nrest")); err != nil || got != token {
			t.Errorf("token %q, %v", got, err)
		}
		if _, err := entry.ReadToken(strings.NewReader("\n")); !errors.Is(err, entry.ErrNoToken) {
			t.Errorf("error = %v", err)
		}
	})

	t.Run("the command's environment holds the token once, whatever the entry's own held [verifies SP-2411~2]", func(t *testing.T) {
		env := entry.Environment([]string{"HOME=/home/demo", entry.TokenVariable + "=stale"}, token)
		if !slices.Equal(env, []string{"HOME=/home/demo", entry.TokenVariable + "=" + token}) {
			t.Errorf("env = %v", env)
		}
	})

	t.Run("the command gets the token in its environment and reads nothing on its standard input [verifies SP-2411~2]", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		script := `test "$` + entry.TokenVariable + `" = "` + token + `" && echo token; cat; echo end`
		code := entry.Run([]string{"/bin/sh", "-c", script}, strings.NewReader(token+"\nleftover input\n"), &stdout, &stderr)
		if code != 0 || stdout.String() != "token\nend\n" {
			t.Errorf("exit %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
		}
	})

	t.Run("no token starts nothing, and the command's exit code is the entry's [verifies SP-2411~2]", func(t *testing.T) {
		if code := entry.Run([]string{"/bin/sh", "-c", "echo started"}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}); code != entry.ExitNoToken {
			t.Errorf("exit %d", code)
		}
		if code := entry.Run([]string{"/bin/sh", "-c", "exit 7"}, strings.NewReader(token+"\n"), &bytes.Buffer{}, &bytes.Buffer{}); code != 7 {
			t.Errorf("exit %d", code)
		}
	})
}
