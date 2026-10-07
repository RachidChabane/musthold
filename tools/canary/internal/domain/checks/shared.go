package checks

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/harness"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/evidence"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/modules"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/verdict"
)

// Names of the checks both harnesses share.
const (
	checkCLIVersion      = "cli_version_pinned"
	checkCapWithoutHooks = "cap_without_hooks"
	checkStopFires       = "stop_fires"
	checkEditApplied     = "edit_applied"
	checkWithinCaps      = "session_within_caps"
	checkServersStart    = "executor_servers_start"
	checkConfigUntouched = "user_config_untouched"
)

// noExecutorNote is the summary of a check whose subject never ran.
const noExecutorNote = "the session launched no executor of the primary module"

func primaryModule() modules.Module { return modules.Primary() }

// strategy returns the harness's rules as the primary module spells them.
func strategy(flag string) harness.Harness {
	selected, err := harness.ByFlag(primaryModule().Identity(), flag)
	if err != nil {
		panic(err)
	}
	return selected
}

// pinnedVersion is the CLI version the module's rules were checked against.
func pinnedVersion(flag string) string { return strategy(flag).Version }

// cliName is the executable of a harness.
func cliName(flag string) string { return flag }

func versionCheck(flag string) Check {
	return newCheck(checkCLIVersion,
		"The canary ran on the CLI version the module's rules were checked against; a mark in ADR 0005 holds for that version only.",
		func(found evidence.Evidence) verdict.Verdict {
			line, recorded := found.Session.Versions[cliName(flag)]
			pointer := evidence.At(evidence.SessionFile, 0, "versions")
			switch {
			case !recorded:
				return verdict.Undecided("the session did not record the CLI version", pointer)
			case strings.Contains(line, pinnedVersion(flag)):
				return verdict.Passed(fmt.Sprintf("%s reports %q, pinned %s", cliName(flag), line, pinnedVersion(flag)), pointer)
			default:
				return verdict.Undecided(fmt.Sprintf("%s reports %q but the module is pinned to %s: the verdicts describe the version that ran", cliName(flag), line, pinnedVersion(flag)), pointer)
			}
		})
}

// capWithoutHooksCheck probes the executor server with no capability, as a host
// without hooks would call it. What is right depends on the harness: where the
// project's requirement of attestation applies, the server must refuse; where the
// harness cannot issue a capability, the requirement is waived and it serves.
func capWithoutHooksCheck(flag string) Check {
	mustRefuse := strategy(flag).AttestationRequired(true)
	proves := "The executor server serves a ticket to a caller whose hooks added no capability only where the harness waives attestation."
	if mustRefuse {
		proves = "Without the hooks no capability reaches the executor server, and the server refuses to serve the ticket: provenance fails closed."
	}
	return newCheck(checkCapWithoutHooks, proves, func(found evidence.Evidence) verdict.Verdict {
		probe := found.NoCall.CapProbe
		pointer := evidence.At(evidence.NoCallFile, 0, "capProbe")
		switch {
		case !probe.Attempted:
			return verdict.Undecided("the cap probe did not run", pointer)
		case !probe.Responded:
			return verdict.Undecided("the executor server never answered the probe: "+probe.Detail, pointer)
		case mustRefuse && probe.ServedTicket:
			return verdict.Failed("fail-open: fetch_request without a capability served the ticket", pointer)
		case mustRefuse && probe.Refused:
			return verdict.Passed("fetch_request without a capability was refused: "+probe.Detail, pointer)
		case mustRefuse:
			return verdict.Failed("fetch_request without a capability was neither refused nor served: "+probe.Detail, pointer)
		case probe.Refused:
			return verdict.Failed("the server refused although the harness waives attestation: "+probe.Detail, pointer)
		case probe.ServedTicket:
			return verdict.Passed("fetch_request without a capability served the ticket, as attestation is waived on this harness", pointer)
		default:
			return verdict.Undecided("the answer is neither a refusal nor a ticket: "+probe.Detail, pointer)
		}
	})
}

// editAppliedCheck verifies the work the prompt asked of the agent: one line of the
// project replaced. It shows that the session ran to the point of an edit under the
// permissions the canary granted.
func editAppliedCheck() Check {
	return newCheck(checkEditApplied,
		"The agent could edit the project under the canary's permission settings: the session did real work, not only the probes.",
		func(found evidence.Evidence) verdict.Verdict {
			pointer := evidence.At(evidence.EditedFile, 0, found.Session.EditFile)
			switch {
			case found.Edited == "" || found.Session.EditFrom == "":
				return verdict.Undecided("the edited file was not recorded", pointer)
			case strings.Contains(found.Edited, found.Session.EditTo) && !strings.Contains(found.Edited, found.Session.EditFrom+"\n"):
				return verdict.Passed("the line was replaced", pointer)
			case strings.Contains(found.Edited, found.Session.EditFrom):
				return verdict.Failed("the line the agent was asked to replace is unchanged", pointer)
			default:
				return verdict.Undecided("the file changed, but not the way the prompt asked", pointer)
			}
		})
}

// stopProbeCheck verifies that the harness runs a Stop hook in a non-interactive
// session, with a probe hook the canary installs in the temporary project: the
// modules install none.
// stopSources are what a Stop check reads of a harness: whether the session
// finished, the Stop events its own stream shows, and the control hooks that come
// from the same project settings as the Stop probe. A Stop probe that stays silent
// beside a silent control proves only that the settings were not loaded.
type stopSources struct {
	finished    func(evidence.Evidence) (bool, verdict.Pointer)
	streamStops func(evidence.Evidence) []verdict.Pointer
	controls    func(evidence.Evidence) []verdict.Pointer
}

