package e2e_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/RachidChabane/spec-proof/format/formattest/parcel"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest/claudecode"
)

const (
	maxWaves        = 10
	exitClean       = 0
	exitBlocked     = 1
	exitHookHold    = 2
	exitAwaiting    = 3
	stopEvent       = `{"session_id":%q,"cwd":%q}`
	enabledPolicy   = "proofs:\n  require:\n    - discriminating-power\n  trust: local\n  code:\n    - parcel\n"
	editedLine      = "\n# reviewed\n"
	reportFile      = "report.json"
	passVerdict     = "PASS"
	blockVerdict    = "BLOCK"
	escalateVerdict = "ESCALATE"
)

// Criteria of the parcel project and the Shallnot findings their verdicts raise.
const (
	adequateCriterion   = "PAR-1"
	escalatingCriterion = "PAR-2"
	weakCriterion       = "PAR-3"
	survivingValid      = "surviving_valid_mutant"
	escalatedMutant     = "escalated_mutant"
)

// editCode makes the change an agent would make: it touches the code under test.
func (p *project) editCode() {
	p.t.Helper()
	p.write("parcel/rates.py", p.read("parcel/rates.py")+editedLine)
}

// equip installs Shallnot and the module the way a user does, then gives the
// project the policy of the end-to-end run: local trust and the code tree.
func (p *project) equip() {
	p.t.Helper()
	if r := p.sn("", "init", "--dir", p.root, "--hooks", "claude"); r.code != exitClean {
		p.t.Fatalf("shallnot init: %s", r.output())
	}
	traceability := p.read("shallnot.yaml")
	if r := p.dp("", "init", "--hooks", "claude"); r.code != exitClean {
		p.t.Fatalf("init: %s", r.output())
	}
	if got := p.read("shallnot.yaml"); got != traceability {
		p.t.Fatalf("init changed shallnot.yaml:\n%s", got)
	}
	if r := p.dp("", "enable"); r.code != exitClean {
		p.t.Fatalf("enable: %s", r.output())
	}
	if got := p.read("shallnot.yaml"); got != traceability+enabledPolicy {
		p.t.Fatalf("enable did not leave the policy of the local loop:\n%s", got)
	}
}

// stop runs Shallnot's end-of-turn hook as Claude Code does.
func (p *project) stop(session string) result {
	p.t.Helper()
	return p.sn(fmt.Sprintf(stopEvent, session, p.root), "hook", "claude-stop")
}

// check runs `shallnot check` and returns its report.
type checked struct {
	result
	findings []finding
}

type finding struct {
	Category    string `json:"category"`
	Blocking    bool   `json:"blocking"`
	Requirement string `json:"requirement_id"`
	Message     string `json:"message"`
}

func (p *project) check() checked {
	p.t.Helper()
	r := p.sn("", "check", "--json-out", reportFile)
	var report struct {
		Verdict  string    `json:"verdict"`
		Findings []finding `json:"findings"`
	}
	if err := json.Unmarshal([]byte(p.read(reportFile)), &report); err != nil {
		p.t.Fatalf("the report of shallnot check: %v\n%s", err, r.output())
	}
	if (report.Verdict == "pass") != (r.code == exitClean) {
		p.t.Fatalf("verdict %q with exit %d", report.Verdict, r.code)
	}
	return checked{result: r, findings: report.Findings}
}

// settle lets a host answer the requests wave after wave, until `prove` stops
// waiting or the waves run out, and returns the last run.
func (p *project) settle(answer func() int) result {
	p.t.Helper()
	var last result
	for wave := 0; wave < maxWaves; wave++ {
		answer()
		last = p.dp("", "prove", "--changed", "--resume")
		if last.code != exitAwaiting {
			break
		}
	}
	return last
}

// start edits the code and runs the first `prove`, which waits for the host, then
// publishes the pending proofs the way the verifier does.
func (p *project) start() result {
	p.t.Helper()
	p.editCode()
	first := p.dp("", "prove", "--changed")
	if first.code != exitAwaiting {
		p.t.Fatalf("the first prove exited %d, want %d:\n%s", first.code, exitAwaiting, first.output())
	}
	p.writeVerifiedSet(awaiting(first.stdout))
	return first
}

