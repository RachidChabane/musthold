# discpower requirements

The requirements of the `discpower` module, which measures the discriminating
power of tests by executing them against mutants, in Shallnot's Markdown spec
format. `shallnot gate` traces them to the Go tests of the module in CI.

## Schemas

- **SP-110~1**: WHEN the embedded schemas are loaded THE SYSTEM SHALL compile
  each from its embedded bytes.
- **SP-146~1**: WHEN a host-protocol request, ticket or response is validated
  against its schema THE SYSTEM SHALL accept it exactly when the
  schema-expressible domain rules of the host protocol hold.
- **SP-23~1**: WHEN a statement is validated against its schema THE SYSTEM SHALL
  require a confidence for a judged claim and forbid one for an executed claim.
- **SP-25~1**: WHEN a golden vector is validated against the schemas THE SYSTEM
  SHALL give the schema result its manifest states.
- **SP-26~1**: WHEN a generated predicate is checked THE SYSTEM SHALL accept it
  against the schemas exactly when the schema-expressible domain invariants
  hold.
- **SP-27~1**: WHEN a schema references a resource that is not embedded THE
  SYSTEM SHALL fail to compile it rather than fetch it.
- **SP-28~1**: WHEN a document is not JSON, or its predicate type is outside the
  format, THE SYSTEM SHALL refuse it before schema validation.
- **SP-29~1**: WHEN a spec path is judged by the subject rule THE SYSTEM SHALL
  agree between the Go check and the schema pattern.

## Domain

- **SP-30~1**: WHEN a mutant run is classified THE SYSTEM SHALL answer `killed`
  when a bound test failed, `survived` when every bound test passed, and `error`
  when the build check failed, collection failed, the run timed out, or a bound
  test is absent or skipped.
- **SP-31~1**: WHEN a span is built THE SYSTEM SHALL refuse negative or inverted
  bounds and slice only inside the source.
- **SP-35~1**: WHEN project settings are created THE SYSTEM SHALL disable every
  proof, and refuse an unknown proof or ecosystem.

## Mutant execution

- **SP-60~1**: WHEN an edit names a definition by its qualified name THE SYSTEM
  SHALL replace exactly that definition's byte span with the new source, apply
  the edits of one file in order, and refuse an unknown or ambiguous qualname.
- **SP-61~1**: WHEN a splice is done THE SYSTEM SHALL re-parse the file and
  accept it only when the target definition's text equals the new source and
  every definition outside the target is byte-identical.
- **SP-62~1**: WHEN an edit names a path THE SYSTEM SHALL refuse it unless it is
  a clean relative POSIX path inside a source root and not a test file of the
  ecosystem.
- **SP-63~1**: WHEN a definition is spliced with its own source THE SYSTEM SHALL
  leave the file unchanged.
- **SP-64~1**: WHEN a test is named by a report and by a results file THE SYSTEM
  SHALL give both the same identity, and WHEN one test is reported several times
  THE SYSTEM SHALL keep its most severe outcome.
- **SP-65~1**: WHEN a bound test is mapped to a definition by its source line
  THE SYSTEM SHALL choose the innermost definition containing the line, else the
  nearest definition that follows it.
- **SP-66~1**: WHEN a command is run THE SYSTEM SHALL capture its output within
  a limit and report its exit status, a non-zero status being a result, and a
  suite run SHALL read the per-test outcomes its reporter wrote.
- **SP-67~1**: WHEN a command exceeds its deadline or its context ends THE
  SYSTEM SHALL kill its whole process group and report the timeout.
- **SP-68~1**: WHEN a reference workspace is prepared THE SYSTEM SHALL copy the
  project once under the temporary directory, outside the repository, leaving
  out version control data and the excluded directories of the ecosystem.
- **SP-69~1**: WHEN a mutant workspace is created THE SYSTEM SHALL clone it from
  the built reference with the first working strategy of copy-on-write clone and
  plain copy, link the dependency directories when copying, and leave the
  reference unchanged.
