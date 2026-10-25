# agent-loop runner requirements

The runner of the agent-loop demonstration, in Shallnot's Markdown spec format. It
equips a small project with Shallnot and the proof modules the documented way,
runs one headless Claude Code session on a task in containers that hold no
credential but the model token and reach no host but the model endpoints, records
what the session and the gate did, and renders the record as a transcript. These
requirements trace to the Go tests of this module, which replace the session by a
recorded stream or a simulated Claude Code and make no model call; the container
tests run on the demonstration's own engine and skip where it is not running.

## Configuration

- **SP-2400~2**: WHEN a configuration is read THE SYSTEM SHALL take from it the
  tasks with their fixture, prompt and operation bindings, the arms among
  traceability and four-proofs, the repetitions, the seed, the model, the caps on
  assistant messages, time and spend, and the permission rules;
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
- **SP-2402~2**: WHEN the session of a run is built THE SYSTEM SHALL launch the
  pinned Linux Claude Code of the product tree non-interactively on the task's
  prompt and the configured model, with its events streamed as JSON with the hook
  events, the project's settings and a session settings file that holds the
  configured permission rules and nothing else, edits accepted, the configured
  spend cap, the plugin tree of each proof module and the umbrella plugin for the
  four-proofs arm, an isolated configuration directory in which the project is
  trusted, `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC` set, and on its PATH first the
  shims that record the hook calls of Shallnot and of each module, then the
  product's binaries and the pinned CLI.
- **SP-2403~2**: WHEN a live run is started THE SYSTEM SHALL refuse a session
  whose time cap would end after the nightly window next closes, checked before
  each session, unless the window is forced, and refuse to start without a
  credential source, on a machine the memory guard refuses, without a built Linux
  product, or with a Claude Code binary that does not report the pinned version
  inside its container; SHALL hold the session lock the canary holds; SHALL end the
  process group of the session's docker client when the session reaches its
  assistant-message cap or its time limit, record why, and remove the session's
  and the proxy's containers and the run's network whatever the session did; and
  WHEN a dry run is requested SHALL print the window, the credential source, the
  allowlist and every command of every run in order, each docker command in full
  with the token shown as reaching the session's standard input, and start
  nothing.
- **SP-2404~2**: WHEN a session has ended THE SYSTEM SHALL write into the run's
  records the description of the run with the session image, the SHA-256 of the
  Linux Claude Code binary and the egress allowlist, the stream with the time each
  line arrived, what the CLI printed to its error stream, every hook call the shims
  recorded, the egress proxy's log, the session's change as a diff against the
  equipped commit with new files included and the gate's report on the project the
  session left; SHALL then scan every record for the token and for any text of a
  token's shape; and SHALL fail the run on a hit, naming the files and not what
  they hold.

## Isolation

- **SP-2410~1**: WHEN a step of a live run touches the project THE SYSTEM SHALL run
  it in a container of the demonstration's engine, removed once it ends, as the
  user's uid with no capability and no new privilege, a read-only root file
  system, an empty scratch HOME on tmpfs, two CPUs and 2.5 GiB of memory, no
  network, the run directory mounted read-write at its own path and the product
  tree read-only, no docker socket, no engine log, and an environment emptied by
  `env -i` and refilled from a fixed list that names nothing of the host but the
  run directory.
- **SP-2411~1**: WHEN the session of a live run starts THE SYSTEM SHALL run it in a
  container with the same bounds on an internal network with no route out, behind
  an egress proxy whose own container joins that network, and SHALL hand the token
  to the container's entry on its standard input only, which starts the session's
  command with the token in that command's environment and its standard input
  closed; the token SHALL be in no command line, no environment of the docker
  client and no option of any container.
- **SP-2415~1**: WHEN the session's container is looked at from inside THE SYSTEM
  SHALL show an empty HOME, no `SSH_AUTH_SOCK`, no `/var/run/docker.sock` and no
  socket file, no host reachable without the proxy, and a proxy that forwards a
  CONNECT to the model endpoints alone, refuses every other host and every other
  request, and logs each request with its decision; and every docker command
  SHALL reach the demonstration's own engine, never the user's current context.
- **SP-2416~1**: WHEN a session's container runs with a token THE SYSTEM SHALL keep
  the token out of what `docker inspect` and `docker logs` report and out of every
  file of the run directory, while the session's command receives it.
- **SP-2417~1**: WHEN the fixture's gate test runs inside a container of the
  session image with no network, the Linux product and the recorded answers of
  the project's honest host THE SYSTEM SHALL block the gate on the weak test of
  BOX-2 with the example the tests miss and pass it once the test is strengthened.
- **SP-2418~1**: WHEN no work area is named THE SYSTEM SHALL create the run
  directories under `.tools/demo-loop/work` of the repository, never in the
  system's temporary directory.

## Transcript

- **SP-2405~1**: WHEN a recording is rendered THE SYSTEM SHALL write a Markdown
  transcript and a self-contained HTML fragment that show the run's model,
  versions and caps, the task's prompt, then in the order of time the main
  agent's messages, its commands with their output, its edits as removed and
  added lines, its skill calls, the executors it launched counted together,
  every decision of the end-of-turn gate with the message the agent received, how
  the session ended,
  the gate's verdict on the project it left with its blocking findings, the
  session's change, and the counts and timings the records hold; SHALL name for
  every entry the record and the line it comes from; SHALL take every shown text
  from the records and escape all of it in the HTML; and SHALL render a run that
  ended on a cap, a give-up of the gate or a blocked gate as it ended.
- **SP-2406~1**: WHEN a recording is rendered THE SYSTEM SHALL read files only
  and start no process, give the same bytes on every rendering, and WHEN a check
  is requested SHALL write nothing and fail when a transcript beside the
  recording differs from what it renders.
- **SP-2409~2**: WHEN the records of a run are kept as a recording THE SYSTEM
  SHALL copy them with the run directory, the repository root and the home
  directory written as `<run>`, `<repo>` and `~`, in their resolved and their
  `/private`-less forms, SHALL refuse a destination that already holds a
  recording, and SHALL refuse, keeping nothing, records that hold the token a
  credential source holds or any text of a token's shape.

## Fixture

- **SP-2407~1**: WHEN the demonstration's fixture is equipped as a run equips it
  and its task is done with its tests left as they are THE SYSTEM SHALL block the
  gate with a surviving valid mutant of BOX-2 and no blocking finding on another
  requirement, whose end-of-turn block tells the agent the example of BOX-2 the
  tests do not check and shows no mutant code, and WHEN the test of BOX-2 then
  reaches age 65 THE SYSTEM SHALL
  pass the gate, every ticket served through each module's real hooks and
  executor server by a simulated Claude Code that replays the recorded answers of
  the project's honest host.
