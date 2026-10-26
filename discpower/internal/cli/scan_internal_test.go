package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/adapters/junitxml"
	"github.com/RachidChabane/musthold/discpower/internal/adapters/process"
	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	"github.com/RachidChabane/musthold/discpower/internal/app/scan"
	domain "github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/scanscope"
	"github.com/RachidChabane/musthold/format/hostproto"
)

func TestDepthRequest(t *testing.T) {
	t.Run("a scan is quick unless --full is given [verifies SP-2119~4]", func(t *testing.T) {
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
	t.Run("--sample, --mutants and --budget replace what the depth sets, even with zero [verifies SP-2119~4]", func(t *testing.T) {
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

func TestWitnessRecorder(t *testing.T) {
	strategies, err := newStrategies(process.New(), junitxml.NewReader())
	if err != nil {
		t.Fatal(err)
	}
	strategyOf := func(t *testing.T, fixture string) ports.Ecosystem {
		t.Helper()
		candidates, err := scanCandidates(strategies, fixture)
		if err != nil {
			t.Fatal(err)
		}
		return candidates[0].strategy
	}
	goStrategy, pytestStrategy := strategyOf(t, goScanFixture), strategyOf(t, pytestScanFixture)
	t.Run("a Go module records the coverage of its witnesses running them alone [verifies SP-2271~1]", func(t *testing.T) {
		got, want := witnessRecorder(goStrategy), goStrategy.(ports.WitnessCovering).WitnessCoverage()
		if got == nil || fmt.Sprintf("%T", got) != fmt.Sprintf("%T", want) {
			t.Fatalf("recorder %T, want %T", got, want)
		}
	})
	t.Run("a pytest project records the coverage of its witnesses with the whole suite's recorder [verifies SP-2271~1]", func(t *testing.T) {
		got, want := witnessRecorder(pytestStrategy), pytestStrategy.(ports.Covering).Coverage()
		if got == nil || fmt.Sprintf("%T", got) != fmt.Sprintf("%T", want) {
			t.Fatalf("recorder %T, want %T", got, want)
		}
	})
	t.Run("a Go module runs the witnesses of a scan with its own runner, a pytest project with its suite runner [verifies SP-2273~1]", func(t *testing.T) {
		for strategy, own := range map[ports.Ecosystem]bool{goStrategy: true, pytestStrategy: false} {
			var deps scan.Deps
			wireWitnesses(&deps, strategy, os.DirFS(t.TempDir()))
			witnesses, wired := deps.Witnesses.(scan.ExecutionWitnesses)
			if !wired || (witnesses.Runner != nil) != own {
				t.Errorf("%s: witnesses %+v", strategy.Name(), deps.Witnesses)
			}
		}
	})
	t.Run("a Jest or a Vitest project is scanned by its runner and records the coverage of its witnesses running them alone [verifies SP-2296~1]", func(t *testing.T) {
		for fixture, name := range map[string]domain.Name{jestScanFixture: domain.Jest, vitestScanFixture: domain.Vitest} {
			strategy := strategyOf(t, fixture)
			got, want := witnessRecorder(strategy), strategy.(ports.WitnessCovering).WitnessCoverage()
			if strategy.Name() != name || got == nil || fmt.Sprintf("%T", got) != fmt.Sprintf("%T", want) {
				t.Errorf("%s: %s, recorder %T, want %T", fixture, strategy.Name(), got, want)
			}
		}
	})
	t.Run("prove records coverage only through the whole suite's recorder, which a Go module does not offer [verifies SP-2271~1]", func(t *testing.T) {
		if _, covering := goStrategy.(ports.Covering); covering {
			t.Fatal("the Go strategy records the whole suite's coverage, which prove would use")
		}
	})
}

func TestScanCandidates(t *testing.T) {
	strategies, err := newStrategies(process.New(), junitxml.NewReader())
	if err != nil {
		t.Fatal(err)
	}
	polyglot := t.TempDir()
	if err := os.WriteFile(filepath.Join(polyglot, "pyproject.toml"), []byte("[tool.pytest.ini_options]\ntestpaths = [\"tooling\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(polyglot, "vitest.config.ts"), []byte("export default {};\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	candidates, err := scanCandidates(strategies, polyglot)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("a tree that declares several test tools offers each of them, in registration order [verifies SP-2119~4]", func(t *testing.T) {
		var names []domain.Name
		for _, candidate := range candidates {
			names = append(names, candidate.strategy.Name())
		}
		if want := []domain.Name{domain.Pytest, domain.Vitest}; !slices.Equal(names, want) {
			t.Fatalf("candidates %v, want %v", names, want)
		}
	})
	t.Run("a replay runs the ecosystem its finding names, the first one when it names none [verifies SP-2119~4]", func(t *testing.T) {
		tree := scanTree{candidates: candidates}
		if got := tree.candidateRunning(string(domain.Vitest)).strategy.Name(); got != domain.Vitest {
			t.Fatalf("runs %s", got)
		}
		if got := tree.candidateRunning("").strategy.Name(); got != domain.Pytest {
			t.Fatalf("runs %s", got)
		}
	})
	t.Run("a tree no scan ecosystem recognizes has no candidate [verifies SP-2119~4]", func(t *testing.T) {
		if _, err := scanCandidates(strategies, t.TempDir()); !errors.Is(err, errScanEcosystem) {
			t.Fatalf("error %v", err)
		}
	})
}

// Scan fixtures whose ecosystem the scan detects.
const (
	goScanFixture     = "../../fixtures/scan/go"
	pytestScanFixture = "../../fixtures/scan/pytest"
	jestScanFixture   = "../../fixtures/scan/jest"
	vitestScanFixture = "../../fixtures/scan/vitest"
)

func TestScanStrategyRefusal(t *testing.T) {
	strategies, err := newStrategies(process.New(), junitxml.NewReader())
	if err != nil {
		t.Fatal(err)
	}
	t.Run("the root of a Go workspace is refused with its modules and --root [verifies SP-2425~1]", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(root+"/go.work", []byte("go 1.26\n\nuse (\n\t./format\n\t./discpower\n)\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := scanCandidates(strategies, root)
		if !errors.Is(err, errScanEcosystem) || !strings.Contains(err.Error(), "--root <module>: format, discpower") {
			t.Fatalf("refusal %v", err)
		}
	})
	t.Run("a tree with no suite and no workspace is refused as before [verifies SP-2425~1]", func(t *testing.T) {
		if _, err := scanCandidates(strategies, t.TempDir()); !errors.Is(err, errScanEcosystem) || err.Error() != errScanEcosystem.Error() {
			t.Fatalf("refusal %v", err)
		}
	})
}
