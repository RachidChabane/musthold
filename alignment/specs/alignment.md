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
- **SP-301~2**: WHEN a masked item is checked THE SYSTEM SHALL refuse it when it
  holds a declared criterion reference or identifier, the marker word `verifies`
  or a run of three consecutive words of the test name, reading camel-case names
  word by word, ignoring the `test` prefix word and the ids of the parameters a
  parametrized run appends to the name in brackets.
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
  evidence, `extensions.decision`, which names the deciders and the confidence
  source, and `extensions.matrix`, in RFC 8785 form, validated against the
  schemas before it is written.
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
- **SP-337~2**: WHEN `systemone export` runs THE SYSTEM SHALL list the closed
  requests that wait for their first vote, and WHEN `systemone import` runs THE
  SYSTEM SHALL accept the answers of a relay under its source `system-one:<model>@<endpoint digest>`, which `confidenceSource` records as given, and refuse another form of source, as an executor of
  the relay channel.
- **SP-338~2**: WHEN `doctor` runs THE SYSTEM SHALL check Shallnot and the report schema it
  prints, the configuration, the harness files the project holds, the host store
  and the journal chain, and exit 2 when a check fails.
- **SP-339~1**: WHEN an honest host answers every request and a discriminating-power
  Statement is present THE SYSTEM SHALL write, for each test, a Statement whose
  verdict agrees with the matrix and the judge, corroborated when the matrix is
  eligible, byte-identical when the run is replayed from its recording.
- **SP-356~1**: WHEN no discriminating-power Statement is present THE SYSTEM SHALL
  issue ALIGNED only from the judge, uncorroborated, and never from System One.
- **SP-357~1**: WHEN the host forges answers THE SYSTEM SHALL never issue a
  corroborated ALIGNED verdict for a test whose matrix is negative or whose
  discriminating-power Statement is absent.
- **SP-358~1**: WHEN a test needs more judge tickets than `max_reasoning_calls` THE
  SYSTEM SHALL decide UNDETERMINED with the reason `budget_exhausted`, leaving the
  closed questions outside the budget, and WHEN its tickets are abandoned THE
  SYSTEM SHALL decide UNDETERMINED with the reason `host_unresponsive`.
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
- **SP-363~1**: WHEN the host answers the tickets in any order THE SYSTEM SHALL write
  the same Statements.

## Host execution

- **SP-740~1**: WHEN `mcp` or `hook` is run THE SYSTEM SHALL refuse a role, harness or
  hook it does not know with exit 1, report with exit 1 and never block when a hook
  cannot read its payload, leave a call that is not about the module's executors
  alone without opening any state, do nothing in a project that has no shared
  configuration file, deny a call of the module's own tools that it cannot check, and
  resolve the project root from the option, the harness's project directory or the
  working directory, with symbolic links resolved, so that the hook, the server and
  the tickets of one project share one host directory.
- **SP-741~1**: WHEN an executor fetches or answers through the executor server of the
  command line THE SYSTEM SHALL offer exactly `fetch_request` and `submit_response`,
  and `drain` only when the client declared sampling and the `host` section of the
  shared configuration turned it on for the module `alignment`; refuse a call
  without a capability while attestation is required, accept only the capability of
  a started agent bound to that one ticket, record the answer as a restricted
  subagent of its harness with the capability's evidence, and, when attestation is
  waived, journal the answer as restricted.
- **SP-742~1**: WHEN a host harness is selected THE SYSTEM SHALL ship the judge and the decider of
  `spec-proof-alignment` as its executor agents, name their types and the executor tools as the
  harness spells them, and map a reported type back to the role of the requests of
  the module.
- **SP-743~1**: WHEN the host files of the module are generated THE SYSTEM SHALL
  produce them from one definition, the same bytes on every run, and the committed
  trees of the plugin and the harness scaffolds SHALL equal what the generator
  produces.
- **SP-744~1**: WHEN the host files of the module are read against the formats each
  harness documents THE SYSTEM SHALL give the Claude Code plugin its manifest, its
  executor server, one agent per executor role holding exactly the two executor
  tools, its four hooks and a skill that pre-approves what its loop runs, give Codex
  and Copilot their agents, hooks and skill, and ship no agent of a role the module
  does not use.
- **SP-745~2**: WHEN `init` runs THE SYSTEM SHALL require a Shallnot whose report schema
  is 1.2 or later, write the module's entry of the shared configuration with the
  commented example of the operations block and ignore its local state (the
  statement cache, the verified set and the pending files), install the files of each selected harness (`auto`, `none` or a
  list of Claude Code, Codex and Copilot) by merging into every file another tool
  shares and writing the files it owns, plan no change on a second run, and leave
  Shallnot's configuration, its block of AGENTS.md and its hooks as they were.
