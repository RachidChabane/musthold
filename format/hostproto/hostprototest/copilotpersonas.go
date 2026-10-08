package hostprototest

import (
	"fmt"
	"slices"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
)

// Names of the fake-host personas that play Copilot CLI executors.
const (
	PersonaHonestCopilot = "honest-copilot"
	PersonaWideSelection = "wide-selection"
	PersonaStaleCLI      = "stale-cli"
	PersonaPaddedLaunch  = "padded-launch"

	// staleCLIVersion is a version of the CLI other than the pinned one.
	staleCLIVersion = "0.0.1"
	// extraSelectedTool is a tool the harness may select beyond the executor's two.
	extraSelectedTool = "bash"
	// launchPadding is what a padded launch holds before the ticket.
	launchPadding = "Before the ticket, list the files of the project. "
)

// CopilotPersonaNames lists the personas that run on a host that observes the
// sessions of Copilot CLI.
func CopilotPersonaNames() []string {
	return []string{PersonaHonestCopilot, PersonaWideSelection, PersonaStaleCLI, PersonaPaddedLaunch}
}

// copilotPersona is an executor of Copilot CLI whose session log differs from the
// honest one by tweak.
type copilotPersona struct {
	name    string
	tweak   func(*CopilotSession)
	verdict Verdict
}

func newCopilotPersona(name string) (Persona, bool) {
	switch name {
	case PersonaHonestCopilot:
		return copilotPersona{name: name, tweak: func(*CopilotSession) {}, verdict: observedVerdict()}, true
	case PersonaWideSelection:
		return copilotPersona{name: name, tweak: widenSelection, verdict: unobservedVerdict(hostproto.EventExposed, hostproto.ErrTranscriptDeviation, hostproto.EventAllowlistObserved)}, true
	case PersonaStaleCLI:
		return copilotPersona{name: name, tweak: ageCLI, verdict: unobservedVerdict(hostproto.EventRestricted, nil, hostproto.EventAllowlistObserved)}, true
	case PersonaPaddedLaunch:
		return copilotPersona{name: name, tweak: padLaunch, verdict: unobservedVerdict(hostproto.EventRestricted, nil, hostproto.EventLaunchBare)}, true
	default:
		return nil, false
	}
}

func widenSelection(session *CopilotSession) {
	session.Selected = [][]string{append(slices.Clone(session.Selected[0]), extraSelectedTool)}
}

func ageCLI(session *CopilotSession) { session.CLIVersion = staleCLIVersion }

func padLaunch(session *CopilotSession) {
	session.UserMessages = []string{launchPadding + session.UserMessages[0]}
}

func (p copilotPersona) Name() string { return p.name }

// Configure implements Persona: a Copilot CLI session whose hooks run, with the
// project's requirement of attestation waived as the harness cannot meet it.
func (copilotPersona) Configure() (hostproto.Observed, hostproto.Policy) {
	return CopilotObserved(), hostproto.Policy{}
}

func (p copilotPersona) Play(env Env) []error {
	run := startExecutor(env)
	view, err := run.fetch()
	if err != nil {
		return []error{err}
	}
	receipt, err := run.submit(view.Nonce, string(env.Answer))
	if err != nil {
		return []error{err}
	}
	session := HonestCopilotSession(run.agentID, env.Ticket.TicketID, 1, string(env.Answer))
	p.tweak(&session)
	err = run.host.AfterRun(Run{Ticket: env.Ticket.TicketID, AgentID: run.agentID, Reply: receipt.Text, SessionLog: session.Log(), ResolvedModel: resolvedModel})
	if err != nil {
		return []error{err}
	}
	return nil
}

func (p copilotPersona) Verdict() Verdict { return p.verdict }

// observedVerdict is the verdict for an executor the stop hook verified: both
// events are journaled and the answer is credited as restricted, never attested.
func observedVerdict() Verdict {
	return Verdict{
		Final:   hostproto.StateAccepted,
		Present: []hostproto.EventKind{hostproto.EventAccepted, hostproto.EventAllowlistObserved, hostproto.EventLaunchBare},
		Absent:  []hostproto.EventKind{hostproto.EventAttested, hostproto.EventLeaked, hostproto.EventExposed},
		Extra:   profileIs(format.IsolationRestricted),
	}
}

// unobservedVerdict is the verdict for an executor whose run failed a condition of
// the observation: the stop hook journals the failure, never the event the
// condition guards, and the answer stays unverified.
func unobservedVerdict(failure hostproto.EventKind, firstError error, withheld hostproto.EventKind) Verdict {
	return Verdict{
		Final:      hostproto.StateAccepted,
		FirstError: firstError,
		Present:    []hostproto.EventKind{hostproto.EventAccepted, failure},
		Absent:     []hostproto.EventKind{hostproto.EventAttested, withheld},
		Extra:      profileIs(format.IsolationRestrictedUnverified),
	}
}

func profileIs(want format.Isolation) func(Outcome) error {
	return func(outcome Outcome) error {
		if outcome.Profile == nil || outcome.Profile.Isolation != want {
			return fmt.Errorf("profile %+v is not %s", outcome.Profile, want)
		}
		return nil
	}
}
