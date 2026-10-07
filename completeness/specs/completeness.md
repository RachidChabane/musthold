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

- **SP-504~1**: WHEN the units of a changed file are compared with those before
  the change THE SYSTEM SHALL give one hunk per function, method, package-level
  variable or constant that is new or whose code differs, and none for a unit
  whose code is the same, whatever its formatting, comments and docstrings, and
  none for a unit the change deleted.
- **SP-508~1**: WHEN entry names are closed over the units of a project THE SYSTEM
  SHALL reach the units they designate and, transitively, those the reached units
  mention, across files and through assignments, reach the constructor of a
  class named as an entry, end on a cycle, and reach exactly the union of what
  each entry reaches.
- **SP-509~1**: WHEN a changed unit is justified THE SYSTEM SHALL name the
  requirements whose example probes or ALIGNED tests may execute it, sorted and
  without repeats, designate the functions of a test from its label and those of
  an example from its operation binding, and justify nothing without evidence.

## Questions

- **SP-510~1**: WHEN a changed unit that no example probe and no ALIGNED test may
  execute is checked THE SYSTEM SHALL ask one choice question over the criteria
  of the project and NONE, as a tournament above 23 criteria, and raise an
  `unjustified_change` question when the tournament ends on NONE at a
  confidence at or above `tau_adv`, and none otherwise.
- **SP-512~1**: WHEN the testability of the requirements of a spec is checked THE
  SYSTEM SHALL ask one choice question per requirement, answered TESTABLE,
  AMBIGUOUS or UNTESTABLE, in groups of at most 64, and raise a `clarity`
  question for each requirement answered AMBIGUOUS or UNTESTABLE at any
  confidence.
- **SP-518~1**: WHEN a question is written THE SYSTEM SHALL build its text from a
  deterministic template of its signal, name the requirements it concerns, merge
  identical questions with the union of their requirements, and return the
  questions sorted and the same for any order of the signals.

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
- **SP-537~1**: WHEN a proof asks the host for an answer THE SYSTEM SHALL emit one
  ticket per vote, record the request in the queue, reissue a rejected or
  expired ticket with a new nonce, abandon it as unresponsive after the last
  emission, read a vote only from its accepted, validated or consumed state,
  move accepted votes to validated and validated ones to consumed, supersede
  emitted tickets nobody asked for in the wave, and list the tickets still
  waiting.
- **SP-538~1**: WHEN tickets are listed THE SYSTEM SHALL list only the tickets in
  state `emitted`, with the role and the view an executor sees, and WHEN a ticket
  is shown THE SYSTEM SHALL move it to `fetched` and refuse another emission of
  it.
- **SP-539~1**: WHEN an answer is submitted THE SYSTEM SHALL accept it only for the
  ticket and the nonce of the issue after the response schema holds, allow one
  correction before it rejects, refuse a different second answer as a
  resubmission conflict, record a submission without a capability as exposed and
  an executor profile that cannot claim more than inline, and keep the profile
  and the exposure out of replayed and relayed answers.
- **SP-540~1**: WHEN recorded answers are replayed THE SYSTEM SHALL answer every
  pending ticket from the recording of its request through the same checks and
  fail with a miss when a request has no recording, and WHEN answers are recorded
  THE SYSTEM SHALL keep each output and its executor under the request id.
- **SP-541~1**: WHEN a command is run THE SYSTEM SHALL capture its output within a
  limit and report its exit status, a non-zero status being a result.
- **SP-542~1**: WHEN a command exceeds its deadline or its context ends THE SYSTEM
  SHALL kill its whole process group and report the timeout.

## Architecture

- **SP-553~1**: WHEN the package `internal/domain` is inspected THE SYSTEM SHALL
  import no adapter, `os`, `io/fs`, `os/exec` or `net`.
- **SP-554~1**: WHEN the package `internal/app` is inspected THE SYSTEM SHALL import
  no adapter.
- **SP-555~1**: WHEN an adapter is inspected THE SYSTEM SHALL import no other
  adapter.