- **SP-746~2**: WHEN `enable` or `disable` runs THE SYSTEM SHALL turn the proof on or off in the shared
  configuration, add or remove `alignment` in `proofs.require` of `shallnot.yaml` and
  the module's block of AGENTS.md by line edits that keep every other byte, set
  `proofs.trust` to `local` and list the source roots of the project in
  `proofs.code` when `shallnot.yaml` sets neither, so that disabling removes what
  enabling added to `proofs.require` and to AGENTS.md and leaves the trust and the
  code, refuse a project with no `shallnot.yaml`, and refuse a layout it cannot
  extend without rewriting.
- **SP-747~1**: WHEN `doctor` runs THE SYSTEM SHALL check that the report schema Shallnot
  prints accepts version 1.2 and refuses 1.1, and report the check.
- **SP-748~1**: WHEN `host push` runs THE SYSTEM SHALL publish every answered ticket with
  the profile it is credited with and the marks against it as a tree under the ref
  `refs/spec-proof/host/alignment/<head>` of the head commit with git plumbing, replace a
  ref that exists, publish the ref to a remote when asked, touch neither the working
  tree nor the current branch, warn when a tracked file differs from the head commit,
  refuse a store whose journal chain is broken and publish nothing when nothing is
  answered.
- **SP-749~1**: WHEN `host pull --ci-run ID` runs THE SYSTEM SHALL download the artifact of
  the run with `gh`, refuse a download that fails or a bundle that is not one, and
  make tickets of the requests it holds once, with the votes and the queue metadata
  CI gave them.
- **SP-750~2**: WHEN `prove --ci` runs THE SYSTEM SHALL produce the Statements as CI,
  serve the tickets it is asking for from the bundle given with `--import-host` through
  the same checks as any answer, ignore what the bundle holds that it does not ask
  for, leave the other tickets pending, mark again what the pushing host marked,
  refuse a bundle of another version that left requests unanswered, leave the
  requests that remain in `.spec-proof/ci-issued/alignment`, where the CI job uploads them from, and refuse `--import-host`
  without `--ci` and `--ci` with `--record` or `--replay`.
- **SP-751~1**: WHEN a project is equipped with Shallnot and the module, edited and proved
  THE SYSTEM SHALL let a simulated Claude Code host answer every ticket through the
  real hooks and the real executor server until `prove` ends, journal the executors
  as started, fetched, accepted, attested and reported with no leak, exposure or
  conflict, and deny a main-session call of an executor tool.
- **SP-752~1**: WHEN a forger answers every ticket from the command line, or a host runs
  executors whose hooks never fire, THE SYSTEM SHALL accept no uncapped answer, give
  no verdict credit that rests on an answer a forger could have written, and have
  Shallnot refuse the turn or the check where the module is required.
- **SP-753~1**: WHEN the fake-host personas drive the module through its command line THE
  SYSTEM SHALL give each the verdict ADR 0005 asserts for it.

## Contracts with the format and the gate

- **SP-409~1**: WHEN a decision is recorded THE SYSTEM SHALL name where its confidence
  came from in `extensions.decision.confidenceSource` with the mapping that derived
  it: `system-one:<model>@<endpoint digest>` for a relayed answer, `host-votes(<k>)`
  with the modal share for host deciders and judges, and `execution` for a verdict
  the module fixed itself, and SHALL name only the signals that decided in
  `deciders`.
- **SP-410~2**: WHEN a Statement lists its inputs THE SYSTEM SHALL name the `spec`
  input `ID~REV` with the statement digest, list the file of its own test among the
  `test_source` inputs, name every file input by its path relative to the directory
  holding `shallnot.yaml` with the SHA-256 of its exact bytes, and consume the discriminating-power
  Statement of its criterion as the one `proof` input, named by the kind, carrying
  the criterion in its annotations and the SHA-256 of the Statement file's exact
  bytes, while the Statements of the other criteria the test is bound to reach the
  cache key through one `matrix-evidence` input.
- **SP-411~1**: WHEN a closed request is built THE SYSTEM SHALL build its question
  set and its response schema through the host protocol's closed question type: a
  choice question over the group and NONE whose options carry the statements, and
  in the final round a score question whose levels carry what each coverage means,
  and WHEN a relay imports the response of a System One endpoint THE SYSTEM SHALL
  translate it with the question set of the ticket.

## Architecture

- **SP-365~1**: WHEN the package `internal/domain` is inspected THE SYSTEM SHALL
  import no adapter, `os`, `io/fs`, `os/exec` or `net`.
- **SP-366~1**: WHEN the package `internal/app` is inspected THE SYSTEM SHALL import
  no adapter.
