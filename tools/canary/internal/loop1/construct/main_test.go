package construct_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/accept"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/construct"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

func runMain(args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := construct.Main(context.Background(), args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestConstructMain(t *testing.T) {
	t.Run("a bad flag or a stray argument is a usage error [verifies SP-2559~1]", func(t *testing.T) {
		for _, args := range [][]string{{"--nonsense"}, {"extra"}} {
			if code, _, _ := runMain(args...); code != accept.ExitUsage {
				t.Errorf("%v exited %d", args, code)
			}
		}
	})

	t.Run("a build with no container runner wired refuses to run [verifies SP-2559~1]", func(t *testing.T) {
		accept.Wire = nil
		if code, _, stderr := runMain("--repo", t.TempDir()); code != accept.ExitFailure || !strings.Contains(stderr, "no container runner") {
			t.Errorf("exit %d: %s", code, stderr)
		}
	})

	t.Run("a wiring that fails is a failure [verifies SP-2559~1]", func(t *testing.T) {
		accept.Wire = func(string, study.Layout) (accept.Wiring, error) { return accept.Wiring{}, accept.ErrNotWired }
		t.Cleanup(func() { accept.Wire = nil })
		if code, _, _ := runMain("--repo", t.TempDir()); code != accept.ExitFailure {
			t.Errorf("exit %d", code)
		}
	})
}
