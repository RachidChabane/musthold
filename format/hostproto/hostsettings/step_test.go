package hostsettings_test

import (
	"errors"
	"testing"
	"time"

	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/hostsettings"
)

const verifiesStep = " [verifies SP-2666~1]"

func TestStepSeconds(t *testing.T) {
	t.Run("without host.step_seconds a step works 60 seconds"+verifiesStep, func(t *testing.T) {
		settings, err := hostsettings.Parse([]byte("host:\n  sampling: on\n"))
		if err != nil || settings.StepWork() != hostproto.DefaultStepWork || hostproto.DefaultStepWork != time.Minute {
			t.Fatalf("%+v, %v", settings, err)
		}
	})

	t.Run("a whole number of seconds from 1 to 75 sets the budget"+verifiesStep, func(t *testing.T) {
		for text, want := range map[string]time.Duration{"1": time.Second, "30": 30 * time.Second, "75": 75 * time.Second} {
			settings, err := hostsettings.Parse([]byte("host:\n  step_seconds: " + text + "\n"))
			if err != nil || settings.StepWork() != want || settings.IsDefault() {
				t.Errorf("%s: %+v, %v", text, settings, err)
			}
		}
	})

	t.Run("any other value is refused"+verifiesStep, func(t *testing.T) {
		for _, text := range []string{"0", "76", "-5", "1.5", "fast", "[30]"} {
			if _, err := hostsettings.Parse([]byte("host:\n  step_seconds: " + text + "\n")); !errors.Is(err, hostsettings.ErrInvalidSection) {
				t.Errorf("%s: %v", text, err)
			}
		}
	})
}
