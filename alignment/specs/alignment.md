# alignment requirements

The requirements of the `alignment` module, which judges whether each test bound
to a criterion verifies what the criterion states, in Shallnot's Markdown spec
format. `shallnot gate` traces them to the Go tests of the module in CI.

## Binary

- **SP-101~1**: WHEN the `spec-proof-alignment` binary is run with the `version`
  argument THE SYSTEM SHALL print its version.

## Blind input

- **SP-300~1**: WHEN criteria are shown to a judge THE SYSTEM SHALL give each an
  opaque token that names no criterion, unique among them, and an order, both a
  pure function of the test and the criteria, so that identical work gives an
  identical request and another test gives another token set and order.
- **SP-301~1**: WHEN a masked item is checked THE SYSTEM SHALL refuse it when it
  holds a declared criterion reference or identifier, the marker word `verifies`
  or a run of three consecutive words of the test name, reading camel-case names
  word by word and ignoring the `test` prefix word.
- **SP-302~1**: WHEN a criterion statement is shown THE SYSTEM SHALL remove every
  declared reference and identifier from it, longer identifiers first.
- **SP-321~1**: WHEN a Python test is masked THE SYSTEM SHALL return the test and
  the module-level definitions it reaches, with the imports they use, under
  neutral names, without comments, docstrings or citation decorators.
- **SP-322~1**: WHEN the signatures of the code under a Python test are built THE
  SYSTEM SHALL return, without bodies, the functions and classes of the project
  that the test imports.
- **SP-323~1**: WHEN a Go test is masked THE SYSTEM SHALL return the top-level
  test, reduced to the bound subtest when its name is literal, with the
  package-level declarations it reaches and the imports they use, under neutral
  names, without comments or citation markers.
- **SP-324~1**: WHEN the signatures of the code under a Go test are built THE
  SYSTEM SHALL return, without bodies, the declarations of the project that the
  test references.
- **SP-325~1**: WHEN a masked input is built for any test name THE SYSTEM SHALL
  hold no declared criterion reference, no marker word and no run of three
  consecutive words of the test name.
- **SP-318~1**: WHEN a judge request is built THE SYSTEM SHALL show the criteria
  under tokens, the masked test and the signatures, name no criterion in any
  payload, and enumerate the tokens and NONE in a schema that ties NONE to the
  coverage NONE.
- **SP-319~1**: WHEN a closed request is built THE SYSTEM SHALL ask the criterion
  question over a group of at most 23 tokens and NONE and, in the final round,
  the coverage question over NONE, WEAK, PARTIAL and FULL, with a schema that
  requires each question to be answered once with one of its options.
- **SP-320~1**: WHEN a request is built THE SYSTEM SHALL give it the same identity
  for the same work and another identity when the test, the signatures, a
  statement or the tokens change, and a template digest that does not depend on
  the tokens of one test.

## Matrix

- **SP-303~1**: WHEN a discriminating-power Statement is read into pools THE
  SYSTEM SHALL keep as VALID only owner-attributed or judged, non-collateral
  mutants, as unattributed those of any grade, and as sentinels the sentinel
  mutants, count a kill only for an outcome killed, and leave out of a test's
  facts a Statement that does not list the test.
- **SP-304~1**: WHEN a test is classified THE SYSTEM SHALL answer UNDEFINED_OWN
  when its criterion has no VALID mutant, INERT when it kills no VALID or
  unattributed mutant, OFF_TARGET when it kills none of its criterion's, PARTIAL
  below `theta_own`, SPECIFIC at or above it with other kills at or below
  `theta_other`, and OVERBROAD otherwise.
- **SP-305~1**: WHEN a test is classified THE SYSTEM SHALL mark it contaminated
  when it kills a sentinel, and as pinning off-spec behaviour when its share of
  killed unattributed mutants exceeds `theta_none`, and SHALL never find a
  contaminated or pinning test eligible.

## Verdict

