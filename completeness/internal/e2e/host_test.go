package e2e_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/RachidChabane/spec-proof/completeness/internal/testkit"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest/claudecode"
)

// Requirements the end-to-end tests verify.
const (
	verifiesHonestHost = " [verifies SP-794~1]"
	verifiesHookless   = " [verifies SP-795~1]"
	verifiesForger     = " [verifies SP-796~1]"

	denied = "deny"
)

// alarms are the journal events no honest run produces.
var alarms = []hostproto.EventKind{
	hostproto.EventLeaked, hostproto.EventExposed, hostproto.EventResubmissionConflict, hostproto.EventExposureAttempt,
}

func TestTheModuleThroughTheHost(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the binary and runs the project through the simulated host")
	}

	t.Run("an honest host answers every ticket through the real hooks and server, the Statement is written and the main session is denied the executor tools"+verifiesHonestHost, func(t *testing.T) {
		p := newProject(t)
		p.mustRun("init", "--hooks", "claude")
		p.mustRun("enable")
		if config, err := os.ReadFile(filepath.Join(p.root, gateConfig)); err == nil && strings.Contains(string(config), requireKey) {
			t.Fatalf("enable wrote a proofs.require entry:\n%s", config)
		}

		host := p.claudeHost()
		first := p.run("", "prove", "--all")
		if first.code != exitAwaiting {
			t.Fatalf("the first prove exited %d:\n%s", first.code, first.output())
		}
		firstTicket := p.pending().Items[0].Ticket.TicketID.String()
		last := first
		for wave := 0; last.code == exitAwaiting && wave < maxWaves; wave++ {
			host.Serve(testkit.Honest())
			last = p.run("", "prove", "--all", "--resume")
		}
		if last.code != exitClean {
			t.Fatalf("the proof ended with exit %d:\n%s", last.code, last.output())
		}

		events := p.journal()
		started := countKind(events, hostproto.EventAgentStarted)
		if started == 0 {
			t.Fatalf("no executor was started among %d events", len(events))
		}
		for _, kind := range []hostproto.EventKind{hostproto.EventFetched, hostproto.EventAccepted, hostproto.EventAttested} {
			if got := countKind(events, kind); got < started {
				t.Errorf("%d %s events for %d started executors", got, kind, started)
			}
		}
		for _, kind := range alarms {
			if got := countKind(events, kind); got != 0 {
				t.Errorf("%d %s events", got, kind)
			}
		}
		if len(p.statements()) == 0 {
			t.Fatal("the module wrote no Statement below its proof cache")
		}

		decision, code := host.Fire(hostproto.HookPreTool, map[string]any{
			"hook_event_name": "PreToolUse", "session_id": claudecode.SessionID,
			"tool_name": (moduleOf{p}).ToolName(hostproto.ToolFetchRequest), "tool_input": map[string]any{"ticket": firstTicket, "cap": ""},
		})
		if code != exitClean || decision.Permission != denied {
			t.Fatalf("a main-session call of an executor tool: %+v, exit %d", decision, code)
		}
		if countKind(p.journal(), hostproto.EventExposureAttempt) == 0 {
			t.Fatal("the attempt of the main session was not journaled")
		}
	})
}

func TestTheModuleWithoutHooks(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the binary and runs the project through the executor server")
	}

	t.Run("executors whose hooks never fire get no capability, the server refuses them and every ticket stays pending"+verifiesHookless, func(t *testing.T) {
		p := newProject(t)
		p.mustRun("init", "--hooks", "claude")
		p.mustRun("enable")
		if first := p.run("", "prove", "--all"); first.code != exitAwaiting {
			t.Fatalf("the first prove exited %d:\n%s", first.code, first.output())
		}
		server := claudecode.StartServer(t, moduleOf{p})
		listing := p.pending()
		if len(listing.Items) == 0 {
			t.Fatal("no ticket waits")
		}
		for _, item := range listing.Items {
			ticket := item.Ticket.TicketID.String()
			fetch := mustJSON(t, map[string]any{"ticket": ticket, "cap": ""})
			if fetched := server.Call(hostproto.ToolFetchRequest, fetch); !fetched.IsError {
				t.Fatalf("an uncapped fetch of %s was served: %s", ticket, fetched.Text)
			}
			submit := mustJSON(t, map[string]any{"ticket": ticket, "nonce": item.Ticket.Nonce, "output": json.RawMessage(`{}`), "cap": ""})
			if submitted := server.Call(hostproto.ToolSubmitResponse, submit); !submitted.IsError {
				t.Fatalf("an uncapped answer for %s was accepted: %s", ticket, submitted.Text)
			}
		}
		if again := p.run("", "prove", "--all", "--resume"); again.code != exitAwaiting {
			t.Fatalf("prove exited %d after answers nobody accepted:\n%s", again.code, again.output())
		}
		if accepted := countKind(p.journal(), hostproto.EventAccepted); accepted != 0 {
			t.Fatalf("%d answers were accepted without a capability", accepted)
		}
	})
}

func TestTheModuleUnderAForger(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the binary and runs the project against a forger")
	}

	t.Run("a forger that answers from the command line gets no answer accepted without a capability and the tickets stay pending"+verifiesForger, func(t *testing.T) {
		p := newProject(t)
		p.mustRun("init", "--hooks", "claude")
		p.mustRun("enable")
		last := p.run("", "prove", "--all")
		for wave := 0; last.code == exitAwaiting && wave < maxWaves/4; wave++ {
			if _, err := hostprototest.PlayCLI(p, hostprototest.PersonaForger, hostprototest.SchemaAnswers{}, func(time.Duration) {}); err != nil {
				t.Fatalf("the forger could not be played: %v", err)
			}
			last = p.run("", "prove", "--all", "--resume")
		}
		if last.code != exitAwaiting {
			t.Fatalf("prove exited %d under a forger:\n%s", last.code, last.output())
		}
		events := p.journal()
		if accepted := countKind(events, hostproto.EventAccepted); accepted != 0 {
			t.Fatalf("%d forged answers were accepted", accepted)
		}
		if countKind(events, hostproto.EventExposureAttempt)+countKind(events, hostproto.EventExposed) == 0 {
			t.Fatalf("the forger left no exposure in the journal (%d events)", len(events))
		}
		if len(p.statements()) != 0 {
			t.Fatal("a Statement was written for answers nobody was entitled to give")
		}
	})
}
