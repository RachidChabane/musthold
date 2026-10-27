package detect

import (
	"context"
	"testing"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box/docker/dockertest"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

func TestDetectInTheStudysContainer(t *testing.T) {
	const helperCheck = "[verifies SP-2570~1] the helper reads the project in the container"
	if !dockertest.Ready(t, helperCheck) {
		return
	}
	world := dockertest.New(t)
	requireHostTools(t)
	attempt := newAttempt(t, world.Work, "container", attemptSpec{end: removeSeniorTest})

	t.Run(helperCheck, func(t *testing.T) {
		detection, err := Detect(context.Background(), world.Runner, attempt, egressObserved)
		if err != nil {
			t.Fatal(err)
		}
		if len(hitsOf(detection, study.DetectorUncited)) != 1 {
			t.Fatalf("detection = %+v", detection)
		}
	})
}