- **SP-307~1**: WHEN the votes of a judge request are aggregated THE SYSTEM SHALL
  take the criterion with the highest count over the tickets, ties to the higher
  mean confidence then to the smaller reference, the commonest coverage on it,
  and a share that counts a missing ticket against agreement.
- **SP-308~1**: WHEN a vote comes from an executor below the isolation floor or
  marked leaked, exposed or conflicting THE SYSTEM SHALL count it, when it would
  support the declared criterion, as an answer that verifies nothing.
- **SP-309~1**: WHEN a project has more than 23 criteria THE SYSTEM SHALL play the
  closed criterion question as groups of at most 23 plus NONE and a final round
  among the winners, take the minimum confidence over the rounds, recompute the
  plan from the recorded answers, and end for any number of criteria.
- **SP-310~1**: WHEN a test kills a sentinel THE SYSTEM SHALL decide UNDETERMINED
  with the reason `mutation_sensitive_test`, whatever else was said and before
  anything is asked.
- **SP-311~1**: WHEN the matrix is SPECIFIC or OVERBROAD and not pinning off-spec
  behaviour THE SYSTEM SHALL ask System One first and issue ALIGNED, corroborated
  by the matrix, when it names the declared criterion with a supporting coverage
  at a raw confidence at or above `tau_route`, and SHALL never issue a System One
  ALIGNED without such a matrix.
- **SP-312~1**: WHEN the judge decides THE SYSTEM SHALL issue MISALIGNED when its
  modal answer is another criterion, NONE or a weak coverage, UNDETERMINED when
  its modal share is below `modal_min`, and ALIGNED when it names the declared
  criterion with a supporting coverage, and SHALL wait for exactly one signal at
  a time.
- **SP-313~1**: WHEN the judge supports alignment against an OFF_TARGET, INERT,
  PARTIAL or off-spec-pinning matrix THE SYSTEM SHALL ask one more ticket and
  decide UNDETERMINED when it supports too and MISALIGNED when it does not.
- **SP-314~1**: WHEN ALIGNED is issued THE SYSTEM SHALL corroborate it by the
  matrix exactly when the matrix is eligible, and otherwise say `uncorroborated`,
  and SHALL decide UNDETERMINED instead when the policy forbids a judge to decide
  without a matrix.
- **SP-315~1**: WHEN the evidence is worsened THE SYSTEM SHALL never turn a verdict
  that is not ALIGNED into ALIGNED.
- **SP-316~1**: WHEN verdicts are audited THE SYSTEM SHALL select a share of them by
  the digest of the work, the same whatever the order, a smaller rate selecting a
  subset of a larger one, and replace an audited two-signal verdict by the
  judge's outcome.
- **SP-317~1**: WHEN the decision policy is built THE SYSTEM SHALL default to the
  parameters of the ADRs, refuse a threshold outside [0,1], a count below one and
  an unknown isolation level, and let every parameter reach the policy digest.

## Criteria and proofs read from the project

- **SP-326~1**: WHEN a Shallnot report of schema 1.2 or later is read THE SYSTEM
  SHALL give each active requirement with its statement, its statement digest and
  the tests bound to it, identified by name or by classname and name, and SHALL
  refuse a report below 1.2 or without a digest.
- **SP-327~1**: WHEN criteria are loaded THE SYSTEM SHALL run `shallnot gate
  --advisory --json-out` on the project, read the report it wrote, remove it, and
  parse no spec itself.
- **SP-328~1**: WHEN the discriminating-power Statements of a criterion are looked
  up THE SYSTEM SHALL list those of the verified set before those of the local
  cache, only those of the kind asked for, and none of another criterion.
- **SP-329~1**: WHEN a matrix is read from a discriminating-power Statement THE
  SYSTEM SHALL use it only when it is valid, states the criterion's current
  statement digest and lists the test, and its test and code inputs match the
  project, record the digest of its exact bytes as an input of role `proof`, and
  give another cache key when that Statement changes.
- **SP-331~1**: WHEN `prove --changed` runs THE SYSTEM SHALL skip a test whose
  inputs are unchanged since its Statement, and `prove --all` SHALL prove it again.