- **SP-70~1**: WHEN a file is written into a workspace THE SYSTEM SHALL give it
  a modification time later than every file of the workspace outside the
  dependency directories.
- **SP-71~1**: WHEN JUnit XML written by pytest, jest-junit, Vitest, Surefire,
  Gradle or gotestsum is read THE SYSTEM SHALL give each test its outcome, with
  an error element counting as a failure only where the test body raises it.
- **SP-72~1**: WHEN a Go source is located THE SYSTEM SHALL return every
  function and method declaration with its byte span, lines and qualified name,
  and refuse a source that does not parse.
- **SP-73~1**: WHEN a project tree is inspected THE SYSTEM SHALL choose the
  ecosystem strategies that recognize it, refuse two strategies of one name, and
  add a strategy without a change to the registry.
- **SP-74~1**: WHEN a pytest project is executed THE SYSTEM SHALL resolve the
  interpreter from the configuration, the virtual environment, `uv run python`
  for a uv project, then `python3`, compile and import the changed modules,
  select bound tests by node id, and run `python -m pytest` with bytecode
  writing disabled.
- **SP-75~1**: WHEN a Go module is executed THE SYSTEM SHALL compile the test
  binaries of the changed packages, select bound tests by an anchored `-run`
  expression over their subtest paths, and run `go tool gotestsum` with a JUnit
  file.
- **SP-76~1**: WHEN a mutant that a bound test fails is executed THE SYSTEM
  SHALL classify it killed, in every ecosystem.
- **SP-77~1**: WHEN a behaviour-preserving mutant is executed THE SYSTEM SHALL
  classify it survived, in every ecosystem.
- **SP-78~1**: WHEN a mutant that does not parse or compile is executed THE
  SYSTEM SHALL classify it error with diagnostics, in every ecosystem.
- **SP-79~1**: WHEN a mutant is executed against the tests bound to a criterion
  THE SYSTEM SHALL run only those tests.
- **SP-80~1**: WHEN a mutant workspace is cloned from an already-built reference
  THE SYSTEM SHALL still kill a mutant the tests fail.
- **SP-81~1**: WHEN a bound test is display-named or parametrized THE SYSTEM
  SHALL select it and attribute its outcome to the criterion it is bound to.
- **SP-82~1**: WHEN a mutant is executed THE SYSTEM SHALL clone the reference,
  splice, build-check, select, run and classify, report a splice or build
  failure as the outcome error, refuse a path the policy forbids before any
  clone, and release the mutant workspace.

## Architecture

- **SP-36~1**: WHEN the package `internal/domain` is inspected THE SYSTEM SHALL
  import no adapter, `os`, `io/fs`, `os/exec` or `net`.
- **SP-37~1**: WHEN the package `internal/app` is inspected THE SYSTEM SHALL
  import no adapter.
- **SP-38~1**: WHEN an adapter is inspected THE SYSTEM SHALL import no other
  adapter.
- **SP-40~1**: WHEN the `spec-proof-dp` binary is run with the `version`
  argument THE SYSTEM SHALL print its version.

## Adapters

- **SP-41~1**: WHEN a Python, Java, Kotlin, TypeScript, TSX or JavaScript source
  is located THE SYSTEM SHALL return every function and method definition with
  its byte span.
- **SP-42~1**: WHEN a definition is nested in a class or a function THE SYSTEM
  SHALL name it by its qualified path, such as `Class.method`.
- **SP-43~1**: WHEN a program links one grammar through the build-tag subset THE
  SYSTEM SHALL stay below the recorded size budget and parse that language, and
  a program linking the full registry SHALL be larger.
- **SP-44~1**: WHEN a file has an extension no grammar covers THE SYSTEM SHALL
  refuse it.
- **SP-45~1**: WHEN a project configuration is decoded from YAML THE SYSTEM
  SHALL read the proofs and the ecosystems, and refuse an unknown proof.
- **SP-46~1**: WHEN glob patterns are expanded over a tree THE SYSTEM SHALL
  collect the matching files so that their file-set digest equals the digest of
  the same files given directly.
