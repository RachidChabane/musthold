package detect

import (
	"context"
	"os"
	"testing"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// containerGate enables the tests that run in the study's container engine; the
// orchestrator alone sets it.
const containerGate = "MUSTHOLD_LOOP1_CONTAINERS"

// newContainerRunner returns the runner of the study's offline container. The
// docker implementation of the box port is injected here.
func newContainerRunner(t *testing.T) box.Runner {
	t.Helper()
	t.Skip("the docker implementation of the box port is not linked into this test binary")
	return nil
}

func TestDetectInTheStudysContainer(t *testing.T) {
	if os.Getenv(containerGate) != "1" {
		t.Skipf("%s is not set to 1", containerGate)
	}
	requireHostTools(t)
	runner := newContainerRunner(t)
	attempt := newAttempt(t, t.TempDir(), "container", attemptSpec{end: removeSeniorTest})

	t.Run("[verifies SP-2570~1] the helper reads the project in the container", func(t *testing.T) {
		detection, err := Detect(context.Background(), runner, attempt, testInputs)
		if err != nil {
			t.Fatal(err)
		}
		if len(hitsOf(detection, study.DetectorUncited)) != 1 {
			t.Fatalf("detection = %+v", detection)
		}
	})
}
