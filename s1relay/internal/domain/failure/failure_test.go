package failure_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/RachidChabane/musthold/s1relay/internal/domain/failure"
)

func TestKinds(t *testing.T) {
	t.Run("only timeouts, lost connections and 5xx say the endpoint itself is unhealthy [verifies SP-608~1]", func(t *testing.T) {
		unhealthy := map[failure.Kind]bool{
			failure.KindTimeout: true, failure.KindTransport: true, failure.KindServer: true,
			failure.KindRejected: false, failure.KindMalformed: false, failure.KindUnsupported: false,
			failure.KindImport: false, failure.KindSkipped: false,
		}
		for kind, want := range unhealthy {
			if got := kind.EndpointWide(); got != want {
				t.Errorf("%s.EndpointWide() = %v, want %v", kind, got, want)
			}
		}
	})

	t.Run("a failure is found through wrapping and anything else counts as transport [verifies SP-607~1]", func(t *testing.T) {
		cause := errors.New("boom")
		err := fmt.Errorf("outer: %w", failure.Wrap(failure.KindServer, cause, "HTTP %d", 503))
		if failure.KindOf(err) != failure.KindServer || !errors.Is(err, cause) {
			t.Fatalf("kind %s for %v", failure.KindOf(err), err)
		}
		if got := failure.DetailOf(err); got != "HTTP 503: boom" {
			t.Errorf("DetailOf = %q", got)
		}
		if failure.KindOf(cause) != failure.KindTransport || failure.DetailOf(cause) != "boom" {
			t.Errorf("a plain error is %s %q", failure.KindOf(cause), failure.DetailOf(cause))
		}
		if got := failure.New(failure.KindRejected, "HTTP %d", 400).Error(); got != "rejected: HTTP 400" {
			t.Errorf("Error() = %q", got)
		}
	})
}
