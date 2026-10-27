package construct_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/accept"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/construct"
)

var errWiring = errors.New("no engine")

func runMain(args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	failing := construct.Command{Wire: func(accept.Flags) (accept.Wiring, error) { return accept.Wiring{}, errWiring }}
	code := failing.Main(context.Background(), args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestConstructMain(t *testing.T) {
	t.Run("a bad flag or a stray argument is a usage error [verifies SP-2559~2]", func(t *testing.T) {
		for _, args := range [][]string{{"--nonsense"}, {"extra"}} {
			if code, _, _ := runMain(args...); code != accept.ExitUsage {
				t.Errorf("%v exited %d", args, code)
			}
		}
	})

	t.Run("a wiring that fails is a failure [verifies SP-2559~2]", func(t *testing.T) {
		if code, _, stderr := runMain("--repo", t.TempDir()); code != accept.ExitFailure || !strings.Contains(stderr, errWiring.Error()) {
			t.Errorf("exit %d: %s", code, stderr)
		}
	})
}