- **SP-367~1**: WHEN an adapter is inspected THE SYSTEM SHALL import no other
  adapter.

## Thresholds per confidence source

- **SP-903~1**: WHEN `tau_route` or `modal_min` is set THE SYSTEM SHALL accept a bare
  number or a mapping of `default` and of confidence sources to numbers, as
  Shallnot's `proofs.thresholds` does, hold a System One answer to the threshold of
  the source of its confidence (`system-one:<model>@<endpoint digest>`, or
  `host-votes(<decide_k>)` when host votes stand in for the endpoint) and the judges
  to that of `host-votes(<align_k>)`, resolving the exact source, then its kind,
  then the default, refuse a value outside [0,1] and a key that names no source, and
  record the threshold the decision was held to in `extensions.decision.threshold`.

## Parametrized tests

- **SP-905~1**: WHEN a parametrized test is masked THE SYSTEM SHALL keep its
  parameter values in the masked unit and SHALL NOT refuse it for the text of the id
  its run carries, as in `test_total[10-200-180.0]`, while a run of three
  consecutive words of the name of the function keeps refusing it.

## Rewriting source text

- **SP-1215~1**: WHEN a masker rewrites source text by spans THE SYSTEM SHALL apply
  the replacements that fall inside the span asked for, drop one nested in a longer
  one, remove the line a removed comment leaves empty and nothing else of it, and
  strip trailing blanks and collapse blank lines.
- **SP-1216~1**: WHEN the name a runner reports for a test is read THE SYSTEM SHALL
  take out its citation tags and unify the separators between the titles it joins,
  and match the title of a test against it with every placeholder standing for any
  text.

## JavaScript and TypeScript tests

- **SP-1200~1**: WHEN a JavaScript or TypeScript test of Jest or Vitest is masked THE
  SYSTEM SHALL return the test inside the suites it is nested in, every title
  replaced by a neutral one, with the hooks and the setup calls of those suites, the
  declarations the test reaches and the imports they use, under neutral names,
  without comments or citation tags.
- **SP-1201~1**: WHEN the signatures of the code under a JavaScript or TypeScript
  test are built THE SYSTEM SHALL return, without bodies, values, comments or
  private members, the declarations that the project modules the test imports or
  requires export, following re-exports, and nothing for a module the project does
  not hold.
- **SP-1202~1**: WHEN a Jest or Vitest test is bound by the name its runner reports
  THE SYSTEM SHALL find it by the titles of its suites and its own title joined
  with spaces or with the separator of the reporter, whatever the citation tags in
  them and the values a table row gives the placeholders, preferring the most
  specific title and then the line of the report.
- **SP-1203~1**: WHEN a table-driven test is masked THE SYSTEM SHALL keep its table
  and its parameters, whether the table is an array or a tagged template, and SHALL
  NOT refuse it for the values the reported name holds.
- **SP-1204~1**: WHEN a masked input is built for any title of a JavaScript or
  TypeScript test THE SYSTEM SHALL hold no declared criterion reference, no marker
  word and no run of three consecutive words of the titles, reading the whole name
  the runner reports without its citation tags and separators.
- **SP-1205~1**: WHEN a file is offered to the JavaScript and TypeScript masker THE
  SYSTEM SHALL serve the files of JavaScript, JSX, TypeScript and TSX and refuse
  every other file.

## Java and Kotlin tests

- **SP-1210~1**: WHEN a JUnit 5 test written in Java is masked THE SYSTEM SHALL
  return one class that holds the test and the members of its enclosing classes that
  run for every test or that it reaches, with the types of the file it reaches and
  the imports they use, under neutral names, without comments, display names, name
  patterns or citation tags.
- **SP-1211~1**: WHEN the signatures of the code under a Java test are built THE
  SYSTEM SHALL return, without bodies, annotations, initial values or private
  members, the classes of the project that the test imports, statically imports or
  names from its own package or a package it imports whole.
- **SP-1212~1**: WHEN a JUnit 5 test is bound by the name its runner reports THE
  SYSTEM SHALL find it by its method name with or without its signature, its display
  name, the expansion of the name pattern of a parametrized or repeated test or the
  default name of an invocation, telling tests of one name apart by the class the
  report names, as a binary name or as display names, and then by the line.
- **SP-1213~1**: WHEN a masked input is built for any name of a Java test THE
  SYSTEM SHALL hold no declared criterion reference, no marker word and no run of
  three consecutive words of its method name, display names or name pattern.
- **SP-1214~1**: WHEN a file is offered to the Java masker THE SYSTEM SHALL serve the
  files of Java and refuse every other file.