## Statements

- **SP-330~1**: WHEN a test is proved THE SYSTEM SHALL write one Statement per
  declared criterion, with the verdict as a judged claim, the executor with the
  weakest isolation of the answers it rests on and `blind`, the `corroboratedBy`
  evidence, `extensions.decision` and `extensions.matrix`, in RFC 8785 form,
  validated against the schemas before it is written.
- **SP-333~1**: WHEN the pending file is written THE SYSTEM SHALL list the criteria
  that wait for tickets as `awaiting_host`, those that wait for the
  discriminating-power proof as `awaiting_proof` and those whose host stopped
  answering as `host_unresponsive`, in the form the format fixes, and no file
  when nothing waits.
- **SP-345~1**: WHEN a Statement is written to the proof cache THE SYSTEM SHALL
  validate it against the embedded schemas first, refuse an invalid one, and
  write it atomically at `.spec-proof/cache/alignment/<ID>~<REV>/<cacheKey>.json`.
- **SP-346~1**: WHEN a decision is memoized THE SYSTEM SHALL return it for its key,
  treat an absent or unreadable entry as a miss and write atomically.
- **SP-354~1**: WHEN a Statement is validated against its schema THE SYSTEM SHALL
  accept a valid alignment Statement, refuse one that breaks its schema, and
  refuse a document that is not a Statement of the format before the schema.
- **SP-355~1**: WHEN the module's configuration is decoded THE SYSTEM SHALL read the
  decision parameters, keep the default of an absent one and refuse an unknown
  key or an invalid parameter.

## Host protocol

- **SP-340~1**: WHEN the host store of a project is opened THE SYSTEM SHALL keep
  `queue.json`, `requests/`, `tickets/`, `responses/`, a digest-chained
  `journal.jsonl` and a `secret` under
  `$XDG_STATE_HOME/spec-proof/alignment/<repo-digest>/host/`, with directories
  private to the user and the secret readable by the user only, and SHALL derive
  the repository digest from the absolute path of the project root.
- **SP-341~1**: WHEN the filesystem store is tested THE SYSTEM SHALL satisfy the
  shared Store contract suite of the host protocol.
- **SP-342~1**: WHEN a store is opened again THE SYSTEM SHALL return the requests,
  entries, responses, journal chain and secret it held, and SHALL write each
  record atomically so that a reader never sees a partial file.
- **SP-343~1**: WHEN a request is recorded in the queue THE SYSTEM SHALL keep the
  wave of its first record, merge the criteria it serves without duplicates and
  list the items ordered by request id, and WHEN the wave advances THE SYSTEM
  SHALL return consecutive numbers starting at one.
- **SP-344~1**: WHEN the state and cache locations are resolved THE SYSTEM SHALL
  use `$XDG_STATE_HOME` and `$XDG_CACHE_HOME` when they are absolute paths and
  the defaults under the home directory otherwise.
- **SP-347~1**: WHEN an executor's output is checked against a response schema THE
  SYSTEM SHALL accept it exactly when the schema holds and list every violation
  otherwise.
- **SP-348~1**: WHEN a proof asks the host for an answer THE SYSTEM SHALL emit one
  ticket per vote, record the request in the queue, reissue a rejected or
  expired ticket with a new nonce, abandon it as unresponsive after the last
  emission, read a vote only from its accepted, validated or consumed state,
  move accepted votes to validated and validated ones to consumed, supersede
  emitted tickets nobody asked for in the wave, and list the tickets still
  waiting.
- **SP-349~1**: WHEN tickets are listed THE SYSTEM SHALL list only the tickets in
  state `emitted`, with the role and the view an executor sees, and WHEN a ticket
  is shown THE SYSTEM SHALL move it to `fetched` and refuse another emission of
  it.
- **SP-350~1**: WHEN an answer is submitted THE SYSTEM SHALL accept it only for the
  ticket and the nonce of the issue after the response schema holds, allow one
  correction before it rejects, refuse a different second answer as a
  resubmission conflict, record a submission without a capability as exposed and
  an executor profile that cannot claim more than inline, and keep the profile
  and the exposure out of replayed and relayed answers.
