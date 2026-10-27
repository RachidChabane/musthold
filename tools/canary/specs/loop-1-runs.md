# loop-1 study tooling: runs and canary

The `run` and `canary` verbs of the loop-1 study (`experiments/loop-1/PREREG.md`):
the sessions of the study's order, each in containers of the study's image, with
their records, outcomes and deviations, and the canary sessions that fix the
egress allowlist, the auxiliary models and the quota patterns. These requirements
trace to Go tests that replace every session by a recorded stream and make no
model call; the container tests run on the study's engine and skip where it is
not running.

## Configuration and equipping

- **SP-2510~1**: WHEN the study's configuration `experiments/loop-1/loop-1.json` is
  read THE SYSTEM SHALL parse it, with its toolchain and the arms control and
  treatment, and SHALL order its runs by the seed exactly as the configuration of
  the agent-loop runner orders them.
- **SP-2511~1**: WHEN a task is prepared for an arm of the study THE SYSTEM SHALL
  copy and commit its fixture, run `shallnot init`, then `init` and `enable` of the
  discriminating-power, alignment and completeness modules in that order, bind the
  task's operations, write `proofs.require` of `shallnot.yaml` as `[]` for control
  and `[discriminating-power, alignment]` for treatment, whatever form the list
  had, leaving every other line as it was, and commit the result; so that the
  equipped trees of the two arms differ in `shallnot.yaml` alone.
- **SP-2512~1**: WHEN the session settings of a run of the study are written THE
  SYSTEM SHALL hold in them the permission rules of the configuration under
  `permissions.allow`, `WebFetch` and `WebSearch` under `permissions.deny`, and
  nothing else.
- **SP-2513~1**: WHEN a run of the study starts THE SYSTEM SHALL launch the Claude
  Code CLI of the study's image at the configured path, and SHALL refuse, before any
  attempt starts, a CLI whose `--version` does not begin with the pin of
  `script/claude-code.pin`.
- **SP-2514~1**: WHEN a container of a run of the study is built THE SYSTEM SHALL
  give it 3 GiB of memory and of memory and swap, at most 1024 processes, a HOME
  of its own under the run directory, created empty, and `NO_PROXY` naming the
  container's loopback.
- **SP-2517~1**: WHEN a task is prepared to its equipped state THE SYSTEM SHALL
  return the hash of its base commit and the SHA-256 of its equipped tree's file
  list and contents with every occurrence of the run directory written as `<run>`,
  the same for every run directory the task is prepared in.

## Egress

- **SP-2515~1**: WHEN the session of a run of the study starts THE SYSTEM SHALL
  have its proxy forward a CONNECT on port 443 to the hosts of the inputs
  freeze's `egress_allowlist` alone, and SHALL keep the proxy's log among the
  run's records as `egress.log`.
- **SP-2516~1**: WHEN the proxy runs in its allow-all mode THE SYSTEM SHALL forward
  every CONNECT and log each with its host, and still refuse every other request.

## Records

- **SP-2518~1**: WHEN an end-of-turn hook call of Shallnot returns in a run of the
  study THE SYSTEM SHALL number the call from 1, write `stops/<n>/report.json` with
  `shallnot check --json-out` on the same files, and write in `stops/<n>/tree.txt`
  the project's tree id, written into the run's own object store through an index
  of its own, so that the project's index and work tree do not change; nothing of
  this capture SHALL reach the harness, and the hook's exit code SHALL be the
  shim's.
- **SP-2519~1**: WHEN a session of the study is about to start and once it has
  ended THE SYSTEM SHALL write `hashes-before.json` and `hashes-after.json`, the
  SHA-256 of every file of the project, git-ignored files included and `.git/`
  left out.
- **SP-2520~1**: WHEN a session of the study has ended THE SYSTEM SHALL run the
  acceptance gate into `gate-after.json` on the project the session left with
  `shallnot.yaml`, `specs/` and `AGENTS.md` taken from the equipped commit, files
  the session added under `specs/` removed, and SHALL then put the session's own
  versions back.
- **SP-2521~1**: WHEN the session's container of a run of the study has ended THE
  SYSTEM SHALL write `container.json` with the image's id, the container's exit
  code and whether it was killed for memory, reading from the engine those two
  fields of the container and nothing else.
- **SP-2522~1**: WHEN the records of an attempt are complete THE SYSTEM SHALL scan
  every record, those under `stops/` included, for the token and for any text of a
  token's shape, replace each match by `<token>`, write `token-scan.json` with the
  records scanned and the matches replaced per record, log a `token-exposed`
  deviation through the timestamping writer when any match was replaced, and keep
  the attempt's outcome.
- **SP-2523~1**: WHEN an attempt of a run ends, whatever its outcome THE SYSTEM
  SHALL write `run.json` with the task, the arm, the repetition, the attempt, the
  model, the prompt, the command line, the caps, the versions, the start and end,
  the exit, why the session was stopped, the base commit, the equipped commit and
  the equipped tree digest, the outcome, the session model of `system/init` and
  the sorted models of the result event's usage, and SHALL render the transcript
  of every attempt whose session ran.

## Outcomes and order

