package cli

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
)

func TestScanFailure(t *testing.T) {
	t.Run("a coverage run past its bound fails the scan with an error that names the bound and --coverage-timeout [verifies SP-2127~1]", func(t *testing.T) {
		cause := fmt.Errorf("record the line coverage, each run bounded by 2m0s: %w", ports.ErrCoverageTimedOut)
		err := scanFailure(cause)
		if !errors.Is(err, ports.ErrCoverageTimedOut) || !strings.Contains(err.Error(), "2m0s") || !strings.Contains(err.Error(), "--"+flagCoverageTimeout) {
			t.Fatalf("error %v", err)
		}
	})
	t.Run("any other failure of a scan is reported as it is [verifies SP-2127~1]", func(t *testing.T) {
		cause := errors.New("the reference does not build")
		if err := scanFailure(cause); err != cause {
			t.Fatalf("error %v", err)
		}
	})
}