- **SP-351~1**: WHEN recorded answers are replayed THE SYSTEM SHALL answer every
  pending ticket from the recording of its request through the same checks and
  fail with a miss when a request has no recording, and WHEN answers are recorded
  THE SYSTEM SHALL keep each output and its executor under the request id.
- **SP-352~1**: WHEN a command is run THE SYSTEM SHALL capture its output within a
  limit and report its exit status, a non-zero status being a result.
- **SP-353~1**: WHEN a command exceeds its deadline or its context ends THE SYSTEM
  SHALL kill its whole process group and report the timeout.

## The prove use case

- **SP-334~1**: WHEN `prove` runs THE SYSTEM SHALL exit 3 while tickets wait for the
  host, 1 when a test is not ALIGNED or a proof is awaited, 0 otherwise, advance
  one wave per run, give the same requests when run again, and refuse `--resume`
  when no request was ever emitted.
- **SP-335~1**: WHEN `prove` is given conflicting flags or unexpected arguments THE
  SYSTEM SHALL fail without acting.
- **SP-336~1**: WHEN `request list|show|submit` runs THE SYSTEM SHALL list, show and
  accept answers through the same checks as the host protocol, and a refused
  answer SHALL exit as a refusal, not a tool failure.
- **SP-337~1**: WHEN `systemone export` runs THE SYSTEM SHALL list the closed
  requests that wait for their first vote, and WHEN `systemone import` runs THE
  SYSTEM SHALL accept the answers of a relay under its source, as an executor of
  the relay channel.
- **SP-338~1**: WHEN `doctor` runs THE SYSTEM SHALL check Shallnot, the
  configuration, the host store and the journal chain, and exit 2 when a check
  fails.
- **SP-339~1**: WHEN an honest host answers every request and a discriminating-power
  Statement is present THE SYSTEM SHALL write, for each test, a Statement whose
  verdict agrees with the matrix and the judge, corroborated when the matrix is
  eligible, byte-identical when the run is replayed from its recording.
- **SP-356~1**: WHEN no discriminating-power Statement is present THE SYSTEM SHALL
  issue ALIGNED only from the judge, uncorroborated, and never from System One.
- **SP-357~1**: WHEN the host forges answers THE SYSTEM SHALL never issue a
  corroborated ALIGNED verdict for a test whose matrix is negative or whose
  discriminating-power Statement is absent.
- **SP-358~1**: WHEN a test needs more tickets than `max_reasoning_calls` THE
  SYSTEM SHALL decide UNDETERMINED with the reason `budget_exhausted`, and WHEN
  its tickets are abandoned THE SYSTEM SHALL decide UNDETERMINED with the reason
  `host_unresponsive`.
- **SP-359~1**: WHEN the discriminating-power module still waits for a criterion and
  no usable Statement exists THE SYSTEM SHALL not write the Statement and SHALL
  list the criterion as `awaiting_proof`.
- **SP-360~1**: WHEN a test cannot be masked or its masked input leaks THE SYSTEM
  SHALL ask no judge and decide UNDETERMINED with the reason that says why.
- **SP-361~1**: WHEN System One relays the closed answers THE SYSTEM SHALL use them
  with the confidence the endpoint gave and no other host vote, and WHEN host
  deciders answer it SHALL take the modal share of `decide_k` votes.
- **SP-362~1**: WHEN the shadow audit selects a two-signal verdict THE SYSTEM SHALL
  ask the judge too and let its outcome replace the verdict.

## Architecture

- **SP-365~1**: WHEN the package `internal/domain` is inspected THE SYSTEM SHALL
  import no adapter, `os`, `io/fs`, `os/exec` or `net`.
- **SP-366~1**: WHEN the package `internal/app` is inspected THE SYSTEM SHALL import
  no adapter.
- **SP-367~1**: WHEN an adapter is inspected THE SYSTEM SHALL import no other
  adapter.