- **SP-2524~1**: WHEN the outcome of an attempt is judged THE SYSTEM SHALL call it
  capped-messages or capped-time when the runner ended the session at that cap,
  capped-budget when the result event reports the spend cap, capped-memory when the
  container was killed for memory after `system/init`, harness-error when it was
  killed for memory before, when a preparation step failed, the session did not
  start or the runner was interrupted, and completed when the CLI exited by itself.
- **SP-2525~1**: WHEN the stream of a session holds a `result` event with
  `is_error` whose text matches a quota pattern of the inputs freeze, or an API
  error with HTTP status 429 THE SYSTEM SHALL call the attempt a quota stop, end the
  session at that event, and record the reset time the error names, when it names
  one.
- **SP-2526~1**: WHEN an attempt's base commit, equipped tree digest or session
  model differs from the inputs freeze THE SYSTEM SHALL call it a harness error;
  and WHEN a recorded version differs from the inputs freeze or the usage names a
  model beyond the session model and `cli_auxiliary_models` THE SYSTEM SHALL log a
  provenance deviation and keep the outcome.
- **SP-2527~1**: WHEN the attempts of a run are counted THE SYSTEM SHALL number them
  from 1 in fresh run directories, count harness errors toward the limit of three
  and quota stops not, make the run harness-blocked at its third harness error,
  and count an attempt directory with no `run.json` as a harness error.
- **SP-2528~1**: WHEN an attempt is a quota stop THE SYSTEM SHALL start no further
  attempt in that invocation; and WHEN the next invocation starts THE SYSTEM SHALL
  resume the order at that run in a fresh attempt directory, and refuse to start
  it before the reset time the stop recorded.
- **SP-2529~1**: WHEN 28 days have passed since the start of the first attempt THE
  SYSTEM SHALL start no attempt, and SHALL give each run with no counted attempt the
  state quota-blocked when its last attempt was a quota stop, harness-blocked when
  it was a harness error, and not-run when it has none.
- **SP-2530~1**: WHEN the runs of the study are executed THE SYSTEM SHALL refuse an
  order that differs from the inputs freeze's or from the configuration's seeded
  order, run the attempts strictly one at a time in that order, and resume at the
  first run that is neither counted nor blocked.
- **SP-2531~1**: WHEN a quota stop, a harness error, a blocked run, a provenance
  difference or an exposed token occurs THE SYSTEM SHALL append one entry of that
  kind to `DEVIATIONS.md` in the deviation log's format, with the run as its scope
  and the protocol's section, write the title first in a new log, hand the
  timestamped kinds to the timestamping writer, and never log a blocked run twice.
- **SP-2532~1**: WHEN a dry run of the study is requested THE SYSTEM SHALL print the
  order and, for the next run to start, every command of its attempt, each docker
  command in full with the token shown as reaching the session's standard input,
  and SHALL start nothing and read no credential value.
- **SP-2533~1**: WHEN a run of the study is prepared THE SYSTEM SHALL let no truth
  file, truth path or truth digest of the task into the run directory, the mounts
  of the session's container, the prompt or the session settings.
- **SP-2534~1**: WHEN the `run` verb is invoked THE SYSTEM SHALL read the
  repository, the configuration, the work directory, the inputs freeze, the image
  and the CLI path from its flags, exit 0 when the invocation ends without a
  refusal, 1 on a refused or failed check, and 2 on a usage error.

## Canary

- **SP-2535~1**: WHEN canary C1 runs THE SYSTEM SHALL run a one-turn session of the
  pinned CLI and model in an empty project through the proxy in its allow-all
  mode, and SHALL give as `egress_observed` every host the proxy forwarded to, and
  as `egress_allowlist` those that are `anthropic.com` or `claude.ai` or end in
  `.anthropic.com` or `.claude.ai`.
- **SP-2536~1**: WHEN canary C2 runs THE SYSTEM SHALL run the same session through
  the proxy with C1's allowlist enforced and, in the same container,
  `curl https://github.com` and a read of a host path outside the two mounts; and
  SHALL hold C2 when the session completes, the proxy refused no host C1 recorded,
  and both probes failed.
- **SP-2537~1**: WHEN canary C3 runs THE SYSTEM SHALL run a treatment session on
  `boxoffice` in its done state with the prompt "Run the `musthold:prove-all` skill
  and tell me the gate's verdict.", and SHALL hold C3 when `system/init` lists
  every plugin the run gives the session and names the session model, the
  acceptance gate passes with both proofs required and no alignment verdict below
  the isolation floor, and the detectors find no hit; the other models of its
  usage SHALL be `cli_auxiliary_models`.
- **SP-2538~1**: WHEN the canary ends THE SYSTEM SHALL keep its records under the
  work directory's `canary/`, and write there the canary's output: whether each of
  C1 to C3 held, `egress_observed`, `egress_allowlist`, `cli_auxiliary_models`, and
  the runner's fixed `quota_patterns`.
- **SP-2539~1**: WHEN the `canary` verb is invoked THE SYSTEM SHALL read its flags
  as the `run` verb does, exit 0 when C1 to C3 hold, 1 when one fails or a check
  refuses, and 2 on a usage error.
