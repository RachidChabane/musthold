package e2e_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/format/formattest/parcel"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest/claudecode"
)

// Requirements the alignment host tests verify.
const (
	verifiesAlignmentHost  = " [verifies SP-840~1]"
	verifiesHooklessHost   = " [verifies SP-841~1]"
	verifiesAlignmentForge = " [verifies SP-842~1]"

	alignmentHostRequirements = "SP-840~1"
	hostlessRequirements      = "SP-841~1"
	forgeRequirements         = "SP-842~1"

	stateDirectory     = "spec-proof"
	alignmentStateName = "alignment"
	hostDirectory      = "host"
	journalName        = "journal.jsonl"
)

// equipAlignmentOnly installs Shallnot and the alignment module alone the way a
// user does and edits nothing by hand: `enable` leaves the policy of the run, the
// alignment proof required, the local trust and the code tree.
func (p *parcelProject) equipAlignmentOnly() {
	p.t.Helper()
	if r := p.sn("", "init", "--dir", p.root, "--hooks", "claude"); r.code != exitClean {
		p.t.Fatalf("shallnot init: %s", r.output())
	}
	if r := p.alignment("", "init", "--hooks", "claude"); r.code != exitClean {
		p.t.Fatalf("alignment init: %s", r.output())
	}
	if r := p.alignment("", "enable"); r.code != exitClean {
		p.t.Fatalf("alignment enable: %s", r.output())
	}
	const policy = "proofs:\n  require:\n    - alignment\n  trust: local\n  code:\n    - parcel\n"
	if config := p.read("shallnot.yaml"); !strings.Contains(config, policy) {
		p.t.Fatalf("enable did not leave the policy of the local loop:\n%s", config)
	}
}

// alignmentJournal reads the events of the host journal of the alignment module.
func (p *parcelProject) alignmentJournal() []hostproto.Event {
	p.t.Helper()
	matches, err := filepath.Glob(filepath.Join(p.state, stateDirectory, alignmentStateName, "*", hostDirectory, journalName))
	if err != nil || len(matches) != 1 {
		p.t.Fatalf("the journal of the alignment module: %v, %v", matches, err)
	}
	content, err := os.ReadFile(matches[0])
	if err != nil {
		p.t.Fatal(err)
	}
	var events []hostproto.Event
	for _, line := range strings.Split(strings.TrimSpace(string(content)), "\n") {
		var record hostproto.Record
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			p.t.Fatalf("a journal record: %v", err)
		}
		events = append(events, record.Event)
	}
	return events
}

// statementsOf lists the statement files the module wrote below the cache.
func (p *parcelProject) statementsOf(kind string) []string {
	p.t.Helper()
	matches, err := filepath.Glob(filepath.Join(p.root, filepath.FromSlash(cachePath), kind, "*"))
	if err != nil {
		p.t.Fatal(err)
	}
	return matches
}

func countKind(events []hostproto.Event, kind hostproto.EventKind) int {
	count := 0
	for _, event := range events {
		if event.Kind == kind {
			count++
		}
	}
	return count
}

func TestAlignmentThroughTheHost(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the binaries and runs the parcel project's tests")
	}

	t.Run("an honest host answers every ticket through the real hooks and server and the journal shows no leak, exposure or conflict"+verifiesAlignmentHost, func(t *testing.T) {
		p := newParcelProject(t, []string{alignmentHostRequirements})
		p.equipAlignmentOnly()
		p.edit(rangeCodeFile)
		last := p.proveAlignment(parcel.HonestAlignment())
		if last.code == exitAwaiting || last.code == exitToolFailed {
			t.Fatalf("the alignment proof ended with exit %d:\n%s", last.code, last.output())
		}
		events := p.alignmentJournal()
		started := countKind(events, hostproto.EventAgentStarted)
		if started == 0 {
			t.Fatalf("no executor was started: %v", events)
		}
		for _, kind := range []hostproto.EventKind{hostproto.EventFetched, hostproto.EventAccepted, hostproto.EventAttested} {
			if got := countKind(events, kind); got < started {
				t.Errorf("%d %s events for %d started executors", got, kind, started)
			}
		}
		for _, kind := range []hostproto.EventKind{hostproto.EventLeaked, hostproto.EventExposed, hostproto.EventResubmissionConflict, hostproto.EventExposureAttempt} {
			if got := countKind(events, kind); got != 0 {
				t.Errorf("%d %s events", got, kind)
			}
		}
		if len(p.statementsOf(alignmentDirectory)) == 0 {
			t.Fatal("the module wrote no alignment statement")
		}
	})

	t.Run("a call of an executor tool from the main session is denied by the hook and journaled as an exposure attempt"+verifiesAlignmentHost, func(t *testing.T) {
		p := newParcelProject(t, []string{alignmentHostRequirements})
		p.equipAlignmentOnly()
		p.edit(rangeCodeFile)
		if first := p.alignment("", "prove", "--all"); first.code != exitAwaiting {
			t.Fatalf("the first prove exited %d:\n%s", first.code, first.output())
		}
		host := claudeAlignmentHostOf(p)
		pending := host.ListPending()
		if len(pending.Items) == 0 {
			t.Fatal("no ticket waits")
		}
		ticket := pending.Items[0].Ticket.TicketID.String()
		decision, code := host.Fire(hostproto.HookPreTool, map[string]any{
			"hook_event_name": "PreToolUse", "session_id": claudecode.SessionID,
			"tool_name": alignmentModule{p}.ToolName(hostproto.ToolFetchRequest), "tool_input": map[string]any{"ticket": ticket, "cap": ""},
		})
		if code != exitClean || decision.Permission != "deny" {
			t.Fatalf("decision = %+v, exit %d", decision, code)
		}
		if countKind(p.alignmentJournal(), hostproto.EventExposureAttempt) == 0 {
			t.Fatal("the attempt was not journaled")
		}
	})
}

