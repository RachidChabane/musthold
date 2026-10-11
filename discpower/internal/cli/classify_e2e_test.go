package cli_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/hoststore"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/xdg"
	"github.com/RachidChabane/spec-proof/discpower/internal/cli"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

// driveClassifying runs the proof as a host would whose mod classifies every
// closed request it meets with one label, and whose deciders answer the rest.
func (h *harness) driveClassifying(label string, answers hostprototest.AnswerSource) (code int, imported []hostprototest.ImportedClassification) {
	h.t.Helper()
	args := []string{proveCommand, allFlag}
	code, _, stderr := h.run(nil, args...)
	for wave := 0; code == cli.ExitAwaitingHost && wave < h.waveLimit(); wave++ {
		imported = append(imported, hostprototest.ClassifyMod{Run: h.run, Model: hostprototest.ClassifySessionModel, Log: h.rawLog}.Play(h.t,
			func(hostproto.ClassifyPlanItem, string) string { return label })...)
		if _, err := hostprototest.PlayCLI(h, hostprototest.PersonaHonest, answers, func(time.Duration) {}); err != nil {
			h.t.Fatalf("the honest persona failed: %v", err)
		}
		code, _, stderr = h.run(nil, append(args, resumeFlag)...)
	}
	if code == cli.ExitToolFailure {
		h.t.Fatalf("prove failed: %s", stderr)
	}
	return code, imported
}

// witnessing opts the project into the check of a raw API log kept in a directory,
// has the mod's calls recorded in it and gives the classification a threshold of
// its own source.
func (h *harness) witnessing() {
	h.t.Helper()
	dir := filepath.Join(h.t.TempDir(), "bodies")
	h.rawLog = hostprototest.NewRawAPILog(h.t, dir, h.now)
	h.rewrite(".spec-proof/config.yaml", "ecosystems:", "host:\n  classify:\n    witness_dir: "+dir+"\npolicy:\n  tau_screen:\n    default: 0.2\n    \""+classifySource+"\": 0.5\necosystems:")
}

func (h *harness) admitting() {
	h.t.Helper()
	source, err := format.SingleVoterClassifySource(hostproto.ClassifyCalls, hostproto.ClassifyScheme, hostprototest.ClassifySessionModel)
	if err != nil {
		h.t.Fatal(err)
	}
	admission := hostprototest.ClassifyAdmissionOf(h.t, format.KindDiscriminatingPower, source, hostprototest.ClassifySessionModel)
	h.admission = &admission
}

// classifiedScreens sums the classifications recorded in the operator screens of
// the parcel criteria.
func (h *harness) classifiedScreens() (records int) {
	h.t.Helper()
	for _, criterion := range []string{"PAR-1~1", "PAR-2~1", "PAR-3~1"} {
		if screen := h.statement(criterion).Predicate.Extensions.OperatorScreen; screen != nil {
			records += len(screen.Classify)
		}
	}
	return records
}

func (h *harness) rejections() int {
	h.t.Helper()
	lookup := func(name string) (string, bool) { return h.state, name == stateVariable }
	store, err := hoststore.Open(xdg.HostDir(lookup, h.state, h.root))
	if err != nil {
		h.t.Fatal(err)
	}
	records, err := store.Records()
	if err != nil {
		h.t.Fatal(err)
	}
	count := 0
	for _, record := range records {
		if record.Event.Kind == hostproto.EventRejected {
			count++
		}
	}
	return count
}

const classifySource = "host-classify(5,rot1):" + hostprototest.ClassifySessionModel + "*5"

func TestClassifiedScreens(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the parcel project's tests against many mutants")
	}
	shared := t.TempDir()

	t.Run("a witnessed classification with a threshold of its own source screens a survivor out [verifies SP-1916~1] [verifies SP-1917~1]", func(t *testing.T) {
		h := newHarness(t, shared)
		h.admitting()
		h.witnessing()
		code, imported := h.driveClassifying("CONSISTENT", screeningOut{})
		if code != cli.ExitClean && code != cli.ExitBlocked {
			t.Fatalf("exit code %d", code)
		}
		if len(imported) == 0 {
			t.Fatal("nothing was classified")
		}
		_, screenedOut, floorEntries := h.screenTotals()
		if screenedOut == 0 || floorEntries != 0 {
			t.Errorf("screened out %d, floor entries %d", screenedOut, floorEntries)
		}
		if h.rejections() != 0 || h.classifiedScreens() == 0 {
			t.Errorf("%d classifications handed back, %d recorded", h.rejections(), h.classifiedScreens())
		}
	})

	t.Run("an unraised classification that lets a survivor go is handed to the deciders and screens nothing out [verifies SP-1916~1]", func(t *testing.T) {
		h := newHarness(t, shared)
		h.admitting()
		code, imported := h.driveClassifying("CONSISTENT", screeningOut{})
		if code != cli.ExitClean && code != cli.ExitBlocked {
			t.Fatalf("exit code %d", code)
		}
		if len(imported) == 0 {
			t.Fatal("nothing was classified")
		}
		if _, screenedOut, _ := h.screenTotals(); screenedOut != 0 {
			t.Errorf("%d survivors were screened out below the floor", screenedOut)
		}
		if h.rejections() == 0 {
			t.Error("no classification was handed to the deciders")
		}
		if records := h.classifiedScreens(); records != 0 {
			t.Errorf("%d classifications are recorded though the deciders answered", records)
		}
	})

	t.Run("an unraised classification that flags a survivor keeps it in the pool and is recorded [verifies SP-1916~1]", func(t *testing.T) {
		h := newHarness(t, shared)
		h.admitting()
		code, imported := h.driveClassifying("VIOLATES", screeningOut{})
		if code != cli.ExitClean && code != cli.ExitBlocked {
			t.Fatalf("exit code %d", code)
		}
		if len(imported) == 0 {
			t.Fatal("nothing was classified")
		}
		flagged, screenedOut, floorEntries := h.screenTotals()
		if screenedOut != 0 || flagged == 0 || floorEntries != 0 {
			t.Errorf("flagged %d, screened out %d, floor entries %d", flagged, screenedOut, floorEntries)
		}
		if h.classifiedScreens() == 0 {
			t.Error("no classification is recorded in the operator screens")
		}
	})

	t.Run("with the shipped, empty admission table nothing is classified and the screens are the host's [verifies SP-1916~1]", func(t *testing.T) {
		h := newHarness(t, shared)
		code, imported := h.driveClassifying("VIOLATES", screeningOut{})
		if code != cli.ExitClean && code != cli.ExitBlocked {
			t.Fatalf("exit code %d", code)
		}
		if len(imported) != 0 || h.classifiedScreens() != 0 {
			t.Errorf("imported %+v, recorded %d", imported, h.classifiedScreens())
		}
	})
}
