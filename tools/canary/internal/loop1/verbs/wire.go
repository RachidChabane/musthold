package verbs

import (
	"context"
	"errors"
	"io"
	"os"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/adapters"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/accept"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/phase"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box/docker"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/canary"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/detect"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/freeze"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/oracle"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/runs"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// ErrNoImage refuses a verb whose containers have no image: neither the inputs
// freeze nor the product's manifest names one.
var ErrNoImage = errors.New("no image: neither the inputs freeze nor the product's manifest names the study's image (build the product)")

var (
	_ accept.Equipper     = runs.ControlEquipper{}
	_ freeze.Preparer     = runs.ResolvePreparer{}
	_ freeze.CanarySource = canary.Latest{}
	_ oracle.Phase        = phase.Reader{}
	_ detect.Phase        = phase.Reader{}
	_ canary.Detectors    = c3Detectors{}
)

// setupFlags are the flags of the run verb a wiring reads the study with.
type setupFlags struct {
	Repo, Config, Work, Product string
}

// setup reads the configuration, the inputs freeze beside it and the product's
// manifest as the run verb does; inputsRequired refuses an absent inputs freeze.
func setup(f setupFlags, inputsRequired bool) (runs.Setup, error) {
	args := []string{"--repo", f.Repo, "--config", f.Config, "--work", f.Work}
	if f.Product != "" {
		args = append(args, "--product", f.Product)
	}
	flags, err := runs.ParseFlags("wire", args, os.Environ(), io.Discard)
	if err != nil {
		return runs.Setup{}, err
	}
	return flags.Resolve(adapters.OSFiles{}, !inputsRequired)
}

// studyBox is the study's docker runner over the setup's product and image.
func studyBox(s runs.Setup) (*docker.Runner, error) {
	if s.Options.Image == "" {
		return nil, ErrNoImage
	}
	return docker.StudyRunner(s.Options.RepoRoot, s.Options.Product, s.Options.Image)
}

// phaseOf is the run-phase rule over the work directory and the frozen order.
func phaseOf(s runs.Setup) phase.Reader {
	return phase.Reader{Layout: s.Layout, Order: s.Inputs.Order, Now: time.Now}
}

// wireAccept gives accept and construct the study's box and the equipping of the
// control arm as a run does it, before or after the inputs freeze.
func wireAccept(f accept.Flags) (accept.Wiring, error) {
	s, err := setup(setupFlags{Repo: f.Repo, Config: f.Config, Work: f.Work}, false)
	if err != nil {
		return accept.Wiring{}, err
	}
	runner, err := studyBox(s)
	if err != nil {
		return accept.Wiring{}, err
	}
	equipper := runs.ControlEquipper{Options: s.Options, Box: runner, Files: adapters.OSFiles{}, Tree: runs.OSTree{}}
	return accept.Wiring{Runner: runner, Equipper: equipper}, nil
}

// wireResolve gives resolve the preparation of a task as a run does it, through
// the box resolve built, and the canary's last output.
func wireResolve(o freeze.Options, runner box.Runner) (freeze.ResolveWiring, error) {
	s, err := setup(setupFlags{Repo: o.Repo, Config: o.Config, Work: o.Work, Product: o.Product}, false)
	if err != nil {
		return freeze.ResolveWiring{}, err
	}
	files := adapters.OSFiles{}
	return freeze.ResolveWiring{
		Preparer: runs.ResolvePreparer{Options: s.Options, Root: s.Layout.Root, Box: runner, Files: files, Tree: runs.OSTree{}},
		Canary:   canary.Latest{Files: files, Layout: s.Layout},
	}, nil
}

// wireOracle gives the oracle the box of the frozen image and the run-phase rule.
func wireOracle(o oracle.Options) (oracle.Wiring, error) {
	s, err := setup(setupFlags{Repo: o.Repo, Config: o.Config, Work: o.Work}, true)
	if err != nil {
		return oracle.Wiring{}, err
	}
	runner, err := studyBox(s)
	if err != nil {
		return oracle.Wiring{}, err
	}
	return oracle.Wiring{Runner: runner, Phase: phaseOf(s)}, nil
}

// wireDetect gives detect the box of the frozen image, the run-phase rule and the
// hosts the canary observed.
func wireDetect(f detect.Flags) (detect.Dependencies, error) {
	s, err := setup(setupFlags{Repo: f.Repo, Config: f.Config, Work: f.Work}, true)
	if err != nil {
		return detect.Dependencies{}, err
	}
	runner, err := studyBox(s)
	if err != nil {
		return detect.Dependencies{}, err
	}
	return detect.Dependencies{Runner: runner, Phase: phaseOf(s), EgressObserved: s.Inputs.EgressObserved}, nil
}

// canaryDetectors are the detectors C3 runs, in the canary's image.
func canaryDetectors(s runs.Setup) (canary.Detectors, error) {
	runner, err := studyBox(s)
	if err != nil {
		return nil, err
	}
	return c3Detectors{held: oracle.NewVerb(s.Options.RepoRoot, s.Layout, runner, nil, io.Discard), runner: runner}, nil
}

// c3Detectors measure the equipped and final tests of C3's attempt against the
// task's held-out violations, which K3 reads, then run every detector on it.
type c3Detectors struct {
	held   *oracle.Verb
	runner box.Runner
}

// Detect implements canary.Detectors.
func (d c3Detectors) Detect(ctx context.Context, runDir string, egressObserved []string) (study.Detection, error) {
	run, err := phase.ReadRun(phase.Records(runDir))
	if err != nil {
		return study.Detection{}, err
	}
	if err := d.held.MeasureEquippedAndFinal(ctx, runDir, run); err != nil {
		return study.Detection{}, err
	}
	return detect.Detect(ctx, d.runner, runDir, egressObserved)
}