func TestAlignmentWithoutHooks(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the binaries and runs the parcel project's tests")
	}
	t.Run("executors whose hooks never fire get no capability, the server refuses them and no statement is written"+verifiesHooklessHost, func(t *testing.T) {
		p := newParcelProject(t, []string{hostlessRequirements})
		p.equipAlignmentOnly()
		p.edit(rangeCodeFile)
		if first := p.alignment("", "prove", "--all"); first.code != exitAwaiting {
			t.Fatalf("the first prove exited %d:\n%s", first.code, first.output())
		}
		server := claudecode.StartServer(t, alignmentModule{p})
		listing := pendingOf(p)
		if len(listing.Items) == 0 {
			t.Fatal("no ticket waits")
		}
		for _, item := range listing.Items {
			args := mustJSON(t, map[string]any{"ticket": item.Ticket.TicketID.String(), "cap": ""})
			if fetched := server.Call(hostproto.ToolFetchRequest, args); !fetched.IsError {
				t.Fatalf("an uncapped fetch of %s was served: %s", item.Ticket.TicketID, fetched.Text)
			}
			submit := mustJSON(t, map[string]any{"ticket": item.Ticket.TicketID.String(), "nonce": item.Ticket.Nonce, "output": json.RawMessage(`{}`), "cap": ""})
			if submitted := server.Call(hostproto.ToolSubmitResponse, submit); !submitted.IsError {
				t.Fatalf("an uncapped answer for %s was accepted: %s", item.Ticket.TicketID, submitted.Text)
			}
		}
		again := p.alignment("", "prove", "--all", "--resume")
		if again.code != exitAwaiting {
			t.Fatalf("prove exited %d after answers nobody accepted:\n%s", again.code, again.output())
		}
		if accepted := countKind(p.alignmentJournal(), hostproto.EventAccepted); accepted != 0 {
			t.Fatalf("%d answers were accepted without a capability", accepted)
		}
		p.verifyLocal()
		if report := p.check(); report.code == exitClean {
			t.Fatalf("shallnot check passed with no alignment proof:\n%s", report.output)
		}
	})
}

func TestAlignmentUnderAForger(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the binaries and runs the parcel project's tests")
	}
	t.Run("a forger that answers every ticket from the command line gets no verdict credit and the gate never passes"+verifiesAlignmentForge, func(t *testing.T) {
		p := newParcelProject(t, []string{forgeRequirements})
		p.equipAlignmentOnly()
		p.edit(rangeCodeFile)
		last := p.alignment("", "prove", "--all")
		for wave := 0; last.code == exitAwaiting && wave < maxWaves; wave++ {
			playForger(p, p.alignment)
			last = p.alignment("", "prove", "--all", "--resume")
		}
		events := p.alignmentJournal()
		if countKind(events, hostproto.EventExposureAttempt) == 0 && countKind(events, hostproto.EventExposed) == 0 {
			t.Fatalf("the forger left no exposure in the journal (%d events)", len(events))
		}
		for _, verdict := range alignmentVerdicts(last.stdout) {
			if verdict == "ALIGNED" {
				t.Fatalf("a forger earned ALIGNED:\n%s", last.output())
			}
		}
		p.verifyLocal()
		report := p.check()
		if report.code == exitClean {
			t.Fatalf("shallnot check passed forged proofs:\n%s", report.output)
		}
		if slices.Contains(verdictsOf(report), verdictPass) {
			t.Errorf("a criterion passed: %v", report.table())
		}
	})
}

// verdictsOf lists the verdicts of the criteria of the parcel project.
func verdictsOf(report gateReport) []string {
	var verdicts []string
	for _, verdict := range report.table() {
		verdicts = append(verdicts, verdict)
	}
	return verdicts
}

// pendingOf lists the tickets of the alignment module that wait for an answer.
func pendingOf(p *parcelProject) hostproto.Listing {
	p.t.Helper()
	r := p.alignment("", hostproto.ListArgs(hostproto.SessionProve)...)
	if r.code != exitClean {
		p.t.Fatalf("request list: %s", r.output())
	}
	var listing hostproto.Listing
	if err := json.Unmarshal([]byte(r.stdout), &listing); err != nil {
		p.t.Fatal(err)
	}
	return listing
}