- **SP-47~1**: WHEN mutant bytes are encrypted to an age recipient THE SYSTEM
  SHALL let the holder of the identity recover them and nobody else.

## Operator mutants and sentinels

- **SP-83~1**: WHEN a mutant is identified THE SYSTEM SHALL derive its id as the
  SHA-256 of the RFC 8785 canonical form of its edit set, regardless of the
  order of the edits, and give another id when any field of any edit changes.
- **SP-84~1**: WHEN an edit set is checked THE SYSTEM SHALL require a known
  target kind (definition, module assignment or module addition), a file and a
  qualname, and no unit edited twice, SHALL apply an edit only inside the bounds
  of its source, replace a module assignment by its span, append a module
  addition on its own line, and refuse a module addition of a name the file
  already declares and a module assignment when the ecosystem has no assignment
  locator.
- **SP-85~1**: WHEN a numeric constant or a `Decimal` string is mutated THE
  SYSTEM SHALL produce the value plus one, minus one and times ten in the form
  of the original, and nothing for a form it cannot re-emit.
- **SP-86~1**: WHEN operator candidates are screened THE SYSTEM SHALL drop those
  that do not parse, those equal to their original after normalization and those
  equal to an earlier candidate, and SHALL never screen a sentinel.
- **SP-87~1**: WHEN operator mutants are sampled THE SYSTEM SHALL keep at most
  `K_op` of them, exactly `K_op` when more exist, stratified by site and family
  so that no stratum with candidates left trails another by more than one.
- **SP-88~1**: WHEN operator mutants are sampled THE SYSTEM SHALL give the same
  sample for the same seed whatever the order of the candidates, and another
  sample for another seed.
- **SP-89~1**: WHEN sentinels are drawn for a footprint file THE SYSTEM SHALL
  draw two by seed from the catalogue of dead assignment, alpha-renaming and
  unused private module constant, with the injected identifiers and the
  positions drawn by seed and no fixed marker in the source, mark them as
  sentinels, and keep them outside the `K_op` cap.
- **SP-90~1**: WHEN a footprint is closed THE SYSTEM SHALL include every
  definition reachable from the entry functions through the static call graph
  and every module-level assignment those definitions read.
- **SP-91~1**: WHEN a Python source is enumerated THE SYSTEM SHALL produce
  relational, arithmetic, numeric constant, boolean and logical, return value,
  module constant, constructor argument, decimal string, min and max, and and
  or, boolean literal and exception class mutants, each as an edit of the whole
  outermost definition or of the module-level assignment.
- **SP-92~1**: WHEN Python sentinels are listed THE SYSTEM SHALL skip functions
  that use `locals`, `vars`, `globals`, `eval`, `exec` or `inspect`, and SHALL
  rename only locals that no nested scope, parameter or comprehension binds.
- **SP-93~1**: WHEN a Python unit is normalized THE SYSTEM SHALL ignore
  comments, docstrings and layout, keep every change of meaning, and refuse a
  unit that does not parse.
- **SP-94~1**: WHEN the call graph of a Python file is read THE SYSTEM SHALL
  give each outermost definition its owner, aliases, calls and reads, and list
  the module-level assignments.
- **SP-95~1**: WHEN a Go source is enumerated THE SYSTEM SHALL produce the
  operator families as edits of whole declarations or package-level specs named
  `pkg.Func`, `(*T).Method` or `T.Method`, taking the error sentinel swap in the
  place of the exception class swap.
- **SP-96~1**: WHEN Go sentinels are listed THE SYSTEM SHALL skip functions that
  use `reflect`, `runtime` or `unsafe`, and SHALL rename only locals resolved by
  the parser and never a name used as a composite literal key.
- **SP-97~1**: WHEN a Go unit is normalized or its call graph is read THE SYSTEM
  SHALL ignore comments and layout, refuse a unit that does not parse, and list
  the package-level names with their qualnames.
