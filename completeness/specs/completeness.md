# completeness requirements

The requirements of the `completeness` module, which advises on what a
specification leaves unstated, in Shallnot's Markdown spec format. `shallnot gate`
traces them to the Go tests of the module in CI.

## Binary

- **SP-102~1**: WHEN the `spec-proof-completeness` binary is run with the
  `version` argument THE SYSTEM SHALL print its version.

## Checklist

- **SP-500~1**: WHEN the checklist is read THE SYSTEM SHALL list the eight
  dimensions of c1, `error_handling`, `boundaries`, `permissions`, `concurrency`,
  `compatibility`, `input_validation`, `time_and_locale` and
  `state_and_persistence`, in that order, each with a description and a phrase,
  and the standings COVERED, NOT_APPLICABLE and MISSING.
- **SP-501~1**: WHEN the dimensions to ask about are chosen THE SYSTEM SHALL leave
  out those the spec declares out of scope, read a declaration of a name outside
  the checklist as excluding nothing, and SHALL never raise a question for a
  dimension it did not ask.
- **SP-502~1**: WHEN the dimension request of a spec is built THE SYSTEM SHALL ask
  one choice question per dimension asked, answered COVERED, NOT_APPLICABLE or
  MISSING, over the requirements of the spec shown under opaque tokens, with a
  schema that requires each question to be answered once.
- **SP-503~1**: WHEN the answers about the dimensions are read THE SYSTEM SHALL
  raise a question for MISSING at a confidence at or above `tau_adv`, raise a
  question flagged as low confidence for MISSING below it, raise none for any
  other answer, and SHALL never lose a question when `tau_adv` rises.

## Changes since the base

- **SP-504~1**: WHEN the units of a changed Go file are compared with those before
  the change THE SYSTEM SHALL give one hunk per function, method, package-level
  variable or constant that is new or whose code differs, and none for a unit
  whose syntax tree is the same whatever its formatting and comments, and none
  for a unit the change deleted.
- **SP-505~1**: WHEN the units of a changed Python file are compared with those
  before the change THE SYSTEM SHALL give one hunk per function, method or
  module-level assignment that is new or whose code differs, and none for a unit
  whose code is the same whatever its comments, docstrings, blank lines and
  spacing, a change of a body, a literal, a decorator or a nesting being a
  change.
- **SP-506~1**: WHEN the files of a change are selected THE SYSTEM SHALL read for
  hunks only the source files an ecosystem serves that hold no tests, lie outside
  the test roots of the report and are not spec files, skip a deleted file, and
  report a file that does not parse without losing the others.
- **SP-507~1**: WHEN the change since the base is read THE SYSTEM SHALL compare the
  work tree with the merge base of HEAD and the default branch, `origin/HEAD`,
  then `init.defaultBranch`, then `main`, then `master`, or with HEAD when none
  exists, or with the reference a flag names, include untracked files as added
  files, follow renames, leave the repository and its index untouched, and
  report a project with no base to compare with without failing the run.
- **SP-508~1**: WHEN entry names are closed over the units of a project THE SYSTEM
  SHALL reach the units they designate and, transitively, those the reached units
  mention, across files and through assignments, reach the constructor of a
  class named as an entry, end on a cycle, and reach exactly the union of what
  each entry reaches.
- **SP-509~1**: WHEN a changed unit is justified THE SYSTEM SHALL name the
  requirements whose example probes or ALIGNED tests may execute it, sorted and
  without repeats, designate the functions of a test from its label and those of
  an example from its operation binding, read the evidence only from valid
  Statements of the requirement's current text whose test and code inputs still
  match the project, count a test only when it is ALIGNED, and justify nothing
  without evidence.

## Questions

- **SP-510~1**: WHEN a changed unit that no example probe and no ALIGNED test may
  execute is checked THE SYSTEM SHALL ask one choice question over the criteria
  of the project and NONE, as a tournament above 23 criteria, and raise an
  `unjustified_change` question when the tournament ends on NONE at a
  confidence at or above `tau_adv`, and none otherwise, ask at most
  `max_hunks` units per spec in file and line order and report the rest.