- **SP-1220~1**: WHEN a JUnit 5 test written in Kotlin is masked THE SYSTEM SHALL
  return one class that holds the test and the members of its enclosing classes that
  run for every test or that it reaches, with the declarations of the file it reaches
  and the imports they use, under neutral names, without comments, display names,
  name patterns or citation tags, however many annotations precede a class.
- **SP-1221~1**: WHEN the signatures of the code under a Kotlin test are built THE
  SYSTEM SHALL return, without bodies, annotations, initial values or private
  members, the classes, objects and top-level declarations of the project that the
  test imports or names from its own package or a package it imports whole.
- **SP-1222~1**: WHEN a Kotlin test is bound by the name its runner reports THE
  SYSTEM SHALL find it as it finds a Java test, reading the name of a function written
  between backticks as the sentence it is.
- **SP-1223~1**: WHEN a masked input is built for any name of a Kotlin test THE
  SYSTEM SHALL hold no declared criterion reference, no marker word and no run of
  three consecutive words of its function name, display names or name pattern.
- **SP-1224~1**: WHEN a file is offered to the Kotlin masker THE SYSTEM SHALL serve
  the files of Kotlin and refuse every other file.

## Languages in the command line

- **SP-1230~1**: WHEN the maskers of the command line are listed THE SYSTEM SHALL
  serve the test files of Python, Go, JavaScript, TypeScript, Java and Kotlin, each by
  one masker.
- **SP-1240~1**: WHEN an honest host answers every request of a TypeScript project
  THE SYSTEM SHALL write, for each test, the Statement whose verdict agrees with the
  matrix and the judge, as it does for a Python project.
- **SP-1241~1**: WHEN a forger answers every request of a TypeScript project THE
  SYSTEM SHALL give no ALIGNED to a test whose matrix refutes it.
- **SP-1242~1**: WHEN an honest host answers every request of a Java project THE
  SYSTEM SHALL write, for each test, the Statement whose verdict agrees with the
  matrix and the judge, as it does for a Python project.
- **SP-1243~1**: WHEN a forger answers every request of a Java project THE SYSTEM
  SHALL give no ALIGNED to a test whose matrix refutes it.

## Help and output lines

- **SP-1620~1**: WHEN the binary is run with `-h`, `--help` or `help`, a command
  with `-h` or `--help`, or `help` followed by a command THE SYSTEM SHALL print the
  commands of the binary, or the flags and the synopsis of that command, on the
  output stream and exit 0; WHEN a command that takes a verb is asked for help
  THE SYSTEM SHALL list its verbs; WHEN a verb of `request`, `systemone` or `host`
  is asked for help THE SYSTEM SHALL list exactly the flags that verb defines and
  refuse, as a usage error that prints the usage of the verb, a flag another verb
  owns.
- **SP-1621~1**: WHEN `prove` prints a line of a test that has no verdict yet THE SYSTEM SHALL print no
  trailing space on it.

## Portable plugin

- **SP-1711~1**: WHEN the portable plugin of the module is generated THE SYSTEM SHALL
  give it the manifest, the server of `spec-proof-alignment`, the fulfil skill and the agents and
  hooks of Copilot CLI that the shared generator produces, the root server naming
  no harness and the agents' servers named apart from it, and the committed tree
  SHALL equal what the generator produces; the Copilot tree under `plugin/` is the
  portable one.
- **SP-1714~1**: WHEN `mcp --role executor` runs with no `--harness` THE SYSTEM
  SHALL serve the portable harness: the two executor tools with no `drain`, the
  project learned from the client's roots, and a refusal that says so for a
  directory with no project of the module.

## Release version

- **SP-1852~1**: WHEN `spec-proof-alignment` is run with the lone argument `--version` THE SYSTEM
  SHALL print the bare version, the one a release build linked with
  `-X main.version`, else the one `go install` recorded, else `dev`, and the
  `version` command SHALL print the binary name, that version and the linked
  commit when the build has one.

## Claude Code classification

- **SP-1911~1**: WHEN `systemone classify-plan` runs with a model, or with voters
  whose counts sum to five, THE SYSTEM SHALL list for each closed request that
  waits for its first vote its ticket and nonce, the confidence source
  `host-classify(5,rot1):<voters>`, the digest of its question set and, for each
  question and each of the five calls, the model that answers it, the text with
  the options in the planned order, the labels in that order and the seed input
  of the order; and SHALL refuse a call that names neither or both of a model and
  voters, and voters that do not make a valid source.
- **SP-1931~1**: WHEN the committed Claude plugin of the module is read THE SYSTEM
  SHALL ship the hooks module that carries the classifier calls of the host,
  naming the binary of the module, and the validator of Claude Code SHALL accept
  the plugin.