- **SP-98~1**: WHEN a file is given to the generator THE SYSTEM SHALL choose the
  mutation engine by the extension of the file and refuse a file no registered
  engine handles.
- **SP-99~1**: WHEN mutants are generated for a footprint THE SYSTEM SHALL
  restrict operator mutants and sentinels to the footprint, cap the operator
  mutants at `K_op`, and give the same mutants for the same files and seed.
- **SP-100~1**: WHEN mutants are generated from any source THE SYSTEM SHALL give
  operator mutants and sentinels that re-parse, differ from the original, and
  splice into a parsable file, deterministically for a fixed seed.

## Host store

- **SP-148~1**: WHEN the host store of a project is opened THE SYSTEM SHALL keep
  `queue.json`, `requests/`, `tickets/`, `responses/`, a digest-chained
  `journal.jsonl` and a `secret` under
  `$XDG_STATE_HOME/spec-proof/discpower/<repo-digest>/host/`, with directories
  private to the user and the secret readable by the user only, and SHALL derive
  the repository digest from the absolute path of the project root.
- **SP-149~1**: WHEN the filesystem store is tested THE SYSTEM SHALL satisfy the
  shared Store contract suite of the host protocol.
- **SP-150~1**: WHEN a store is opened again THE SYSTEM SHALL return the requests,
  entries, responses, journal chain and secret it held, and SHALL write each
  record atomically so that a reader never sees a partial file.
- **SP-151~1**: WHEN a request is recorded in the queue THE SYSTEM SHALL keep the
  wave of its first record, merge the criteria it serves without duplicates and
  list the items ordered by request id, and WHEN the wave advances THE SYSTEM
  SHALL return consecutive numbers starting at one.
- **SP-152~1**: WHEN the state and cache locations are resolved THE SYSTEM SHALL
  use `$XDG_STATE_HOME` and `$XDG_CACHE_HOME` when they are absolute paths and
  the defaults under the home directory otherwise.

## Stores of proofs, mutants and facts

- **SP-153~1**: WHEN a Statement is written to the proof cache THE SYSTEM SHALL
  validate it against the embedded schemas first, refuse an invalid one, and
  write it atomically at
  `.spec-proof/cache/discriminating-power/<ID>~<REV>/<cacheKey>.json`.
- **SP-154~1**: WHEN a mutant body is kept THE SYSTEM SHALL write it under
  `$XDG_CACHE_HOME/spec-proof/<repo-digest>/mutants/` and never in the work tree,
  and return exactly the edit set it was given.
- **SP-155~1**: WHEN an executed fact is memoized THE SYSTEM SHALL return it for
  its key, treat an absent or unreadable entry as a miss and write atomically.
- **SP-156~1**: WHEN an executor's output is checked against a response schema THE
  SYSTEM SHALL accept it exactly when the schema holds and list every violation
  otherwise.

## Criteria from Shallnot

- **SP-157~1**: WHEN a Shallnot report of schema 1.2 or later is read THE SYSTEM
  SHALL give each active requirement with its statement, its statement digest, its
  examples and the tests bound to it, identified the way the JUnit reporter
  names them, and SHALL refuse a report below 1.2 or without a digest.
- **SP-158~1**: WHEN criteria are loaded THE SYSTEM SHALL run `shallnot gate
  --advisory --json-out` on the project, read the report it wrote, remove it, and
  parse no spec itself.

## Pytest probes and coverage

- **SP-159~1**: WHEN the signature of a bound target is asked THE SYSTEM SHALL
  report its parameter names and a type hint from the annotation or the default
  of each, and SHALL report an unknown target as an error.
- **SP-160~1**: WHEN calls are invoked in a workspace THE SYSTEM SHALL return for
  each call the typed value it returned or the name of the exception it raised,
  in order, with integer, decimal, string, boolean, null and datetime arguments
  converted to their native types, and never let one failing call hide the
  others.
- **SP-161~1**: WHEN the coverage of a pytest suite is recorded THE SYSTEM SHALL
  give, per test, the functions and methods of the project's source files that it
  executed, naming nested definitions by their qualified path.