- **SP-556~1**: WHEN a project has several specs THE SYSTEM SHALL attach the
  question of an unjustified change to the first spec, in path order, whose
  requirements have a discriminating-power Statement that executes the changed
  file, and to the first spec of the project when none does.
- **SP-511~1**: WHEN a requirement's discriminating-power Statement lists mutants no
  requirement owns and executes a changed file THE SYSTEM SHALL raise one
  `unconstrained_behaviour` question per requirement and changed file, naming
  the changed functions of the file, the number of mutants and the first of
  their digests, and SHALL raise none without such a Statement.
- **SP-512~1**: WHEN the testability of the requirements of a spec is checked THE
  SYSTEM SHALL ask one choice question per requirement, answered TESTABLE,
  AMBIGUOUS or UNTESTABLE, in groups of at most 64, and raise a `clarity`
  question for each requirement answered AMBIGUOUS or UNTESTABLE at any
  confidence.
- **SP-518~1**: WHEN a question is written THE SYSTEM SHALL build its text from a
  deterministic template of its signal, name the requirements it concerns, merge
  identical questions with the union of their requirements, and return the
  questions sorted and the same for any order of the signals.
- **SP-519~1**: WHEN a Statement is written THE SYSTEM SHALL mark it advisory and give
  it no verdict, whatever the host answers, and validate it against the schemas
  before it is stored.

## Closed requests

- **SP-513~1**: WHEN a closed request is built THE SYSTEM SHALL show the
  requirements under opaque tokens in an order that is a pure function of the
  work, remove every requirement reference and identifier of the project from
  every text it shows, and name no requirement in any payload.
- **SP-514~1**: WHEN a closed request is built THE SYSTEM SHALL carry its question
  set with the options of each question, require in its schema exactly one answer
  per question with one of its options and an optional confidence in [0,1], and
  read an answer back by question id.
- **SP-515~1**: WHEN the votes of a closed request are read THE SYSTEM SHALL use a
  relayed first vote alone with the confidence the endpoint gave, none counting
  as zero, and otherwise take the modal choice of each question, ties to the
  smaller choice, with its share of the votes as confidence, and name the source
  of the confidence.
- **SP-516~1**: WHEN a project has more than 23 criteria THE SYSTEM SHALL play a
  closed choice question as groups of at most 23 plus NONE and a final round
  among the winners, take the minimum confidence over the rounds, recompute the
  plan from the recorded answers, and end for any number of criteria.
- **SP-517~1**: WHEN a request is built THE SYSTEM SHALL give it the same identity
  for the same work and another identity when a statement or the code shown
  changes, and a template digest that does not depend on the tokens of one
  request and differs between the three signals.

## Decision policy

- **SP-520~1**: WHEN the decision policy is built THE SYSTEM SHALL default to the
  parameters of the ADRs, refuse a threshold outside [0,1] and a count below one,
  and let every parameter reach the policy digest.
- **SP-521~1**: WHEN the module's configuration is decoded THE SYSTEM SHALL read
  the decision parameters, keep the default of an absent one and refuse an
  unknown key or an invalid parameter.

## Criteria and proofs read from the project

- **SP-522~1**: WHEN a Shallnot report of schema 1.2 or later is read THE SYSTEM
  SHALL give every spec file of the run, including one without requirements, with
  its active requirements, their statements, statement digests and examples, its
  out-of-scope declarations and the test roots, treat a report without
  declarations as declaring nothing, and SHALL refuse a report below 1.2, an
  active requirement without a digest or a spec file, and a malformed report.
- **SP-523~1**: WHEN criteria are loaded THE SYSTEM SHALL run `shallnot gate
  --advisory --json-out` on the project, read the report it wrote, remove it, and
  parse no spec itself.
- **SP-524~1**: WHEN the Statements of another module are looked up for a criterion
  THE SYSTEM SHALL list those of the verified set before those of the local
  cache, only those of the kind asked for, and none of another criterion.

## Statements

- **SP-525~1**: WHEN the pending file is written THE SYSTEM SHALL list the criteria
  that wait for tickets as `awaiting_host` and those whose host stopped answering
  as `host_unresponsive`, in the form the format fixes, and no file when nothing
  waits.
