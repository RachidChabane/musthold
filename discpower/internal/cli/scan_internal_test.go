package cli

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/scan"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/scanscope"
	"github.com/RachidChabane/spec-proof/format/hostproto"
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

const verifiesHostCommandLine = " [verifies SP-2244~1]"

func TestPullRequestArgument(t *testing.T) {
	t.Run("a scan takes no positional argument, or one pull request number"+verifiesHostCommandLine, func(t *testing.T) {
		cases := []struct {
			args []string
			want int
			fail bool
		}{
			{nil, 0, false},
			{[]string{"42"}, 42, false},
			{[]string{"0"}, 0, true},
			{[]string{"-3"}, 0, true},
			{[]string{"main"}, 0, true},
			{[]string{"1", "2"}, 0, true},
		}
		for _, c := range cases {
			got, err := pullRequestOf(c.args)
			if (err != nil) != c.fail || got != c.want {
				t.Fatalf("%v gives %d, %v", c.args, got, err)
			}
			if err != nil && !errors.Is(err, errUnexpectedParams) {
				t.Fatalf("%v: %v is no usage error", c.args, err)
			}
		}
	})
}

func TestPrintAwaiting(t *testing.T) {
	t.Run("a scan that waits prints its wave, its session and how many tickets wait, as prove does [verifies SP-2245~1]", func(t *testing.T) {
		var out strings.Builder
		session := hostproto.ScanSession("abc")
		printAwaiting(&out, scan.HostResult{Session: session, Wave: 2, Pending: make([]hostproto.Ticket, 3)})
		if want := "wave 2\nsession scan:abc\nawaiting_host 3 tickets\n"; out.String() != want {
			t.Fatalf("printed %q", out.String())
		}
		if (scan.HostResult{}).ExitCode() != scan.ExitAwaitingHost || (scan.HostResult{Finished: true}).ExitCode() != ExitClean {
			t.Fatal("exit codes")
		}
	})
}
