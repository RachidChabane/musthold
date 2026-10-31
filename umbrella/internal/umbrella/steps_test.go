package umbrella_test

import (
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/format/hostproto/pluginfiles"
)

const verifiesBoundedSteps = " [verifies SP-2690~1]"

func TestProveAllBoundedSteps(t *testing.T) {
	_, body := skillText(t)

	t.Run("prove-all drives every module in bounded foreground steps, continues unfinished ones and renews an abandonment once"+verifiesBoundedSteps, func(t *testing.T) {
		for _, want := range []string{
			"`<binary> prove --changed` once, in the foreground", "within 90 seconds", "`<binary> request list --session prove`",
			"printed an `unfinished` line and lists no ticket only runs `<binary> prove --resume`", "`in_flight <n> tickets`",
			"run that module's `<binary> prove --changed` once more", "at a second `host_unresponsive` finding",
			"`" + pluginfiles.VerifyLocalCommand + "`",
		} {
			if !strings.Contains(body, want) {
				t.Errorf("the skill lacks %q", want)
			}
		}
		if strings.Contains(body, "or a `host_unresponsive` finding, report it") {
			t.Error("the skill still stops a module at its first host_unresponsive finding")
		}
	})

	t.Run("the scan skill continues a scan whose step ended unfinished"+verifiesBoundedSteps, func(t *testing.T) {
		_, scanBody := scanSkillText(t)
		if !strings.Contains(scanBody, "starts with `unfinished`") || !strings.Contains(scanBody, "run step 1 again") {
			t.Errorf("scan skill:\n%s", scanBody)
		}
	})
}