- **SP-526~1**: WHEN a spec is proved THE SYSTEM SHALL write one completeness
  Statement per spec file, about the spec path and the hash of the file, with
  the spec, its requirements, the policy, the templates, the changed files, the
  changed units, the Statements it read as inputs of role `proof` and the
  answers it rests on, with the sources and the lowest confidence of the
  answers in `extensions.decision`, sorted, canonical, and with another cache
  key when any input changes.
- **SP-527~1**: WHEN a Statement is written to the proof cache THE SYSTEM SHALL
  validate it against the embedded schemas first, refuse an invalid one, and
  write it atomically at
  `.spec-proof/cache/completeness/<spec path>/<cacheKey>.json`, the spec path
  percent-encoded into one directory name.
- **SP-528~1**: WHEN a Statement is validated against its schema THE SYSTEM SHALL
  accept a valid completeness Statement, refuse one that breaks its schema, and
  refuse a document that is not a Statement of the format before the schema.
- **SP-529~1**: WHEN a decision is memoized THE SYSTEM SHALL return it for its key,
  treat an absent or unreadable entry as a miss and write atomically.

- **SP-530~1**: WHEN `prove --changed` runs THE SYSTEM SHALL skip a spec whose inputs
  are unchanged since its Statement and ask the host nothing, and `prove --all`
  SHALL prove it again to the same bytes.

## Host protocol

- **SP-531~1**: WHEN the host store of a project is opened THE SYSTEM SHALL keep
  `queue.json`, `requests/`, `tickets/`, `responses/`, a digest-chained
  `journal.jsonl` and a `secret` under
  `$XDG_STATE_HOME/spec-proof/completeness/<repo-digest>/host/`, with directories
  private to the user and the secret readable by the user only, and SHALL derive
  the repository digest from the absolute path of the project root.
- **SP-532~1**: WHEN the filesystem store is tested THE SYSTEM SHALL satisfy the
  shared Store contract suite of the host protocol.
- **SP-533~1**: WHEN a store is opened again THE SYSTEM SHALL return the requests,
  entries, responses, journal chain and secret it held, and SHALL write each
  record atomically so that a reader never sees a partial file.
- **SP-534~1**: WHEN a request is recorded in the queue THE SYSTEM SHALL keep the
  wave of its first record, merge the criteria it serves without duplicates and
  list the items ordered by request id, and WHEN the wave advances THE SYSTEM
  SHALL return consecutive numbers starting at one.
- **SP-535~1**: WHEN the state and cache locations are resolved THE SYSTEM SHALL
  use `$XDG_STATE_HOME` and `$XDG_CACHE_HOME` when they are absolute paths and
  the defaults under the home directory otherwise.
- **SP-536~1**: WHEN an executor's output is checked against a response schema THE
  SYSTEM SHALL accept it exactly when the schema holds and list every violation
  otherwise.
- **SP-540~1**: WHEN recorded answers are replayed THE SYSTEM SHALL answer every
  pending ticket from the recording of its request through the same checks and
  fail with a miss when a request has no recording, and WHEN answers are recorded
  THE SYSTEM SHALL keep each output and its executor under the request id.
- **SP-541~1**: WHEN a command is run THE SYSTEM SHALL capture its output within a
  limit and report its exit status, a non-zero status being a result.
- **SP-542~1**: WHEN a command exceeds its deadline or its context ends THE SYSTEM
  SHALL kill its whole process group and report the timeout.

## The prove use case

- **SP-543~1**: WHEN `prove` runs THE SYSTEM SHALL exit 3 while tickets wait for the
  host, 1 when the host stopped answering, 0 otherwise whatever questions it
  raised, list the criteria of the specs that wait as `awaiting_host`, write no
  Statement for them and leave the pending file empty once nothing waits, and
  SHALL continue a resume with the base of the run it continues.

- **SP-544~1**: WHEN `prove` is given conflicting flags or unexpected arguments THE
  SYSTEM SHALL fail without acting, and WHEN a command is unknown or missing THE
  SYSTEM SHALL fail with a usage line.
- **SP-545~1**: WHEN `request list|show|submit` runs THE SYSTEM SHALL list, show and
  accept answers through the same checks as the host protocol, a refused answer
  SHALL exit as a refusal, not a tool failure, and the real binary SHALL be
  driven to the same Statement by the real fake host.
