# canary requirements

The live canary of the host integration, in Shallnot's Markdown spec format. It
runs one short scripted session of a host harness against the plugins of the proof
modules and concludes, from what the session recorded, whether the harness behaves
as ADR 0005 relies on. These requirements trace to the Go tests of this module,
which run the evaluators on recorded event streams and make no model call.

## Session control

- **SP-1500~1**: WHEN a live canary is started THE SYSTEM SHALL refuse to start it
  outside the nightly window of 21:00 to 07:30 local time, the window wrapping
  midnight, unless the window is forced, and SHALL start nothing in that case.
- **SP-1505~1**: WHEN a session runs THE SYSTEM SHALL allow one session at a time,
  record the version of the harness CLI and of the binaries it runs, lower the
  priority of everything it starts, refuse to start on a machine short of memory,
  end the whole process group of a session that passes its event cap or its time
  limit, recording why it ended, and fingerprint the user's configuration
  directories of the harness before and after the session, so that a session with
  a configuration directory of its own is shown to leave them alone and one that
  shares them lists what it wrote.

## Verdicts

- **SP-1501~1**: WHEN a recorded session is evaluated THE SYSTEM SHALL conclude
  each check as PASS, FAIL or INCONCLUSIVE with pointers into the recorded files,
  SHALL fail a check only on positive contrary evidence, SHALL conclude
  INCONCLUSIVE when a probe did not happen or a field is absent, SHALL keep the
  members of an event stream it does not know, and SHALL reject a recorded line
  that is not JSON naming its file and line.
- **SP-1502~1**: WHEN the recorded session of Claude Code is evaluated THE SYSTEM
  SHALL conclude from it whether the plugin and server of each module are listed
  in `system/init`, the main session's fetch is denied, the launch rewrite drops
  the model, the executor made no call but fetch and submit, the hooks fire, the
  hook's allow issues the capability, a Stop hook fires, the executor server
  refuses a fetch with no capability, the executor server of each module starts,
  the plugin trees validate, the edit was applied and the session ended within its
  caps, and SHALL fail the Stop check only when the control hooks of the same
  project settings ran.
- **SP-1503~1**: WHEN the recorded session of Codex is evaluated THE SYSTEM SHALL
  conclude from it whether the agent files load, the hooks fire, PreToolUse names
  the agent type, the MCP tools are spelled as the module's matchers spell them,
  the read-only sandbox of the executor holds, the executor server serves a fetch
  with no capability, and a Stop hook fires, and SHALL fail the Stop check only
  when a control hook of the same hooks file ran.
- **SP-1707~1**: WHEN the recorded session of GitHub Copilot CLI is evaluated THE
  SYSTEM SHALL conclude from it whether the portable plugin and the fulfil skill of
  each module are listed before the session, the plugin trees validate, the
  executor agent starts, the hooks fire, the subagentStop hook fires, PreToolUse
  spells the executor tools as the module's matchers spell them and names the
  agent type, the tool allowlist of the executor agent is honoured, the executor
  server of each module serves a fetch with no capability, the edit was applied
  and the session ended within its caps, SHALL conclude INCONCLUSIVE on a session
  the harness refused for its credentials or quota, and SHALL conclude the
  allowlist INCONCLUSIVE when the executor tried nothing outside its two tools.

## Commands

- **SP-1504~1**: WHEN the command lines of a session are built THE SYSTEM SHALL
  give each harness one non-interactive run on its pinned model with the plugin
  of every module loaded from its generated tree, no setting that hides the hooks,
  no read of the user's settings, no persisted trust, and a prompt whose padded
  launch holds exactly one ticket, and WHEN a dry run is requested SHALL print
  those command lines and start nothing.
- **SP-1506~1**: WHEN the harness runs a hook or starts an executor server of a
  module THE SYSTEM SHALL record the payload, the decision and the exit code of
  the hook without changing either, append whole lines under concurrency, and
  hand an executor server to the real binary.
- **SP-1507~1**: WHEN the module has asked the host for requests THE SYSTEM SHALL
  put the executor of the session on a ticket of the cheapest role, probe the
  server with another ticket, copy the project without its caches, and stop the
  preparation when the module issued no ticket.
- **SP-1508~1**: WHEN a session has run THE SYSTEM SHALL write the session, the
  journal, the edited file and the report into the run's records, and WHEN the
  records of a run are given THE SYSTEM SHALL evaluate them again without
  starting any process.
- **SP-1708~1**: WHEN the command lines of a Copilot CLI session are built THE
  SYSTEM SHALL give it one non-interactive run on the pinned model with the
  portable plugin of every module loaded through `--plugin-dir`, a COPILOT_HOME of
  the run unless the shared home is chosen, a COPILOT_CACHE_HOME and a `--log-dir`
  of the run, no automatic update, a cap on the AI credits, no environment variable
  that trusts the working directory or approves every tool, and the fingerprint of
  the user's `~/.copilot` taken before and after the session.