## Spec values and policy

- **SP-162~1**: WHEN two literals or two observations are compared THE SYSTEM
  SHALL treat integers and decimals as equal when they denote the same number
  whatever their spelling, compare every other type by type and text, and treat
  two raised exceptions as the same behaviour exactly when they have the same
  name.
- **SP-163~1**: WHEN the decision policy is built THE SYSTEM SHALL default to
  `tau_judge` 0.70, two judges, `tau_screen` 0.20, three deciders, `min_valid` 3,
  `min_sites` 2, three rounds, `K_op` 400, 40 host tickets, the `corroborated`
  pass basis and the `restricted` isolation floor, refuse a threshold outside
  [0,1], a count below one and an unknown basis or isolation, and give another
  digest when any parameter changes.

## Decisions on mutants

- **SP-164~1**: WHEN a mutant is a sentinel THE SYSTEM SHALL decide it EQUIVALENT
  by construction, and WHEN a mutant fails the examples of one criterion alone
  THE SYSTEM SHALL decide it VALID for that criterion on the basis of the spec
  example with full confidence and the failed example as corroboration, failing
  examples of several criteria SHALL make it collateral when the criterion is
  among them, and a mutant failing examples of other criteria only SHALL be
  decided NOT_VIOLATING and off target.
- **SP-165~1**: WHEN the judges rule a mutant VALID THE SYSTEM SHALL accept it
  only when every judge agrees at or above `tau_judge`, a witness is confirmed by
  execution, the mutant builds and the tests of every other criterion pass, and
  for a killed mutant a test of the criterion that killed no sentinel killed it
  and every response keeps judged credit under the isolation floor, and SHALL
  otherwise leave it UNDETERMINED, a survivor keeping its power to block under a
  response below the floor.
- **SP-166~1**: WHEN the judges rule a surviving mutant EQUIVALENT THE SYSTEM SHALL
  accept it only when the differential search found no difference and the pass
  basis is not `executed-only`, WHEN they rule it NOT_VIOLATING THE SYSTEM SHALL
  accept it only under the `judged` basis and count it as a host exclusion, and
  WHEN execution refutes an answer, which is EQUIVALENT for a killed mutant or a
  mutant whose behaviour differs, or a witness that execution does not confirm,
  THE SYSTEM SHALL name the contradiction so that the answer is rejected.
- **SP-167~1**: WHEN the outcome of a criterion is decided THE SYSTEM SHALL block
  on a failing reference, a failing example or a surviving VALID mutant, escalate
  on an undetermined survivor, the absence of a killed VALID mutant, a test that
  killed a sentinel, an exhausted budget or an unresponsive host, and pass only
  when a killed VALID mutant was credited and nothing else remains, blocking
  before escalating.
- **SP-168~1**: WHEN the search for mutants is checked for its end THE SYSTEM SHALL
  stop on a block, and otherwise once `min_valid` VALID mutants cover `min_sites`
  distinct sites with at least one killed.
- **SP-169~1**: WHEN screen answers are read THE SYSTEM SHALL take P(VIOLATES) as
  the mean of the answers' probabilities, which for votes without confidence is
  their share of VIOLATES, and flag a survivor into the pool at or above
  `tau_screen`.

## Differential search

- **SP-170~1**: WHEN inputs are drawn for an operation THE SYSTEM SHALL give the
  same inputs for the same pool, signature and seed, sweep each parameter over
  both sides of every number of the code and of the examples before it draws
  random combinations, never repeat an input and never exceed the number asked.
- **SP-171~1**: WHEN the observations on the reference and on a mutant are
  compared THE SYSTEM SHALL report no difference when they agree, and otherwise
  the simplest inputs on which the reference returns and the mutant differs, at
  most three.

## Coverage

- **SP-172~1**: WHEN coverage is queried THE SYSTEM SHALL list the units executed
  by given tests as entry functions, sorted and without duplicates, and the tests
  that executed a unit or a definition nested in it.

## Prompts