- **SP-546~1**: WHEN `systemone export` runs THE SYSTEM SHALL list the closed
  requests that wait for their first vote, and WHEN `systemone import` runs THE
  SYSTEM SHALL accept the answers of a relay under its source, as an executor of
  the relay channel, ask no host vote next to them, and record the confidence
  and the source the endpoint gave.
- **SP-547~2**: WHEN `doctor` runs THE SYSTEM SHALL check Shallnot and the report schema it
  prints, the version control tool, the configuration, the harness files the
  project holds, the host store and the journal chain, and exit 2 when a check
  fails.
- **SP-548~1**: WHEN an honest host answers every request and the Statements of the
  discriminating-power and alignment modules are present THE SYSTEM SHALL ask
  only about the changed units those Statements do not justify, write one
  advisory Statement per spec with the dimension, unjustified change,
  unconstrained behaviour and clarity questions, and leave nothing pending.
- **SP-549~1**: WHEN no Statement of another module is present THE SYSTEM SHALL ask
  about every changed unit, raise no unconstrained behaviour question and record
  no `proof` input.
- **SP-550~1**: WHEN the host forges, breaks, omits or replays stale answers THE
  SYSTEM SHALL accept none that breaks the protocol, end a rejected ticket as
  `host_unresponsive` with no question made up for it, keep waiting for an
  omitted answer, and leave only valid advisory Statements without a verdict.
- **SP-551~1**: WHEN the host answers the tickets in any order THE SYSTEM SHALL write
  the same Statement.
- **SP-552~1**: WHEN recorded answers are replayed THE SYSTEM SHALL write
  byte-identical Statements in one command and fail hard on a request with no
  recording.

## Host execution

- **SP-780~1**: WHEN `mcp` or `hook` is run THE SYSTEM SHALL refuse a role, harness or
  hook it does not know with exit 1, report with exit 1 and never block when a hook
  cannot read its payload, leave a call that is not about the module's executors
  alone without opening any state, do nothing in a project that has no shared
  configuration file, deny a call of the module's own tools that it cannot check, and
  resolve the project root from the option, the harness's project directory or the
  working directory, with symbolic links resolved, so that the hook, the server and
  the tickets of one project share one host directory.
- **SP-781~1**: WHEN an executor fetches or answers through the executor server of the
  command line THE SYSTEM SHALL offer exactly `fetch_request` and `submit_response`,
  and `drain` only when the client declared sampling and the `host` section of the
  shared configuration turned it on for the module `completeness`; refuse a call
  without a capability while attestation is required, accept only the capability of
  a started agent bound to that one ticket, record the answer as a restricted
  subagent of its harness with the capability's evidence, and, when attestation is
  waived, journal the answer as restricted.
- **SP-782~1**: WHEN a host harness is selected THE SYSTEM SHALL ship the decider of
  `spec-proof-completeness` as its executor agents, name their types and the executor tools as the
  harness spells them, and map a reported type back to the role of the requests of
  the module.
- **SP-783~1**: WHEN the host files of the module are generated THE SYSTEM SHALL
  produce them from one definition, the same bytes on every run, and the committed
  trees of the plugin and the harness scaffolds SHALL equal what the generator
  produces.
- **SP-784~1**: WHEN the host files of the module are read against the formats each
  harness documents THE SYSTEM SHALL give the Claude Code plugin its manifest, its
  executor server, one agent per executor role holding exactly the two executor
  tools, its four hooks and a skill that pre-approves what its loop runs, give Codex
  and Copilot their agents, hooks and skill, and ship no agent of a role the module
  does not use.
- **SP-785~1**: WHEN `init` runs THE SYSTEM SHALL require a Shallnot whose report schema
  is 1.2 or later, write the module's entry of the shared configuration and ignore
  its local state, install the files of each selected harness (`auto`, `none` or a
  list of Claude Code, Codex and Copilot) by merging into every file another tool
  shares and writing the files it owns, plan no change on a second run, and leave
  Shallnot's configuration, its block of AGENTS.md and its hooks as they were.
- **SP-786~1**: WHEN `enable` or `disable` runs THE SYSTEM SHALL turn the proof on or off in the shared
  configuration and add or remove the module's block of AGENTS.md by line edits that
  keep every other byte, so that disabling restores what enabling changed, write no
  entry in `proofs.require` because Shallnot refuses `completeness` there and say so,
  work in a project with no `shallnot.yaml`, and refuse a layout it cannot extend
  without rewriting.