func TestHonestHostEndToEnd(t *testing.T) {
	skipWhenShort(t)
	p := newProject(t)
	p.equip()
	first := p.start()

	t.Run("the end-of-turn hook of Shallnot holds the turn while the host owes answers [verifies SP-250~2]", func(t *testing.T) {
		stop := p.stop("e2e-honest")
		if stop.code != exitHookHold || !strings.Contains(stop.stderr, "awaiting_host") || !strings.Contains(stop.stderr, "spec-proof-dp-fulfil") {
			t.Fatalf("stop hook: exit %d\n%s", stop.code, stop.output())
		}
		for _, criterion := range []string{"PAR-1~1", "PAR-2~1", "PAR-3~1"} {
			if !strings.Contains(stop.stderr, criterion) {
				t.Errorf("the block does not name %s", criterion)
			}
		}
		if !slices.Equal(awaiting(first.stdout), []string{"PAR-1~1", "PAR-2~1", "PAR-3~1"}) {
			t.Errorf("awaiting = %v", awaiting(first.stdout))
		}
	})

	host := p.claudeHost()
	var last result
	t.Run("the executor server and the hooks serve the honest host through the waves [verifies SP-250~2]", func(t *testing.T) {
		last = p.settle(func() int { return host.Serve(parcel.Oracle{}) })
		if last.code == exitAwaiting || last.code == 2 {
			t.Fatalf("prove ended with exit %d:\n%s", last.code, last.output())
		}
		// PAR-2's tests let changes of the rounding of a discount survive; no
		// witness fails on them and no requirement fixes the rounding, so execution
		// cannot exclude them and no judgment clears them.
		want := map[string]string{"PAR-1~1": passVerdict, "PAR-2~1": escalateVerdict, "PAR-3~1": blockVerdict}
		for criterion, verdict := range want {
			if got := verdicts(last.stdout)[criterion]; !strings.HasSuffix(got, verdict) {
				t.Errorf("%s: %q, want a %s", criterion, got, verdict)
			}
		}
	})

	t.Run("prove names each survivor of the weak criterion with how it was drawn and the witness it fails, never its code [verifies SP-2421~1]", func(t *testing.T) {
		survivors := survivorLines(last.stdout, weakCriterion)
		if len(survivors) == 0 {
			t.Fatalf("no survivor is named under %s:\n%s", weakCriterion, last.stdout)
		}
		for _, line := range survivors {
			if !strings.Contains(line, "mutant of ") || !strings.Contains(line, "which every test of the criterion passes, fails the witness ") {
				t.Errorf("survivor line %q", line)
			}
		}
		for _, mutated := range []string{"total > FREE_SHIPPING_FROM", "return SHIPPING_FEE\n"} {
			if strings.Contains(last.stdout, mutated) {
				t.Errorf("prove shows mutant code %q:\n%s", mutated, last.stdout)
			}
		}
	})

	t.Run("a main-session call of an executor tool is denied and left in the journal [verifies SP-250~2]", func(t *testing.T) {
		ticket := p.firstJournaledTicket()
		denied, code := host.Fire(hostproto.HookPreTool, map[string]any{
			"hook_event_name": "PreToolUse", "session_id": claudecode.SessionID, "tool_name": p.module().ToolName(hostproto.ToolFetchRequest),
			"tool_input": map[string]string{"ticket": ticket, "cap": ""},
		})
		if code != exitClean || denied.Permission != "deny" {
			t.Fatalf("decision %+v, exit %d", denied, code)
		}
		if !slices.Contains(p.journalKinds(), hostproto.EventExposureAttempt) {
			t.Fatal("the attempt was not journaled")
		}
	})

	t.Run("Shallnot refuses the project for the weak and the escalating criteria, with the verdicts the module reached [verifies SP-250~2]", func(t *testing.T) {
		p.writeVerifiedSet(nil)
		check := p.check()
		if check.code != exitBlocked {
			t.Fatalf("shallnot check exited %d:\n%s", check.code, check.output())
		}
		wantCategory := map[string]string{weakCriterion: survivingValid, escalatingCriterion: escalatedMutant}
		raised := map[string]int{}
		for _, f := range check.findings {
			raised[f.Requirement]++
			if want, expected := wantCategory[f.Requirement]; !expected || f.Category != want || !f.Blocking {
				t.Errorf("finding %+v is neither a surviving valid mutant of %s nor an escalated mutant of %s", f, weakCriterion, escalatingCriterion)
			}
		}
		for requirement := range wantCategory {
			if raised[requirement] == 0 {
				t.Errorf("%s raised no finding", requirement)
			}
		}
		if raised[adequateCriterion] != 0 {
			t.Errorf("the passing %s raised %d findings", adequateCriterion, raised[adequateCriterion])
		}
	})

	t.Run("the end-of-turn block names each survivor of the weak criterion with the witness it fails, never its code [verifies SP-2421~1]", func(t *testing.T) {
		stop := p.stop("e2e-honest-survivor")
		if stop.code != exitHookHold || !strings.Contains(stop.stderr, "surviving_valid_mutant") {
			t.Fatalf("stop hook: exit %d\n%s", stop.code, stop.output())
		}
		if !strings.Contains(stop.stderr, "every test passes on ") || !strings.Contains(stop.stderr, "that fails the witness ") {
			t.Errorf("the block does not name a witness:\n%s", stop.stderr)
		}
		if strings.Contains(stop.stderr, "total > FREE_SHIPPING_FROM") {
			t.Errorf("the block shows mutant code:\n%s", stop.stderr)
		}
	})

	t.Run("the host-side state a proof relied on reads as attested and resolved in the journal [verifies SP-250~2]", func(t *testing.T) {
		kinds := p.journalKinds()
		for _, want := range []hostproto.EventKind{hostproto.EventAgentStarted, hostproto.EventFetched, hostproto.EventAccepted, hostproto.EventAttested, hostproto.EventRunReported, hostproto.EventConsumed} {
			if !slices.Contains(kinds, want) {
				t.Errorf("the journal holds no %s event", want)
			}
		}
		for _, absent := range []hostproto.EventKind{hostproto.EventLeaked, hostproto.EventExposed, hostproto.EventResubmissionConflict} {
			if slices.Contains(kinds, absent) {
				t.Errorf("the journal holds a %s event", absent)
			}
		}
	})
}