- **SP-173~1**: WHEN a request prompt is built THE SYSTEM SHALL name no criterion
  by its identifier, show a generation request no test, change the template digest
  exactly when the prompt text or the response schema changes, and carry at most
  64 questions in a closed request.
- **SP-174~1**: WHEN an executor's output is read THE SYSTEM SHALL give the
  generated mutants, the judged answer with its confidence and witness, an
  equivalence answer other than EQUIVALENT as undetermined, and the screen answers
  by question id, and a vote that states no confidence SHALL count as full
  confidence.

## Execution of mutants

- **SP-175~1**: WHEN a mutant is executed THE SYSTEM SHALL run the tests that
  executed the edited definitions, or every test for a sentinel, a module-level
  edit or an unknown coverage, report a test outside the selection as survived,
  rerun every kill once and count a kill that does not repeat as survived, and
  return the memoized result for the same mutant and suite.
- **SP-176~1**: WHEN a witness is confirmed THE SYSTEM SHALL confirm it when the
  reference returns what the judge expects and the mutant does not, refuse it
  when execution says otherwise and report it absent when it cannot be run, and
  WHEN the examples are run THE SYSTEM SHALL list the criteria whose examples the
  mutant fails.
- **SP-194~1**: WHEN a mutant workspace is requested for use THE SYSTEM SHALL
  splice and build it first, report a mutant that does not splice or build
  without calling the use, call the use with the live workspace otherwise and
  release the workspace afterwards.

## Host tickets

- **SP-180~1**: WHEN recorded answers are replayed THE SYSTEM SHALL answer every
  pending ticket from the recording of its request through the same checks and
  fail with a miss when a request has no recording, and WHEN answers are recorded
  THE SYSTEM SHALL keep each output and its executor under the request id.

## The prove use case

- **SP-181~1**: WHEN `prove` runs THE SYSTEM SHALL exit 3 while tickets wait for the
  host, 1 when a criterion blocks or escalates, 0 otherwise, advance one wave per
  run, give the same requests when run again, and refuse `--resume` when no
  request was ever emitted.
- **SP-182~1**: WHEN an honest host answers every request THE SYSTEM SHALL block a
  criterion whose weak test lets a validated mutant survive and pass criteria
  whose tests kill every validated mutant, writing one Statement per criterion.
- **SP-183~1**: WHEN the host forges, omits, replays with a stale nonce or breaks the
  schema of its answers THE SYSTEM SHALL never pass a criterion.
- **SP-184~1**: WHEN any assignment of judge answers is considered, including the most
  favourable one, THE SYSTEM SHALL never block or credit a mutant on judged
  evidence without executed corroboration.
- **SP-185~1**: WHEN the same votes are consumed in any order THE SYSTEM SHALL write the
  same Statement, with no nonce, time or consumption order in it.
- **SP-186~1**: WHEN any input of a proof changes THE SYSTEM SHALL give another cache key,
  the inputs being the statement, the test sources, the code tree, the reference
  results, the examples and their bindings, the policy, the prompt templates, the
  mutant set, the models and the host responses.
- **SP-187~1**: WHEN a criterion has no host tickets left THE SYSTEM SHALL leave the
  judgments it could not ask for UNDETERMINED with the reason `budget_exhausted`
  and escalate the criterion, never pass it.
- **SP-188~1**: WHEN a criterion's inputs are unchanged since its last Statement and
  `--changed` selects it THE SYSTEM SHALL not prove it again and report its stored
  outcome.
- **SP-189~1**: WHEN a Statement is written THE SYSTEM SHALL validate it against the
  embedded schemas, write it under `.spec-proof/cache/discriminating-power`, and
  keep mutant bodies only in the user cache outside the work tree.
- **SP-190~1**: WHEN the ecosystem cannot probe calls THE SYSTEM SHALL ask the host for
  nothing, report the skipped searches as findings and escalate every criterion
  that has no VALID mutant.

## Configuration and command line