func stopProbeCheck(sources stopSources) Check {
	return newCheck(checkStopFires,
		"The end-of-turn Stop hook runs under non-interactive execution, so a gate hung on it is reached.",
		func(found evidence.Evidence) verdict.Verdict {
			var pointers []verdict.Pointer
			for _, probe := range found.Probes(evidence.LabelStopProbe) {
				pointers = append(pointers, evidence.HooksAt(probe.Line, "Stop probe payload "+probe.Payload().EventName))
			}
			pointers = append(pointers, sources.streamStops(found)...)
			if len(pointers) > 0 {
				return verdict.Passed("a Stop hook ran in the session", pointers...)
			}
			controls := sources.controls(found)
			if len(controls) == 0 {
				return verdict.Undecided("no hook of the project settings ran, control hooks included: the settings were not loaded, so Stop is untested")
			}
			if finished, pointer := sources.finished(found); finished {
				return verdict.Failed("the session finished normally, the project's control hooks ran and no Stop hook did", append(controls, pointer)...)
			}
			return verdict.Undecided("the session did not finish, so a missing Stop proves nothing", controls...)
		})
}

func serversStartCheck() Check {
	return newCheck(checkServersStart,
		"The executor server of each module starts in the prepared project, answers the initialize handshake and lists fetch_request and submit_response: a server missing from the harness's listing is then the harness's doing, not the module's.",
		func(found evidence.Evidence) verdict.Verdict {
			servers := found.NoCall.Servers
			pointer := evidence.At(evidence.NoCallFile, 0, "servers")
			if len(servers) == 0 {
				return verdict.Undecided("no server was started", pointer)
			}
			var failed []string
			for _, server := range servers {
				if !server.Started {
					failed = append(failed, server.Binary+": "+server.Detail)
				}
			}
			if len(failed) > 0 {
				return verdict.Failed("servers that did not start: "+strings.Join(failed, "; "), pointer)
			}
			return verdict.Passed(fmt.Sprintf("%d executor servers answered the handshake and listed their tools", len(servers)), pointer)
		})
}

// configUntouchedCheck reads the fingerprint of the places a harness CLI keeps its
// state in the user's home. A session that has its own configuration directory
// must leave them alone; a session that shares the user's records what it wrote.
func configUntouchedCheck() Check {
	return newCheck(checkConfigUntouched,
		"The session wrote nothing that names its run in the user's own configuration directories. A session that shares them lists what it wrote; the changes nothing ties to the run are another process's and are only counted.",
		func(found evidence.Evidence) verdict.Verdict {
			pointer := evidence.At(evidence.TouchedFile, 0, "attributed")
			touched := found.Touched
			switch {
			case len(touched.Roots) == 0:
				return verdict.Undecided("the user's configuration directories were not fingerprinted", pointer)
			case found.Session.ConfigMode == evidence.ConfigShared:
				return verdict.Undecided(fmt.Sprintf("the session shares the user's configuration: it wrote %d entries that name its run (listed in %s), and the credential file may also have been updated; %d other changes are not attributed", len(touched.Attributed), evidence.TouchedFile, len(touched.Unattributed)), pointer)
			case len(touched.Attributed) > 0:
				return verdict.Failed(fmt.Sprintf("an isolated session wrote %d entries in the user's configuration: %s", len(touched.Attributed), strings.Join(touched.Attributed, ", ")), pointer)
			default:
				return verdict.Passed(fmt.Sprintf("nothing that names the run changed in %s (%d unattributed changes belong to other processes)", strings.Join(touched.Roots, ", "), len(touched.Unattributed)), pointer)
			}
		})
}

func withinCapsCheck(finished func(evidence.Evidence) (summary string, ended bool, pointer verdict.Pointer)) Check {
	return newCheck(checkWithinCaps,
		"The session ended by itself inside the turn, time and spend caps the runner enforces.",
		func(found evidence.Evidence) verdict.Verdict {
			session := evidence.At(evidence.SessionFile, 0, "killed")
			if found.Session.Killed != evidence.KilledNone {
				return verdict.Failed(fmt.Sprintf("the runner ended the session: %s (event cap %d, timeout %ds)", found.Session.Killed, found.Session.EventCap, found.Session.TimeoutSeconds), session)
			}
			summary, ended, pointer := finished(found)
			if !ended {
				return verdict.Undecided("the stream holds no closing event", session)
			}
			return verdict.Passed(summary, pointer)
		})
}

// reportedModel reads the model a run_reported journal detail carries.
func reportedModel(detail string) string {
	var report struct {
		Model string `json:"model"`
	}
	_ = json.Unmarshal([]byte(detail), &report)
	return report.Model
}

// capOf reads the capability member of a tool input.
func capOf(input json.RawMessage) string {
	var members struct {
		Cap string `json:"cap"`
	}
	_ = json.Unmarshal(input, &members)
	return members.Cap
}

// agentInput is the members of an Agent tool call the checks read.
type agentInput struct {
	SubagentType    string `json:"subagent_type"`
	Prompt          string `json:"prompt"`
	Model           string `json:"model"`
	RunInBackground bool   `json:"run_in_background"`
}

func decodeAgentInput(input json.RawMessage) agentInput {
	var decoded agentInput
	_ = json.Unmarshal(input, &decoded)
	return decoded
}

// hasEvent reports whether the journal holds an event of the kind for the ticket.
func hasEvent(found evidence.Evidence, kind hostproto.EventKind, ticket string) (evidence.JournalEntry, bool) {
	for _, entry := range found.Events(kind) {
		if entry.Event.Ticket == ticket {
			return entry, true
		}
	}
	return evidence.JournalEntry{}, false
}