- **SP-787~1**: WHEN `doctor` runs THE SYSTEM SHALL check that the report schema Shallnot
  prints accepts version 1.2 and refuses 1.1, and report the check.
- **SP-788~1**: WHEN `host push` runs THE SYSTEM SHALL publish every answered ticket with
  the profile it is credited with and the marks against it as a tree under the ref
  `refs/spec-proof/host/completeness/<head>` of the head commit with git plumbing, replace a
  ref that exists, publish the ref to a remote when asked, touch neither the working
  tree nor the current branch, warn when a tracked file differs from the head commit,
  refuse a store whose journal chain is broken and publish nothing when nothing is
  answered.
- **SP-789~1**: WHEN `host pull --ci-run ID` runs THE SYSTEM SHALL download the artifact of
  the run with `gh`, refuse a download that fails or a bundle that is not one, and
  make tickets of the requests it holds once, with the votes and the queue metadata
  CI gave them.
- **SP-790~1**: WHEN `prove --ci` runs THE SYSTEM SHALL produce the Statements as CI,
  serve the tickets it is asking for from the bundle given with `--import-host` through
  the same checks as any answer, ignore what the bundle holds that it does not ask
  for, leave the other tickets pending, mark again what the pushing host marked,
  refuse a bundle of another version that left requests unanswered, leave the
  requests that remain where the CI job uploads them from, and refuse `--import-host`
  without `--ci` and `--ci` with `--record` or `--replay`.
- **SP-791~1**: WHEN a project is equipped with Shallnot and the module, edited and proved
  THE SYSTEM SHALL let a simulated Claude Code host answer every ticket through the
  real hooks and the real executor server until `prove` ends, journal the executors
  as started, fetched, accepted, attested and reported with no leak, exposure or
  conflict, and deny a main-session call of an executor tool.
- **SP-792~1**: WHEN a forger answers every ticket from the command line, or a host runs
  executors whose hooks never fire, THE SYSTEM SHALL accept no uncapped answer, give
  no verdict credit that rests on an answer a forger could have written, and have
  Shallnot refuse the turn or the check where the module is required.
- **SP-793~1**: WHEN the fake-host personas drive the module through its command line THE
  SYSTEM SHALL give each the verdict ADR 0005 asserts for it.

## End to end

- **SP-794~1**: WHEN the built module is initialized and enabled, a change is made and a
  simulated Claude Code host answers every ticket THE SYSTEM SHALL answer through its real
  hooks and executor server, journal each executor as started, fetched, accepted and
  attested with no leak, exposure or conflict, write the Statement, write no
  `proofs.require` entry, and deny and journal a main-session call of an executor tool.
- **SP-795~1**: WHEN executors run whose hooks never fire THE SYSTEM SHALL refuse their
  fetch and their answer at the executor server, accept no answer and keep every ticket
  pending with exit code 3.
- **SP-796~1**: WHEN a forger answers from the command line THE SYSTEM SHALL accept no
  answer without a capability, journal the attempt, and keep the tickets pending.

## Architecture

- **SP-553~1**: WHEN the package `internal/domain` is inspected THE SYSTEM SHALL
  import no adapter, `os`, `io/fs`, `os/exec` or `net`.
- **SP-554~1**: WHEN the package `internal/app` is inspected THE SYSTEM SHALL import
  no adapter.
- **SP-555~1**: WHEN an adapter is inspected THE SYSTEM SHALL import no other
  adapter.

## Thresholds per confidence source

- **SP-904~1**: WHEN `tau_adv` is set THE SYSTEM SHALL accept a bare number or a
  mapping of `default` and of confidence sources to numbers, as Shallnot's
  `proofs.thresholds` does, hold a closed answer to the threshold of the source of
  its confidence (`system-one:<model>@<endpoint digest>` for a relayed answer,
  `host-votes(<k>)` for k host votes), resolving the exact source, then its kind,
  then the default, refuse a value outside [0,1] and a key that names no source, and
  record the threshold the weakest answer was held to in
  `extensions.decision.threshold`.