- **SP-191~1**: WHEN the project configuration names operations and policy parameters
  THE SYSTEM SHALL bind each operation per ecosystem or language, apply the
  parameters over the defaults, refuse an invalid parameter, and write both back
  in a fixed order.
- **SP-192~1**: WHEN `request`, `systemone` or `doctor` runs THE SYSTEM SHALL list,
  show and submit tickets, export closed requests and import their answers from a
  relay, report the checks of the project, and exit 1 for a refusal of the
  protocol and 2 for a tool failure.
- **SP-193~1**: WHEN `prove` is given conflicting flags or unexpected arguments THE
  SYSTEM SHALL refuse them before it runs anything.

## Host harness

- **SP-227~1**: WHEN the project configuration has a `host` section THE SYSTEM SHALL
  read `sampling` and `require_attestation` from it, keep attestation required and
  sampling off when they are absent, refuse an unknown or mistyped member, and
  tolerate the sections other modules keep in the same file.
- **SP-228~1**: WHEN a host harness is selected THE SYSTEM SHALL name the executor
  tools as that harness spells them, recognize them and no other tool in a hook
  payload, name the type of each executor agent as that harness reports it and map
  a reported type back to the role of its requests, derive the isolation its
  allowlist reaches (restricted on Claude Code, unrestricted on Codex,
  restricted-unverified on Copilot), and waive attestation on a harness that
  cannot issue a capability.
- **SP-231~1**: WHEN the executor server runs THE SYSTEM SHALL offer exactly
  `fetch_request` and `submit_response`, and `drain` only when the client declared
  sampling and the project turned it on; return the schema violations of an answer
  as a result and allow one correction before the ticket is rejected; refuse a
  different second answer as `resubmission_conflict` and accept the same one as a
  replay; and refuse a ticket that is not an identifier and a nonce other than the
  issue's.
- **SP-232~1**: WHEN `drain` is called THE SYSTEM SHALL put every pending ticket to
  the client's model through sampling with no context and no tools, submit the
  JSON object of each sampled answer as a sampled response with no capability,
  serve only the module it is named for, and stop at the first refused sample or
  invalid answer without accepting it.
- **SP-239~1**: WHEN `mcp` or `hook` is run THE SYSTEM SHALL refuse a role, harness
  or hook it does not know with exit 1, report with exit 1 and never block when a
  hook cannot read its payload, leave a call that is not about the module's
  executors alone without opening any state, do nothing in a project that has no
  configuration of the module, deny a call of the module's own tools that it
  cannot check, accept a receipt with whitespace around it, and resolve the
  project root from the option, the harness's project directory or the working
  directory, with symbolic links resolved, so that the hook, the server and the
  tickets of one project share one host directory.
- **SP-240~1**: WHEN the fake-host personas drive the module through its command
  line THE SYSTEM SHALL give each the verdict ADR 0005 asserts for it.
- **SP-241~1**: WHEN the host files are generated THE SYSTEM SHALL produce them from
  one definition, the same bytes on every run, and the committed trees of the
  plugin and the harness scaffolds SHALL equal what the generator produces.
- **SP-242~1**: WHEN the host files of a harness are read against the formats that
  harness documents THE SYSTEM SHALL give the Claude Code plugin its manifest, its
  executor server, three agents holding exactly the two executor tools with the
  inherited model, a blind context and four turns, its four hooks under matchers
  that select the executor alone, and a skill that pre-approves what its loop
  runs; give Codex read-only agents carrying the executor server, hook entries and
  a skill; give Copilot agents naming server and tool pairs, a hooks file and a
  skill; and carry no text about their own authoring.

## Installation

- **SP-243~1**: WHEN `init` runs THE SYSTEM SHALL require a Shallnot whose report
  schema is 1.2 or later, write the module's configuration and ignore its local
  state, install the files of each selected harness (`auto`, `none` or a list of
  Claude Code, Codex and Copilot) by merging into every file another tool shares
  and writing the files it owns, plan no change on a second run, and leave
  Shallnot's configuration, its block of AGENTS.md and its hooks as they were.
