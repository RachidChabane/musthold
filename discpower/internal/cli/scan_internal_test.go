package cli

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/scan"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/scanscope"
)

func TestDepthRequest(t *testing.T) {
	t.Run("a scan is quick unless --full is given [verifies SP-2119~2]", func(t *testing.T) {
		quick := depthRequest(false, nil, scan.Preset{})
		if quick.Depth != scan.DepthQuick || quick.SampleSize != scan.QuickSampleSize || quick.Mutants != scan.QuickMutants ||
			quick.Budget != scan.QuickBudget || quick.Coverage != scan.CoverSampled {
			t.Fatalf("quick %+v", quick)
		}
		full := depthRequest(true, nil, scan.Preset{})
		if full.Depth != scan.DepthFull || full.SampleSize != scanscope.DefaultSampleSize || full.Mutants != scanscope.DefaultMutants ||
			full.Budget != 0 || full.Coverage != scan.CoverGreen {
			t.Fatalf("full %+v", full)
		}
	})
	t.Run("--sample, --mutants and --budget replace what the depth sets, even with zero [verifies SP-2119~2]", func(t *testing.T) {
		given := map[string]bool{flagSample: true, flagMutants: true, flagBudget: true}
		request := depthRequest(false, given, scan.Preset{SampleSize: 3, Mutants: 9})
		if request.SampleSize != 3 || request.Mutants != 9 || request.Budget != 0 || request.Depth != scan.DepthQuick {
			t.Fatalf("request %+v", request)
		}
	})
}

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