// execRunner runs the binary of the project as a command-line host would.
type execRunner struct{ p *project }

func (r execRunner) Run(args []string, stdin []byte) ([]byte, error) {
	result := r.p.dp(string(stdin), args...)
	if result.code != exitClean {
		return []byte(result.stdout), &runError{result.stderr}
	}
	return []byte(result.stdout), nil
}

type runError struct{ text string }

func (e *runError) Error() string { return strings.TrimSpace(e.text) }

func TestForgerFailsClosedEndToEnd(t *testing.T) {
	skipWhenShort(t)
	p := newProject(t)
	p.equip()
	p.start()
	last := p.settle(func() int {
		results, err := hostprototest.PlayCLI(execRunner{p}, hostprototest.PersonaForger, hostprototest.SchemaAnswers{}, func(time.Duration) {})
		if err != nil {
			t.Fatalf("the forger could not be played: %v", err)
		}
		return len(results)
	})
	t.Run("a forged host earns no pass in the module [verifies SP-251~1]", func(t *testing.T) {
		if last.code == exitClean {
			t.Fatalf("prove exited clean under the forger:\n%s", last.output())
		}
		for criterion, verdict := range verdicts(last.stdout) {
			if strings.HasSuffix(verdict, passVerdict) {
				t.Errorf("%s passed under the forger: %s", criterion, verdict)
			}
		}
	})
	t.Run("and none in Shallnot [verifies SP-251~1]", func(t *testing.T) {
		p.writeVerifiedSet(awaiting(last.stdout))
		check := p.check()
		if check.code == exitClean {
			t.Fatalf("shallnot passed the forged proofs:\n%s", check.output())
		}
		stop := p.stop("e2e-forger")
		if stop.code == exitClean {
			t.Fatalf("the end-of-turn hook let a forged turn end:\n%s", stop.output())
		}
	})
}

func TestHooklessHostFailsClosedEndToEnd(t *testing.T) {
	skipWhenShort(t)
	p := newProject(t)
	p.equip()
	p.start()
	host := p.claudeHost()
	t.Run("a host whose hooks never fire cannot get an answer accepted [verifies SP-251~1]", func(t *testing.T) {
		for wave := 0; wave < 2; wave++ {
			listing := host.ListPending()
			if len(listing.Items) == 0 {
				t.Fatal("no ticket waits")
			}
			for _, item := range listing.Items {
				fetched := host.Server.Call(hostproto.ToolFetchRequest, mustJSON(t, map[string]string{"ticket": item.Ticket.TicketID.String(), "cap": ""}))
				if !fetched.IsError || !strings.Contains(fetched.Text, "capability missing") {
					t.Fatalf("an uncapped fetch of %s: %+v", item.Ticket.TicketID, fetched)
				}
				submitted := host.Server.Call(hostproto.ToolSubmitResponse, mustJSON(t, map[string]any{
					"ticket": item.Ticket.TicketID.String(), "nonce": item.Ticket.Nonce, "output": json.RawMessage(`{"validity":"VALID"}`), "cap": "",
				}))
				if !submitted.IsError {
					t.Fatalf("an uncapped submit of %s was accepted: %+v", item.Ticket.TicketID, submitted)
				}
			}
			if r := p.dp("", "prove", "--changed", "--resume"); r.code != exitAwaiting {
				t.Fatalf("prove exited %d although nothing was answered:\n%s", r.code, r.output())
			}
		}
	})
	t.Run("the tickets stay pending, no Statement is written and Shallnot still holds the turn [verifies SP-251~1]", func(t *testing.T) {
		if !slices.Contains(p.journalKinds(), hostproto.EventExposureAttempt) || slices.Contains(p.journalKinds(), hostproto.EventAccepted) {
			t.Fatalf("journal = %v", p.journalKinds())
		}
		if entries, err := os.ReadDir(filepath.Join(p.root, filepath.FromSlash(statementsDirectory))); err == nil && len(entries) > 0 {
			t.Fatalf("Statements were written: %v", entries)
		}
		p.writeVerifiedSet([]string{"PAR-1~1", "PAR-2~1", "PAR-3~1"})
		stop := p.stop("e2e-hookless")
		if stop.code != exitHookHold || !strings.Contains(stop.stderr, "awaiting_host") {
			t.Fatalf("stop hook: exit %d\n%s", stop.code, stop.output())
		}
	})
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func skipWhenShort(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("builds the binary and Shallnot and runs the parcel project's tests against many mutants")
	}
}