- **SP-244~1**: WHEN `enable` or `disable` runs THE SYSTEM SHALL turn the proof on or
  off in the module's configuration, add or remove its kind in `proofs.require` of
  `shallnot.yaml` and its block of AGENTS.md by line edits that keep every other
  byte, so that disabling restores what enabling changed, refuse a project with
  no `shallnot.yaml`, and refuse a layout it cannot extend without rewriting.
- **SP-245~1**: WHEN `doctor` runs THE SYSTEM SHALL check that the report schema
  Shallnot prints accepts version 1.2 and refuses 1.1, and report the check.

## CI authority

- **SP-247~1**: WHEN `host push` runs THE SYSTEM SHALL publish every answered ticket
  with the profile it is credited with and the marks against it as a tree under the
  ref of the head commit with git plumbing, replace a ref that exists, publish the
  ref to a remote when asked, touch neither the working tree nor the current
  branch, warn when a tracked file differs from the head commit, refuse a store
  whose journal chain is broken and publish nothing when nothing is answered.
- **SP-248~1**: WHEN `host pull --ci-run ID` runs THE SYSTEM SHALL download the
  artifact of the run with `gh`, refuse a download that fails or a bundle that is
  not one, and make tickets of the requests it holds once, with the votes and the
  queue metadata CI gave them.
- **SP-249~1**: WHEN `prove --ci` runs THE SYSTEM SHALL draw the seed of the head
  commit from the first CI run, record it and reuse it on every later run of the
  commit, produce the Statements as CI, serve the tickets it is asking for from the
  bundle given with `--import-host` through the same checks as any answer, ignore
  what the bundle holds that it does not ask for, leave the other tickets pending,
  mark again what the pushing host marked, refuse a bundle of another version that
  left requests unanswered, and leave the requests that remain where the CI job
  uploads them from.

## End to end

- **SP-250~1**: WHEN a project is equipped with Shallnot and the module, edited and
  proved THE SYSTEM SHALL have the end-of-turn hook of Shallnot hold the turn with
  `awaiting_host` naming the module's skill while the host owes answers, let a
  simulated Claude Code host answer every ticket through the real hooks and the
  real executor server until `prove` ends with a pass for the adequate tests and a
  block for the weak one, journal the executors as started, fetched, accepted,
  attested and reported with no leak, exposure or conflict, deny a main-session
  call of an executor tool, and have `shallnot check` refuse the project for the
  weak criterion alone with a surviving valid mutant.
- **SP-251~1**: WHEN a forger answers every ticket from the command line, or a host
  runs executors whose hooks never fire, THE SYSTEM SHALL give no criterion a
  pass, accept no uncapped answer, write no Statement for a hookless host and
  have Shallnot refuse the turn or the check.

## Contracts with the format and the gate

- **SP-406~1**: WHEN a mutant decision is recorded THE SYSTEM SHALL name where its
  confidence came from in `extensions.decision.confidenceSource`: `execution` for a
  sentinel, a spec example and a mutant no judge could classify, and
  `host-votes(<judge_k>)` with the mapping of the weakest stated confidence for a
  judged decision, and SHALL leave the source out of `deciders`.
- **SP-407~1**: WHEN a Statement lists its inputs THE SYSTEM SHALL name the `spec`
  input `ID~REV` with the statement digest, list as `test_source` descriptors the
  files that hold a test bound to the criterion and those that hold a test whose
  execution reaches the footprint under the reference coverage (every file that
  holds a bound test when no footprint restricts the scope), and list as
  `code_tree` descriptors the files of the footprint, each by its path relative to
  the project root and the SHA-256 of its exact bytes.
- **SP-408~1**: WHEN a screen request is built THE SYSTEM SHALL build its question
  set and its response schema through the host protocol's closed question type, with
  one choice question per requirement whose options carry their criteria text, and
  WHEN a relay imports the response of a System One endpoint THE SYSTEM SHALL
  translate it with the question set of the ticket and refuse a response that
  leaves a question unanswered or picks no option.
