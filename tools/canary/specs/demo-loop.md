# agent-loop runner requirements

The runner of the agent-loop demonstration, in Shallnot's Markdown spec format. It
equips a small project with Shallnot and the proof modules the documented way,
runs one headless Claude Code session on a task, records what the session and
the gate did, and renders the record as a transcript. These requirements trace
to the Go tests of this module, which replace the session by a recorded stream
and make no model call.

## Configuration

- **SP-2400~1**: WHEN a configuration is read THE SYSTEM SHALL take from it the
  tasks with their fixture, prompt and operation bindings, the arms among
  traceability and four-proofs, the repetitions, the seed, the model, the caps on
  assistant messages, time and spend, the toolchain and the permission rules;
  SHALL refuse a document with an unknown member or more than one document, and
  an invalid value, naming every member at fault; and SHALL list every run of
  each task in each arm for each repetition in an order that the seed alone
  decides.

## Preparation and session

- **SP-2401~1**: WHEN a run is prepared THE SYSTEM SHALL copy the task's fixture
  into a fresh run directory, commit it with a fixed identity and date, run
  `shallnot init` and, for the four-proofs arm, `init` then `enable` of each
  proof module in order, append the task's operations to
  `.spec-proof/config.yaml` and refuse a file that already binds operations,
  commit the equipped project, run the gate on it, and record the command, the
  purpose, the exit code and the output of every step, stopping at the first
  step that fails.
- **SP-2402~1**: WHEN the session of a run is built THE SYSTEM SHALL launch the
  pinned Claude Code non-interactively on the task's prompt and the configured
  model, with its events streamed as JSON with the hook events, the project's
  settings and a session settings file that holds the configured permission
  rules and nothing else, edits accepted, the configured spend cap, the plugin
  tree of each proof module and the umbrella plugin for the four-proofs arm, an
  isolated configuration directory in which the project is trusted, and on its
  PATH first the shims that record the hook calls of Shallnot and of each module,
  then the product's binaries, the configured toolchain and the pinned CLI; and
  SHALL give the token to the session's environment only.
- **SP-2403~1**: WHEN a live run is started THE SYSTEM SHALL refuse to start it
  outside the nightly window unless the window is forced, without a credential
  source, on a machine the memory guard refuses, or with a Claude Code binary
  that does not report the pinned version, SHALL hold the session lock the
  canary holds, SHALL end the whole process group of a session that reaches its
  assistant-message cap or its time limit and record why, and WHEN a dry run is
  requested SHALL print the window, the credential source with a placeholder in
  place of the token, and every command of every run in order, and start
  nothing.
- **SP-2404~1**: WHEN a session has ended THE SYSTEM SHALL write into the run's
  records the description of the run, the stream with the time each line
  arrived, what the CLI printed to its error stream, every hook call the shims
  recorded, the session's change as a diff against the equipped commit with new
  files included, the gate's report on the project the session left, the
  mutants the discriminating-power module kept, and for each mutant a gate block
  named the diff git writes between the project's file and the same file with
  the mutant in place, and SHALL write the token into none of them.

## Transcript

- **SP-2405~1**: WHEN a recording is rendered THE SYSTEM SHALL write a Markdown
  transcript and a self-contained HTML fragment that show the run's model,
  versions and caps, the task's prompt, then in the order of time the main
  agent's messages, its commands with their output, its edits as removed and
  added lines, its skill calls, the executors it launched counted together,
  every decision of the end-of-turn gate with the message it gave, each
  counterexample a block named with its recorded diff, how the session ended,
  the gate's verdict on the project it left with its blocking findings, the
  session's change, and the counts and timings the records hold; SHALL name for
  every entry the record and the line it comes from; SHALL take every shown text
  from the records and escape all of it in the HTML; and SHALL render a run that
  ended on a cap, a give-up of the gate or a blocked gate as it ended.
- **SP-2406~1**: WHEN a recording is rendered THE SYSTEM SHALL read files only
  and start no process, give the same bytes on every rendering, and WHEN a check
  is requested SHALL write nothing and fail when a transcript beside the
  recording differs from what it renders.
- **SP-2409~1**: WHEN the records of a run are kept as a recording THE SYSTEM
  SHALL copy them with the run directory, the repository root and the home
  directory written as `<run>`, `<repo>` and `~`, in their resolved and their
  `/private`-less forms, and SHALL refuse a destination that already holds a
  recording.

## Fixture

- **SP-2407~1**: WHEN the demonstration's fixture is equipped as a run equips it
  and its task is done with its tests left as they are THE SYSTEM SHALL block the
  gate with a surviving valid mutant of BOX-2 and no blocking finding on another
  requirement, and WHEN the test of BOX-2 then reaches age 65 THE SYSTEM SHALL
  pass the gate, every ticket served through each module's real hooks and
  executor server by a simulated Claude Code that replays the recorded answers of
  the project's honest host.
