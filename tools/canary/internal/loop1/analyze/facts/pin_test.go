package facts

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box/docker/dockertest"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/freeze"
)

// The files of the pinned Shallnot that fix the give-up of its stop hook.
const (
	shallnotSourceVariable = "SHALLNOT_SRC"
	shallnotSourceScript   = "script/shallnot-src"
	giveUpMessageSource    = "internal/adapters/agenthook/message.go"
	stopHookSource         = "internal/adapters/agenthook/stop.go"
)

var (
	noticeExit   = regexp.MustCompile(`exitNotice\s*=\s*([0-9]+)`)
	giveUpExits  = regexp.MustCompile(`case DecisionGiveUp:\s*return Response\{[^}]*ExitCode: exitNotice\}`)
	giveUpFormat = regexp.MustCompile(`"([^"%]*) %d attempts`)
)

// pinnedShallnot reads a file of the pinned Shallnot's source, or skips the test
// when the source repository is not beside this one.
func pinnedShallnot(t *testing.T, file string) string {
	t.Helper()
	repo := dockertest.RepoRoot(t)
	pin, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(freeze.ShallnotPinFile)))
	if err != nil {
		t.Fatal(err)
	}
	source := os.Getenv(shallnotSourceVariable)
	if source == "" {
		found, err := exec.Command(filepath.Join(repo, filepath.FromSlash(shallnotSourceScript))).Output()
		if err != nil {
			t.Skipf("the Shallnot source cannot be found: %v", err)
		}
		source = strings.TrimSpace(string(found))
	}
	content, err := exec.Command("git", "-C", source, "show", strings.TrimSpace(string(pin))+":"+file).Output()
	if err != nil {
		t.Skipf("the pinned Shallnot is not in %s: %v", source, err)
	}
	return string(content)
}

func TestGiveUpOfThePinnedShallnot(t *testing.T) {
	t.Run("a give-up is the exit code and the message prefix of the pinned Shallnot's stop hook [verifies SP-2648~1]", func(t *testing.T) {
		message := giveUpFormat.FindStringSubmatch(pinnedShallnot(t, giveUpMessageSource))
		if message == nil || message[1] != GiveUpMessage {
			t.Errorf("the pinned Shallnot's give-up message begins %q, the analysis reads %q", message, GiveUpMessage)
		}
		stop := pinnedShallnot(t, stopHookSource)
		exit := noticeExit.FindStringSubmatch(stop)
		if exit == nil || exit[1] != strconv.Itoa(exitGiveUp) || !giveUpExits.MatchString(stop) {
			t.Errorf("the pinned Shallnot gives up with exit %v, the analysis reads %d", exit, exitGiveUp)
		}
	})
}
