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
- **SP-1100~1**: WHEN a Java or Kotlin source is enumerated THE SYSTEM SHALL
  produce relational, arithmetic, numeric constant, boolean and logical, return
  value, static constant, constructor argument, decimal string, minimum and
  maximum, and and or, boolean literal and exception class mutants, each as an
  edit of one node inside a whole method, constructor, function or constant
  declaration, so that the rest of the unit keeps its layout.
- **SP-1101~1**: WHEN the units of a Java or Kotlin file are listed THE SYSTEM
  SHALL name each outermost method, constructor or function as the definition
  locator does, take the definitions nested in it as part of it, leave out a
  definition whose name several share, and list the static finals of Java and
  the constant properties of Kotlin as assignments.
- **SP-1102~1**: WHEN Java and Kotlin sentinels are listed THE SYSTEM SHALL offer
  a dead local before a statement of a body and the consistent rename of a local
  that nothing else binds or names, offer the unused private constant to Kotlin
  files only, draw every injected identifier by seed, and skip a unit that uses
  reflection.
- **SP-1103~1**: WHEN the call graph of a Java or Kotlin file is read THE SYSTEM
  SHALL give each outermost definition its owner, aliases, calls and reads,
  following static calls, instance calls, calls on this and constructors, and
  list the constants as assignments.
- **SP-1104~1**: WHEN a Java or Kotlin unit is normalized THE SYSTEM SHALL ignore
  comments and layout, keep every change of meaning, and refuse a unit that does
  not parse.
- **SP-1105~1**: WHEN mutants are generated from any Java or Kotlin source THE
  SYSTEM SHALL give operator mutants and sentinels that splice back into a
  parsable file with the post-splice invariant intact.
- **SP-1106~1**: WHEN Java sentinels and operator mutants are compiled THE SYSTEM
  SHALL compile every sentinel and the large majority of the operator mutants.

## JavaScript and TypeScript mutation

- **SP-1000~1**: WHEN a JavaScript or TypeScript source is enumerated THE SYSTEM
  SHALL produce relational, arithmetic, numeric constant, boolean and logical,
  return value, module constant, constructor argument, decimal string, min and
  max, and and or, boolean literal and exception class mutants, each as an edit
  of the whole outermost function or method definition or of the module-level
  declarator, choose the grammar from the file extension, and leave a mutation
  out when its unit no longer parses.
- **SP-1001~1**: WHEN JavaScript or TypeScript sentinels are listed THE SYSTEM
  SHALL skip functions that mention `eval`, `Function` or `arguments` or use
  `with`, SHALL insert a dead local that a `void` statement reads, SHALL add the
  unused module constant as an export in a module file, and SHALL rename only
  locals declared by a plain identifier that no nested scope, parameter, pattern,
  catch clause, loop binding, shorthand property or other occurrence of the name
  in the file refers to.
- **SP-1002~1**: WHEN the units of a JavaScript or TypeScript file are listed
  THE SYSTEM SHALL name each outermost function or method definition with the
  span and qualified name the shared definition locator gives it, list a
  module-level declarator that holds no definition as an assignment unit, leave
  out every unit whose kind and qualname another unit shares, and give a splice
  of every candidate and sentinel that the shared locator accepts.
- **SP-1003~1**: WHEN a JavaScript or TypeScript unit is normalized THE SYSTEM
  SHALL ignore comments, statement terminators and layout, keep every change of
  meaning, read a bare method inside a class and a declarator inside a
  declaration, and refuse a unit that does not parse.
- **SP-1004~1**: WHEN the call graph of a JavaScript or TypeScript file is read
  THE SYSTEM SHALL give each outermost definition its owner, aliases, calls and
  reads, reference a renamed static import by the name its module exports, and
  list the module-level declarators.
- **SP-1005~1**: WHEN mutants are generated from any JavaScript or TypeScript
  source THE SYSTEM SHALL give operator mutants and sentinels that re-parse,
  differ from the original and splice into a parsable file, deterministically
  for a fixed seed.

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
- **SP-151~2**: WHEN a request is recorded in the queue THE SYSTEM SHALL keep the
  wave of its first record, merge the criteria and the sessions it serves
  without duplicates and list the items ordered by request id, WHEN the wave of
  a session advances THE SYSTEM SHALL return consecutive numbers starting at one
  for that session alone, SHALL satisfy the queue contract suite of the host
  protocol, and SHALL read a `queue.json` written without sessions as the prove
  session's.
- **SP-2153~1**: WHEN `request list` or `systemone export` is given `--session`
  with `prove` or `scan:<key>` THE SYSTEM SHALL list only the tickets of the
  requests that session holds, give with each ticket `request list --json`
  prints the sessions that hold its request, list the tickets of every session
  without the flag, and refuse a text that names no session; and WHEN `prove
  --resume` runs while the prove session holds no request THE SYSTEM SHALL
  refuse it, whatever requests other sessions hold.
- **SP-2152~1**: WHEN the prove session and a scan session use the filesystem
  store of one project at the same time THE SYSTEM SHALL issue one ticket per
  request whichever session asks first, show each session its own tickets and
  the shared ones, and leave the tickets of one session emitted through the
  waves and the end of the other.
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

## Go probes

- **SP-950~1**: WHEN the signature of a bound Go target `import/path:Function` or
  `import/path:Type.Method` is asked THE SYSTEM SHALL report the parameters of the
  exported function or method as the package declares them, named in snake case
  and each with a type hint from the types of the package, leave out a leading
  `context.Context` and a variadic parameter, and SHALL refuse a target that is
  malformed, unexported, absent, generic, or not a function or a method of a
  defined type.
- **SP-951~1**: WHEN calls of Go targets are invoked in a workspace THE SYSTEM
  SHALL return for each call, in order, the typed value it returned, null when it
  returns nothing, the name of the exported error variable of the package that
  the error it returned or panicked with is or wraps, else the name of the type of
  that error, and `Panic` for a panic with another value.
- **SP-952~1**: WHEN a call is made THE SYSTEM SHALL decode each argument of the
  literal grammar into the type of its parameter, whether an integer, a float, a
  string, a boolean, a null, an ISO-8601 datetime or a library number that reads
  text, find it under any spelling of the name of the parameter, and observe a
  call whose arguments do not fit as raising `ArgumentError` without running the
  code.
- **SP-953~1**: WHEN calls are invoked THE SYSTEM SHALL run the code of the
  workspace it is given through a program built inside its module, so that
  internal packages are reachable, write nothing into the workspace, and report a
  program that does not build as an error that carries the compiler's message.
- **SP-954~1**: WHEN a call exceeds its deadline or ends the process THE
  SYSTEM SHALL observe the call as raising and answer the calls after
  it in a new process, and SHALL report a program that cannot read its request or
  write its response as an error.
- **SP-955~1**: WHEN a call is repeated THE SYSTEM SHALL observe the same thing,
  with times in UTC and the default random source seeded the same way.
- **SP-956~1**: WHEN the examples, the differential search and the confirmation of
  a witness run on a Go project THE SYSTEM SHALL pass an example the reference
  meets and fail one a violating mutant breaks, find a witness where a mutant moves
  a boundary, and confirm that witness by running it on the reference and on the
  mutant.
- **SP-957~3**: WHEN a host whose responses are exposed, as those of a command-line
  host are, answers every request for a Go project THE SYSTEM SHALL block the
  criterion whose weak test lets a VALID_e mutant survive, though its writer is
  exposed, and pass the criteria whose tests kill a mutant that fails one of their
  examples and whose survivors execution excludes.

## Jest and Vitest projects

- **SP-1010~1**: WHEN a project tree is inspected THE SYSTEM SHALL recognize Jest
  by `jest.config.*`, a `jest` key of `package.json` or a jest dependency and
  Vitest by `vitest.config.*`, `vitest.workspace.*` or a vitest dependency, name
  each strategy after the runner it binds, and recognize neither in a package
  that depends on none.
- **SP-1011~1**: WHEN a Jest or Vitest project is executed THE SYSTEM SHALL
  resolve the package runner from the configuration, then from the lock file
  (`pnpm exec`, `yarn`, `bunx --no-install`, `npx --no-install`), use
  `npx --no-install` for a project without one, and refuse a runner that is not
  installed.
- **SP-1012~1**: WHEN the changed files of a Node project are checked THE SYSTEM
  SHALL run `tsc --noEmit -p tsconfig.json` when a TypeScript file changed and the
  project has a tsconfig, run `node --check` on each changed JavaScript file,
  report the first failure with the message of the tool as a failed build, and
  check nothing else.
- **SP-1013~1**: WHEN a Jest project is executed THE SYSTEM SHALL select bound
  tests by path and by one anchored regex over the full titles jest-junit
  reports, without the space it leads a test outside any describe with, and run
  `jest --ci --reporters=jest-junit` in one process with the report path in
  `JEST_JUNIT_OUTPUT_FILE`.
- **SP-1014~1**: WHEN a Vitest project is executed THE SYSTEM SHALL select bound
  tests by one anchored regex over the titles its JUnit reporter joins with
  `>`, accepting the space the filter joined them with before, and run
  `vitest run --reporter=junit` with one worker.
- **SP-1015~1**: WHEN a selection runs under a title filter THE SYSTEM SHALL
  report the outcomes of the selection only, dropping the tests the filter left
  out that the runner reports as skipped, and keep every outcome of a full run.
- **SP-1016~1**: WHEN the workspace of a Node project is laid out THE SYSTEM
  SHALL leave out the coverage directories, treat `node_modules` as
  dependencies, and treat tests, setup files, tooling configuration and
  declaration files as files that are no source to mutate.

## Node probes

- **SP-1020~1**: WHEN the signature of a bound Node target `path/file.ts:function`
  or `path/file.ts:Class.method` is asked THE SYSTEM SHALL report its parameters
  named in snake case, each with a type hint read from the annotation by the
  TypeScript compiler of the project, else from the default of the parameter,
  leave out a rest parameter, and SHALL refuse a target that names nothing
  callable.
- **SP-1021~1**: WHEN calls of Node targets are invoked in a workspace THE SYSTEM
  SHALL return for each call, in order, the typed value it returned, null for
  nothing, an object as canonical JSON with sorted keys, a number without
  exponent notation and a time in UTC, awaiting a promise.
- **SP-1022~1**: WHEN a call throws THE SYSTEM SHALL observe it as raising the
  class name of what was thrown, whether the throw is synchronous, a rejected
  promise or an error from a timer, and SHALL observe a target the module does
  not export as raising `UnknownTarget`.
- **SP-1023~1**: WHEN a call is made THE SYSTEM SHALL decode each argument of the
  literal grammar into the type its parameter declares, find it under any
  spelling of the name of the parameter, leave out an optional one, and observe a
  call whose arguments do not fit as raising `ArgumentError` without running the
  code.
- **SP-1024~1**: WHEN calls are invoked THE SYSTEM SHALL load the module of the
  workspace as an ES module or as CommonJS, load TypeScript with the tsx of the
  project, else its ts-node, else a transpile by its typescript that resolves a
  `.js` import to its `.ts` sibling, call a method on a fresh instance, write
  nothing into the workspace, and report a module that does not load, or
  TypeScript with no tooling to load it, as an error that carries the message.
- **SP-1025~1**: WHEN a call exceeds its deadline or ends the process THE SYSTEM
  SHALL observe the call as raising `Timeout` or `Crash`, whether it runs
  synchronously or never settles, and answer the calls after it in a new process.
- **SP-1026~1**: WHEN a call is repeated THE SYSTEM SHALL observe the same thing,
  with times in UTC and the random source seeded the same way.
- **SP-1027~1**: WHEN calls are invoked on a machine without node THE SYSTEM
  SHALL report that no toolchain was found rather than a raised exception.
- **SP-1040~1**: WHEN the examples, the differential search and the confirmation
  of a witness run on a Jest or Vitest project THE SYSTEM SHALL pass an example the
  reference meets and fail one a violating mutant breaks, find a witness where a
  mutant moves a boundary, and confirm that witness by running it on the
  reference and on the mutant.
- **SP-1041~3**: WHEN a host whose responses are exposed, as those of a command-line
  host are, answers every request for a Jest or Vitest project THE SYSTEM SHALL
  block the criterion whose weak test lets a VALID_e mutant survive, though its
  writer is exposed, and pass the criteria whose tests kill a mutant that fails
  one of their examples and whose survivors execution excludes.
- **SP-1042~1**: WHEN a project is inspected THE SYSTEM SHALL recognize a Jest or
  Vitest project among the ecosystems it supports.

## JVM probes and ecosystems

- **SP-1110~1**: WHEN a project tree is inspected THE SYSTEM SHALL recognize a
  Maven project by its `pom.xml` and a Gradle project by a build or settings
  script in Groovy or Kotlin, take test classes of both languages as test files,
  and leave the Gradle caches of a project out of its workspaces.
- **SP-1111~1**: WHEN a Maven or Gradle project is executed THE SYSTEM SHALL
  resolve the project's executable wrapper as `./mvnw` or `./gradlew`, else the
  build tool found on PATH, and report their absence as an error.
- **SP-1112~1**: WHEN the build of a Maven or Gradle project is checked THE
  SYSTEM SHALL compile its main and test sources offline in one process, with no
  daemon left behind, and report a failed or timed out build as diagnostics.
- **SP-1113~1**: WHEN a selection of tests is run THE SYSTEM SHALL clear the
  reports of an earlier run, run Surefire directly on Maven and a clean test task
  on Gradle, filtered to the selection, and read the outcomes from the JUnit XML
  of the build tool.
- **SP-1114~1**: WHEN a test bound by its display name is selected THE SYSTEM
  SHALL map it through its source location to the method whose lines hold it,
  the annotations of the method included, to the whole class of the next method
  when the location is outside every method, and to the binary name of nested
  classes, and refuse a test no definition explains.
- **SP-1115~1**: WHEN the signature of a bound JVM target
  `package.Class:method` is asked THE SYSTEM SHALL report the parameters of the
  method as its Java or Kotlin source declares them, named in snake case and each
  with a type hint, take the top-level functions of a Kotlin file as the methods
  of its file class, and refuse a target that is malformed, absent or
  overloaded.
- **SP-1116~1**: WHEN calls of JVM targets are invoked in a workspace THE SYSTEM
  SHALL decode each argument of the literal grammar into the type of its
  parameter, finding it under any spelling of the parameter's name, whether an
  integer, a decimal, a string, a boolean, a null or an ISO-8601 datetime, and
  return for each call, in order, the typed value it returned, with times as
  ISO-8601, or the name of the class of the exception it threw.
- **SP-1117~1**: WHEN a call does not fit its method, kills the process or exceeds
  its deadline THE SYSTEM SHALL observe the call as raising `ArgumentError`,
  `Crash` or `Timeout` and answer the calls after it in a fresh JVM.
- **SP-1118~1**: WHEN calls are invoked in the workspaces of one project THE
  SYSTEM SHALL resolve its test classpath once and share it between workspaces
  whose build files are the same.
- **SP-1120~1**: WHEN the name of a parameter is spelled for a spec THE SYSTEM
  SHALL write its words in lower case joined by underscores, and write an
  identifier already so spelled as it is.
- **SP-1121~1**: WHEN `init` inspects a Maven or Gradle project THE SYSTEM SHALL
  name the ecosystem in the configuration it writes.
- **SP-1122~3**: WHEN a host whose responses are exposed, as those of a command-line
  host are, answers every request for a Maven project THE SYSTEM SHALL block the
  criterion whose weak test lets a VALID_e mutant survive, though its writer is
  exposed, and pass the criteria whose tests kill a mutant that fails one of their
  examples and whose survivors execution excludes.
- **SP-1123~1**: WHEN the EcosystemContract runs on Maven with Java, Gradle with
  Java and Gradle with Kotlin THE SYSTEM SHALL pass its six cases on each.

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

- **SP-164~2**: WHEN a mutant is classified for a criterion THE SYSTEM SHALL
  decide a sentinel EQUIVALENT by construction, exclude a mutant whose compiled
  form is identical to the reference's, decide a mutant that fails an example of
  the criterion and of no other criterion owner-attributed and VALID on the basis
  of the spec example with full confidence and the failed example as
  corroboration, and exclude as NOT_VIOLATING and off target, corroborated by
  `other_example_failed`, a mutant that fails an example or an admitted witness
  of another criterion and no example and no admitted witness of its own.
- **SP-165~2**: WHEN a mutant fails, reproduced, a block-bearing witness of the
  criterion and no test bound to another criterion killed it or errored on it
  THE SYSTEM SHALL decide it VALID_e, VALID on the basis of execution with full
  confidence and corroborated by `builds`, `other_criteria_pass` and
  `f4_witness`, decide it collateral when it is VALID_e for another criterion
  too, and make it credit-bearing only when it is not collateral, the pass basis
  is not `executed-only` and one of the witnesses it fails was written and judged
  in scope by responses at or above the isolation floor and unmarked; a mutant
  that fails such a witness but is not isolated SHALL stay unsettled.
- **SP-166~2**: WHEN validity votes are cast on a survivor no witness settles THE
  SYSTEM SHALL exclude it only when the full-resolution differential search
  found no difference, at least `judge_k` votes answer EQUIVALENT at or above
  `tau_judge` from responses at or above the floor and unmarked, no vote
  confirms a witness, no test killed it, it fails no witness the reference
  passes and the basis is not `executed-only`,
  SHALL leave it unsettled with the reason the votes give in every other case,
  VALID and NOT_VIOLATING included, and WHEN execution refutes an answer, which
  is EQUIVALENT for a killed mutant or a mutant whose behaviour differs, or a
  witness that execution does not confirm, THE SYSTEM SHALL name the
  contradiction so that the answer is rejected.
- **SP-167~2**: WHEN the outcome of a criterion and a test set is decided THE
  SYSTEM SHALL block on a reference that fails a test of the set, a failing
  example, or an owner-attributed, VALID_e or collateral mutant that survives
  every test of the set; escalate when no credit-bearing mutant of the pool is
  killed by the set, on any other surviving mutant that execution did not
  exclude, on a test of the set that killed a sentinel, an exhausted budget or
  an unresponsive host; pass only when a credit-bearing mutant was killed by a
  test of the set that killed no sentinel and nothing else remains; and block
  before escalating.
- **SP-168~2**: WHEN the search for mutants is checked for its end THE SYSTEM SHALL
  stop once the targeted and owner-attributed mutants of the pool that fail a
  block-bearing witness or an example of the criterion number `min_valid` over
  `min_sites` distinct sites, read no kill vector and no binding to decide it,
  and WHEN a finished search leaves fewer than two targeted mutants that fail a
  block-bearing witness of the criterion and are neither owner-attributed nor
  identical to the reference in compiled form THE SYSTEM SHALL ask one further
  witness writer, reading no kill vector and no binding to decide it.
- **SP-169~2**: WHEN screen answers are read THE SYSTEM SHALL take P(VIOLATES) as
  the mean, over the answers, of the probability each gives to VIOLATES: for a
  System One answer the probability its endpoint states for that option, never its
  `confidence`, which says how concentrated its probabilities are, and for a host
  vote one for VIOLATES and zero for another whatever it states, so that host
  votes give their share of VIOLATES; and flag a survivor into the pool at or
  above `tau_screen`.
- **SP-252~1**: WHEN a survivor is screened THE SYSTEM SHALL let only answers at or
  above the isolation floor and not marked leaked, exposed or resubmission_conflict
  screen it out, keep the power of any answer to flag it, flag a survivor into the
  pool when no such answer remains, and count in `extensions.operatorScreen.screenFloorEntries`
  the survivors that entered the pool only because the floor kept answers below it
  or marked from screening them out.

- **SP-2421~1**: WHEN a mutant blocks its criterion, because its decision gives
  it the power to block and every test of the criterion passes on it, and the
  Statement is produced locally THE SYSTEM SHALL record in its entry how it was
  drawn, the slot of a targeted mutant or the family of an operator one, and one
  confirmed witness: the lowest-indexed example of the criterion it fails, else a
  block-bearing witness of the criterion it fails with its code bounded by the
  bound of a host view; SHALL record none in a Statement produced by CI; and
  `prove` SHALL print for each such survivor its reference, how it was drawn and
  that example, written as the spec writes it, or that witness, and never the
  mutant's code, and the end-of-turn block of the pinned Shallnot SHALL show the
  same for each survivor.

## Witnesses

- **SP-1960~1**: WHEN a witness is judged THE SYSTEM SHALL read its three runs on
  the reference as passing when all pass, a reading conflict when all fail, flaky
  when they disagree and unrunnable when one errors or is missing, admit a
  passing witness when it also passes every planted refactor the admission names,
  make an admitted witness block-bearing when `judge_k` scope votes all answer
  FIXED at or above `tau_judge`, whatever their isolation, and escalate-only
  otherwise, and hold it credit-eligible only when its writer and every scope
  judge answered at or above the isolation floor and unmarked.
- **SP-1961~1**: WHEN a witness set is digested THE SYSTEM SHALL digest its
  criterion, imports and the content digests of its witnesses, so that two sets
  with the same content and different writers have the same digest, and digest a
  universe of sets whatever their order.
- **SP-1962~1**: WHEN the witnesses of a criterion are asked THE SYSTEM SHALL emit
  one `generate.witness` request of two votes before any other request of the
  criterion, carrying the spec documents and the criterion's statement with every
  criterion relabelled, the public signatures of the code the spec mentions and
  the signatures of the helpers of every test file, never a body, a test, a
  mutant or a kill, so that two test trees whose helpers are equal give the same
  request.
- **SP-1963~1**: WHEN a writer's set is read THE SYSTEM SHALL journal
  `witness_set_committed` with the criterion, the draw and the set's digest once,
  before the set runs on any mutant and before the criterion's first generation
  request, and reject a vote whose set is incomplete so that its ticket is
  reissued.
- **SP-1964~1**: WHEN the witnesses of a draw are answered THE SYSTEM SHALL run
  each set alone on the reference three times, run the sets that hold a passing
  witness on every planted refactor, and ask one `judge.scope` request of
  `judge_k` votes that shows the spec and every witness the reference passes, the
  refactor results applying after the scope verdicts and recorded per witness.
- **SP-1965~1**: WHEN witness sets run on a mutant or a refactor THE SYSTEM SHALL
  write their files into a workspace of that program only, run every set the
  memo does not hold in one batch, run each set alone when the whole batch
  errors, rerun every set one of whose witnesses failed and read a failure the
  rerun does not repeat as flaky, and memoize the outcomes per program and set.
- **SP-1966~2**: WHEN a pass proves the criteria THE SYSTEM SHALL let no criterion
  go past its witnesses while the writers or scope judges of a draw any search
  reads still owe an answer, ask the validity votes of the survivors no witness
  settles as the generation rounds go without holding back the next round, prove
  no criterion before every search ended, and settle every pool against the
  witnesses of every criterion.
- **SP-1967~1**: WHEN the footprint of a criterion is built in an ecosystem that
  runs witnesses THE SYSTEM SHALL take its entry functions from the units the
  criterion's runnable witnesses execute on the reference, and every unit of the
  code when they execute none or the ecosystem records no coverage, so that the
  code a generation request shows never depends on the tests.
- **SP-1968~1**: WHEN an operator mutant survives the criterion's tests and fails,
  reproduced, a block-bearing witness of the criterion THE SYSTEM SHALL put it in
  the pool whatever the differential search and the screen would say.
- **SP-1969~2**: WHEN the first or the further witness draw of a criterion is
  seeded THE SYSTEM SHALL draw its seed from the criterion's statement and the
  draw alone, never from the run seed, so that a CI run asks the requests the
  developer's host answered.
- **SP-1987~1**: WHEN a run produces its Statements in CI THE SYSTEM SHALL ask,
  for every criterion that runs witnesses, a credit draw of two writers seeded
  by the CI run seed, withhold the credit of the witnesses of every other draw,
  let neither the footprint, the stop rule nor the further draw read the credit
  draw, run every pool against its witnesses, and write no Statement before it
  is settled.
- **SP-1970~1**: WHEN a witness of the criterion fails on the reference in every
  run THE SYSTEM SHALL report a `reading_conflict` finding that names the
  criterion, the witness and its clause, and count it in the Statement, without
  changing the criterion's outcome.
- **SP-1971~1**: WHEN the ecosystem cannot run witnesses THE SYSTEM SHALL ask no
  witness, report `witness_runner_unavailable`, and take the footprint from the
  criterion's tests.
- **SP-1972~1**: WHEN the configuration names `admission.refactors` THE SYSTEM
  SHALL read every edit set of that directory as a planted refactor named after
  its file, admit witnesses with them, keep the setting when the configuration is
  written again, and write it on neither `init` nor `enable`.

## Compiled forms and fault classes

- **SP-1973~1**: WHEN the compiled forms of a mutant are compared THE SYSTEM SHALL
  call the mutant identical when every edited site compiles to the reference's
  form, different when one does not, and unknown when a site has no form or the
  mutant edits nothing.
- **SP-1974~1**: WHEN a Python unit is compiled for comparison THE SYSTEM SHALL
  digest its code object, nested code objects included, with every line table,
  first line and file name left out, so that a change of layout or position
  keeps the form and a change of behaviour changes it.
- **SP-1975~1**: WHEN a Go unit is compiled for comparison THE SYSTEM SHALL
  digest the assembly the compiler gives its function and closures with every
  source position left out, and give no form for a package that uses cgo or does
  not compile.
- **SP-1976~1**: WHEN the mutants of a pool are grouped by fault THE SYSTEM SHALL
  put the mutants that block or escalate and edit the same sites in one class when
  they fail the same witnesses, or, failing none, differ on the same inputs,
  leave sentinels and excluded mutants out, and list the classes in the
  Statement's `extensions.faults`.

## Facts records

- **SP-1977~1**: WHEN a Statement is written THE SYSTEM SHALL keep beside it, under
  the state directory, the facts record of its criterion: the policy, the
  admission, the reference results, the bindings, every witness with its runs and
  scope votes, and every pool mutant with its kill vector, examples, differential
  search, compiled form, witness outcomes and validity votes, so that the outcome
  of any test set and binding recomputed from the record equals the outcome
  evaluated on its standing.
- **SP-1978~1**: WHEN `facts show ID~REV` runs THE SYSTEM SHALL print the facts
  record of the last Statement written for the criterion, and refuse a criterion
  that has none.

## Witness hosts

- **SP-1979~1**: WHEN a pytest project briefs a witness writer THE SYSTEM SHALL list
  the headers of the public functions, classes and methods of the sources the spec
  mentions by module, with the annotated fields of a class, every public one when
  it mentions none, and the headers of the helpers and fixtures of every test
  file, and SHALL lay out a set as a test module beside the first test module of
  the project, each witness renamed after its short id and named the way the
  JUnit report names it.
- **SP-1980~1**: WHEN a Go module briefs a witness writer THE SYSTEM SHALL list the
  exported declarations of its packages the spec mentions without bodies, the
  helpers of every test file, and the package the spec mentions most, and SHALL
  lay out a set as an internal test file of that package with the imports the
  writer listed, `testing` included, each witness that declares exactly its own
  function renamed after its short id and named the way gotestsum reports it.
- **SP-1981~1**: WHEN a witness set is laid out and run on the reference and on a
  mutant that breaks what it checks, in a real pytest or Go project, THE SYSTEM
  SHALL read the witness passing on the reference and failing, reproduced, on the
  mutant.
- **SP-1985~1**: WHEN a Jest or Vitest project briefs a witness writer THE SYSTEM
  SHALL list the exported functions, the public methods of exported classes and
  the exported variables, classes and types of the sources the spec mentions,
  each under the specifier the witness file imports it by and without a body or a
  value, every exported one when it mentions none, and the exported declarations
  of every test file, and SHALL lay out a set as a test file beside the first test
  file of the project, in TypeScript or JavaScript as that file is, with each
  witness's function renamed after its short id and registered in one describe
  block, named the way the runner's JUnit report names it.
- **SP-1986~1**: WHEN a Maven or Gradle project, in Java or Kotlin, briefs a
  witness writer THE SYSTEM SHALL list the public methods, constructors, types and
  constants of the sources the spec mentions, qualified by their package and owner
  and without a body or a value, every public one when it mentions none, and the
  non-test methods of every test file, and SHALL lay out a set as a JUnit 5 test
  class in the package the spec mentions most, under the test source set of the
  source's language, with each witness's method renamed after its short id,
  annotated as a test and displayed under that name, named the way the build
  tool's JUnit report names it, and compiled by the build tool before it runs.

## Differential search

- **SP-170~2**: WHEN inputs are drawn for an operation THE SYSTEM SHALL give the
  same inputs for the same pool, signature and seed, sweep each parameter over
  every number of the code, the examples and the spec's text with its neighbours
  at full resolution (one unit, one written step, one step a decimal finer and
  half a written step on each side), and for a datetime parameter over the
  anchor day, every time of day and duration the spec names and every example
  datetime with their neighbours a microsecond, a second and a minute away,
  before it draws random combinations, never repeat an input and never exceed
  the number asked.
- **SP-171~1**: WHEN the observations on the reference and on a mutant are
  compared THE SYSTEM SHALL report no difference when they agree, and otherwise
  the simplest inputs on which the reference returns and the mutant differs, at
  most three.

## Scan promises

- **SP-2180~1**: WHEN the promises a host extracted are checked against the saved
  sources THE SYSTEM SHALL keep a promise only when its source was saved, still
  has the digest recorded for it, and holds its quote byte for byte, and SHALL
  report every other promise with the reason it was dropped: an unknown source,
  a source with another digest, a quote absent from its source, or a quote
  already kept.
- **SP-2181~1**: WHEN a promise is kept THE SYSTEM SHALL give it an id derived
  from its source name and its quote alone, so that the same promise has the
  same id in every invocation.
- **SP-2182~1**: WHEN at most three promises are chosen among the kept ones THE
  SYSTEM SHALL prefer those quoted from the change's description or a closing
  issue over those quoted from the README, then take the extraction order, and
  SHALL report each promise left out as unchecked.
- **SP-2183~1**: WHEN the calls of a scan are planned THE SYSTEM SHALL spend at
  most 18 of them in all, whatever the emissions already used and the asks the
  stage wants, and SHALL count a reissue like any other emission.
- **SP-2184~1**: WHEN the calls of a promise are planned THE SYSTEM SHALL
  reserve its two writers and its two scope votes before it emits any of them,
  and SHALL drop a promise the remaining budget cannot cover and report it as
  unchecked.
- **SP-2185~1**: WHEN reissues overrun the budget THE SYSTEM SHALL cut the
  labels ticket first, and SHALL keep the two scope votes of every promise it
  keeps.
- **SP-2186~1**: WHEN reissues overrun the budget beyond the labels ticket THE
  SYSTEM SHALL cut the generation tickets in the reverse of the order they were
  assigned, and SHALL name every cut ask and the reason.
- **SP-2187~1**: WHEN the generation tickets are assigned THE SYSTEM SHALL give
  them to the kept promises round-robin in their kept order, and none when no
  promise is kept.
- **SP-2188~1**: WHEN the planner is given the same emissions and asks in any
  order THE SYSTEM SHALL return the same plan, listed in a fixed order.
- **SP-2189~1**: WHEN a set of witnesses is admitted THE SYSTEM SHALL admit it
  only when it passes the reference on all three admission runs, and SHALL turn
  a set the reference fails into a question about the promise, never a finding.
- **SP-2190~1**: WHEN an admitted set runs again in the final stage THE SYSTEM
  SHALL let it back a claim only when all six runs passed, and otherwise turn it
  into a question that it is unstable on the reference and withhold its routes.
- **SP-2191~1**: WHEN a witness is judged for a promise THE SYSTEM SHALL hold it
  block-bearing only when it is admitted, stable on the reference, and both
  scope votes held its output fixed by the quote alone.
- **SP-2192~1**: WHEN a mutant is read against the witnesses of the promises THE
  SYSTEM SHALL call it a broken promise of a promise only when it is no
  sentinel, a block-bearing witness of the promise fails on it and the failure
  reproduced, and the whole suite passes on it three times out of three.
- **SP-2193~1**: WHEN labels map promises to functions THE SYSTEM SHALL label a
  test with a promise when one of the settled changes of the test sits in a
  function labeled with it, and SHALL ignore a label that names a promise not
  kept or a function outside the generation view.
- **SP-2194~1**: WHEN a test is labeled THE SYSTEM SHALL change nothing of it but
  its labels: no class, no reason, no change and no count.

## Coverage

- **SP-172~1**: WHEN coverage is queried THE SYSTEM SHALL list the units executed
  by given tests as entry functions, sorted and without duplicates, and the tests
  that executed a unit or a definition nested in it.

## Reading a pull request

- **SP-2200~1**: WHEN `scan <PR>` reads a pull request THE SYSTEM SHALL ask `gh`
  for its number, title, body, URL, base commit and head commit, and for each
  issue it closes the number, title, body, URL and state, and SHALL give them as
  data of the application layer that names no `gh` detail.
- **SP-2201~1**: WHEN the title or body of a pull request or an issue is read
  THE SYSTEM SHALL keep their bytes exactly as `gh` returned them, line endings,
  trailing spaces, non-ASCII text and text that reads as an instruction
  included.
- **SP-2202~1**: WHEN a pull request is read THE SYSTEM SHALL run only
  `gh pr view` and `gh issue view`, and SHALL read no diff from `gh`.
- **SP-2203~1**: WHEN the work tree is not at the head commit of the pull request
  THE SYSTEM SHALL refuse with a typed error that names both commits and
  `gh pr checkout <n>`, and SHALL neither fetch nor check anything out.
- **SP-2204~1**: WHEN the pull request is read in a work tree at its head THE
  SYSTEM SHALL give as the scan base the merge base of the head and the base
  commit of the pull request.
- **SP-2205~1**: WHEN the base commit of the pull request is not in the local
  repository THE SYSTEM SHALL refuse with a typed error that names the commit and
  `git fetch`.
- **SP-2206~1**: WHEN `gh` cannot be run THE SYSTEM SHALL refuse with a typed
  error that tells to install it, and WHEN `gh` is not signed in a typed error
  that names `gh auth login`.
- **SP-2207~1**: WHEN `gh` exits with a failure or prints an answer that is not
  the JSON asked for THE SYSTEM SHALL refuse with a typed error that carries
  `gh`'s message.
- **SP-2208~1**: WHEN a file is read from the head commit THE SYSTEM SHALL read
  it from the committed tree and not from the work tree, and SHALL report a path
  that is not a file of that tree as a typed error.
- **SP-2209~1**: WHEN named sources are saved THE SYSTEM SHALL write each one's
  exact bytes under `.spec-proof/scan/<commit>/sources/` of the repository, give
  its SHA-256 digest as the format computes it, and read the same bytes back.
- **SP-2210~1**: WHEN a source name is turned into a file name THE SYSTEM SHALL
  do it deterministically and without two names sharing a file, keeping every
  name that is safe, whatever characters it holds.
- **SP-2211~1**: WHEN a source name is empty, holds a NUL or has a `.` or `..`
  element, or the commit is not a full hexadecimal id THE SYSTEM SHALL refuse
  with a typed error and write nothing outside the sources folder.

## Scan with a host

- **SP-2220~1**: WHEN `spec-proof-dp scan` runs without `--host` THE SYSTEM SHALL
  call no model, open no host store and exit 0 once it wrote its reports, and
  SHALL write the same statement, headline, Markdown report, patches,
  reproduction scripts and finding records as a scan that has no host mode,
  the route killers of its finding folders being files of their own.
- **SP-2221~1**: WHEN a scan asks its host THE SYSTEM SHALL take as its
  quotable sources the README of the head commit and, for `scan <PR>`, the
  title and body of the pull request and of each issue it closes, byte for
  byte, save them under `.spec-proof/scan/<commit>/sources/`, which no
  reference copy holds, and SHALL never take the change's diff as a source.
- **SP-2222~1**: WHEN a scan asks its host for its promises THE SYSTEM SHALL ask
  one extraction ticket that shows each source under its name with its exact
  text and, for a change, the paths of its changed files and the public
  signatures of its changed functions, never a hunk or a body, and SHALL wait
  with exit 3 until it is answered.
- **SP-2223~1**: WHEN a scan that asks its host has no quotable source THE
  SYSTEM SHALL ask no extraction ticket, hold its promises checked with none
  kept, and finish in the same invocation.
- **SP-2224~1**: WHEN the extraction is answered THE SYSTEM SHALL check each
  quote on the saved bytes, keep at most three promises, ask two blind writers
  for each as the budget allows, journal the digest of each set a writer
  returns before it runs anywhere, and reject and ask again a set that is not 3
  to 6 examples and 1 to 3 properties.
- **SP-2225~1**: WHEN the writers of a promise answered THE SYSTEM SHALL build
  the reference, run each witness set alone on it three times, admit the
  witnesses that pass every run, make each witness the reference fails a
  question, and remember the admission and the coverage of the witnesses in a
  memo keyed by the file-set digest of the tracked files and the ecosystem.
- **SP-2226~1**: WHEN the witnesses of the promises are admitted THE SYSTEM
  SHALL ask, as the budget allows, two scope votes per promise about its
  admitted witnesses, four generation tickets round-robin over the promises,
  each showing the quote and the code the promise's admitted witnesses run,
  narrowed in a change to the functions it touches, and one labels ticket
  that shows the promises and every function of those views.
- **SP-2227~1**: WHEN a stage of a scan that asks its host has nothing to ask
  THE SYSTEM SHALL go on to the next stage in the same invocation.
- **SP-2228~1**: WHEN an invocation of a scan that asks its host leaves tickets
  waiting THE SYSTEM SHALL finish the wave of its session once, retire what
  the session no longer wants, print the wave, the session and how many of
  its own tickets wait, and exit 3.
- **SP-2229~1**: WHEN the final stage reads a generation answer none of whose
  mutants splices, changes the code, calls no sink and builds THE SYSTEM SHALL
  reject it, ask it again and exit 3 before any run of the suite.
- **SP-2230~1**: WHEN a scan that asks its host finishes THE SYSTEM SHALL run
  the suite, its coverage, the draw, the mutants and their evidence, the
  classification and the confirmation in that one invocation, and in no
  earlier one.
- **SP-2231~1**: WHEN model mutants join the draw THE SYSTEM SHALL add at most
  20 of them, with origin model, after the sentinels and the operator mutants,
  each mutating the lines of the reference its splice changes, and keep a
  mutant the operators already drew as theirs.
- **SP-2232~1**: WHEN a mutant no test killed ran THE SYSTEM SHALL run the
  witness sets that hold a stable witness on it in a workspace of their own and
  prove it by a `witness_failure` route of each stable witness that fails on
  it, reproduced.
- **SP-2233~1**: WHEN the final stage starts THE SYSTEM SHALL run each admitted
  set three more times on the reference with no memo, and SHALL make a witness
  that fails any of those runs a question that it is unstable on the
  reference, with no route and no block.
- **SP-2234~1**: WHEN the lines a change of a file mutates are read THE SYSTEM
  SHALL give the reference lines between the first and the last that differ,
  the line an insertion comes before or the last line for an append, and none
  for an unchanged file.
- **SP-2235~1**: WHEN a mutant that is no sentinel, that no test killed and
  that fails a block-bearing witness ran THE SYSTEM SHALL run the whole suite
  on it three times, a run passing when every test green on the reference
  passed it, and record the promise it breaks when every run passed.
- **SP-2236~1**: WHEN the labels are answered THE SYSTEM SHALL label each test
  with the promises labelling the function of a mutant among its settled
  changes and change no class, reason, change or count.
- **SP-2237~1**: WHEN a scan that asks its host writes its statement THE SYSTEM
  SHALL end its session, and WHEN a scan opens a session without `--resume` it
  SHALL end every other open scan session and never the prove session.
- **SP-2238~1**: WHEN a scan that asks its host opens its session THE SYSTEM
  SHALL key it by a digest of its commit, tree digest, mode, base, pull
  request, seed, counts, budgets, module version and source digests.
- **SP-2239~1**: WHEN `--resume` finds no open scan session with the binding of
  the scan THE SYSTEM SHALL exit 2 naming, field by field, what differs from the
  binding of the open scan session, sources added, removed or changed
  included, and with no open scan session SHALL say there is nothing to
  resume.
- **SP-2240~1**: WHEN the reference behaves differently from one invocation to
  the next THE SYSTEM SHALL ask the same requests of the extraction, the
  writers, the scope votes, the generation and the labels in every invocation
  of a session whose memo holds.
- **SP-2241~1**: WHEN the memo of the witness facts is lost in the middle of a
  session THE SYSTEM SHALL only ask again the requests the new admission
  changes, never the extraction or the writers, and finish the scan.
- **SP-2242~1**: WHEN `prove` and a scan that asks its host share one file host
  store THE SYSTEM SHALL keep each session's tickets out of the other's
  pending tickets and retire none of the other session's tickets.
- **SP-2243~1**: WHEN a scan that asks its host plans its asks THE SYSTEM SHALL
  read the emissions of its session from the journal, each as the ask its
  queue tag names, let the budget planner decide what to issue, cap the
  session at 18 emissions as a backstop, and report the asks it cut and the
  model calls it used.
- **SP-2244~1**: WHEN `spec-proof-dp scan` is given `--host`, `--resume` or a
  pull request number THE SYSTEM SHALL ask the host with either flag, take at
  most one positive number, refuse a number beside `--base`, and refuse
  anything else as a usage error.
- **SP-2245~1**: WHEN a scan that asks its host waits THE SYSTEM SHALL print
  only the wave, the session and the count of waiting tickets, and print its
  headline only once it finished.
- **SP-2247~1**: WHEN a scan that asked its host is rendered THE SYSTEM SHALL
  list each checked promise with its quote and source.
- **SP-2248~1**: WHEN a scan that asked its host broke a promise THE SYSTEM
  SHALL render the broken promise with its quote, its change, the witness that
  fails on it and the path of the script that runs that witness.
- **SP-2249~1**: WHEN a witness is a question THE SYSTEM SHALL render it with the
  quote of its promise and whether the reference fails it or it is unstable on
  the reference.
- **SP-2250~1**: WHEN asks went unanswered THE SYSTEM SHALL render each promise
  left out over the cap or the budget, each ask the planner cut, each ask the
  call cap left unasked and how many extracted quotes the check dropped.
- **SP-2251~1**: WHEN a scan that asked its host found no quotable promise THE
  SYSTEM SHALL print "no quotable promise in the README, description or
  closing issues", and when the host never answered the extraction that the
  promises were not checked.
- **SP-2252~1**: WHEN a scan is rendered THE SYSTEM SHALL end its footer with
  "no model used · promises not checked" without a host, and with the model
  calls used out of the budget and whether promises were checked with one.
- **SP-2253~1**: WHEN a test that verifies nothing carries labels THE SYSTEM
  SHALL render under its finding the quotes of the promises it may break.
- **SP-2254~1**: WHEN a finding folder is written THE SYSTEM SHALL offer each
  route's killer as a test that fails on the mutant: the killing test of the
  suite with a script that runs it on the mutant, the admitted witness laid
  out as a test file with such a script, and the operation, input and results
  of a differential route for `scan replay`; never a test a host wrote for
  the mutant.
- **SP-2255~1**: WHEN a scan broke a promise THE SYSTEM SHALL write its folder
  with the patch, the witness laid out as a test file, the script that runs it
  on the mutant and the record a replay reads.
- **SP-2256~1**: WHEN a `witness_failure` route is replayed THE SYSTEM SHALL lay
  out the recorded witnesses through the ecosystem's witness host, briefed
  with the recorded promises, and hold the route only when the witness passes
  the reference and fails the mutant three runs out of three.
- **SP-2257~1**: WHEN a broken promise is replayed THE SYSTEM SHALL hold it only
  when its witness passes the reference and fails the mutant three runs out of
  three, the mutant builds and the whole suite passes it three times.
- **SP-2258~1**: WHEN a finding is replayed THE SYSTEM SHALL use no host store
  and no memo, so that a replay holds with the host directory and the memo
  deleted.
- **SP-2259~1**: WHEN a scan session opens THE SYSTEM SHALL journal its binding
  once, in the session's name, so that a later resume can name what changed.
- **SP-2261~1**: WHEN `scan <PR>` runs, with or without a host, THE SYSTEM SHALL
  scan the change from the merge base of the head and the pull request's base
  commit.
- **SP-2262~1**: WHEN `prove` or a scan assembles a host request THE SYSTEM SHALL
  build it the one same way, the prompt template and the decision policy among
  its sorted, distinct inputs, so that the same work always has the same
  request id.
- **SP-2270~1**: WHEN a scan records the coverage of the witnesses of a Go
  module THE SYSTEM SHALL build the coverage binary of each package that holds
  a witness, run its start-up once with no test and each witness alone, and
  name for each witness the innermost definitions of the module holding a
  block it ran more often than the start-up did, a witness the reference
  fails included.
- **SP-2271~1**: WHEN a scan wires the runner of its witnesses THE SYSTEM SHALL
  record their coverage with the ecosystem's recorder of tests run alone when
  it has one, else with its recorder of the whole suite, and `prove` SHALL
  record coverage with the recorder of the whole suite only.
- **SP-2272~1**: WHEN a generation or labels request of a scan shows code THE
  SYSTEM SHALL keep its view within 48 KiB of unit source, whole units in the
  order of the code, leaving out each unit that would take it past the bound,
  SHALL say in the request how many units it leaves out, and SHALL name in the
  report each view the bound cut with how many of its functions it shows.
- **SP-2273~1**: WHEN a scan runs the witnesses of a Go module THE SYSTEM SHALL
  run them with `go test -json`, which needs no tool the module declares, read
  each witness's outcome by its package and test name, and fail the
  collection when a witness file does not compile; an ecosystem without a
  runner of witnesses runs them with its suite runner.
- **SP-2274~2**: WHEN `spec-proof-dp scan --host` runs on a pytest, a Go, a
  Jest or a Vitest project committed in its own repository, whose README holds
  a promise the
  suite runs and never asserts and a promise the code does not keep, and a
  host answers each ticket the scan lists for its session THE SYSTEM SHALL
  exit 3 until every answer is in and then exit 0, print the broken promise
  with its quote, a change the whole suite passes 3/3 and the script whose
  witness fails on that change, print each witness the reference fails as a
  question, count a quote its source does not hold as dropped, and write a
  statement the scan format admits with its promises checked.
- **SP-2275~1**: WHEN the host directory and the memo of a scan that asked its
  host are deleted THE SYSTEM SHALL still hold, on `scan replay`, the
  `witness_failure` route of a finding and the broken promise.
- **SP-2276~2**: WHEN a scan of a whole repository that asks its host shows a
  generation ticket of a promise whose witnesses were admitted, in a pytest, a
  Go, a Jest or a Vitest project THE SYSTEM SHALL show the functions those
  witnesses run and no other.
- **SP-2277~1**: WHEN `spec-proof-dp scan --resume` runs with a flag that
  differs from the open scan session's THE SYSTEM SHALL exit 2 naming the
  field, its old value and its new one.
- **SP-2278~2**: WHEN the host of a scan of a pytest, a Go, a Jest or a Vitest
  project answers
  every ticket with an answer its schema accepts and the scan rejects THE
  SYSTEM SHALL ask the rejected tickets again while the session's emissions
  allow, make at most 18 emissions in the session, report the asks the cap
  left out and the calls used, and finish with exit 0.
- **SP-2279~2**: WHEN a scan that asks its host and `prove` wait for the host
  in one pytest, Go, Jest or Vitest repository at the same time, each answered
  between its
  own invocations THE SYSTEM SHALL finish both, and no invocation of the scan
  SHALL retire a ticket `prove` waits for.
- **SP-2280~1**: WHEN `spec-proof-dp scan --host <PR>` runs with gh answering
  for the pull request and the issue it closes THE SYSTEM SHALL read both
  through gh, show the description and the issue byte for byte as sources
  of the extraction, save the sources of the head commit, scan the change
  from the merge base and check a promise of each source.
- **SP-2281~1**: WHEN a generation answer of a scan proposes a source that ends
  with line breaks THE SYSTEM SHALL read it as the text of its unit, which
  ends with its last character, so that the line breaks alone never make a
  proposal unusable.

## Prompts

- **SP-173~2**: WHEN a request prompt is built THE SYSTEM SHALL name no criterion
  by its identifier, the spec documents and statements of a witness or scope
  request naming every criterion by a label of its position instead, show a
  generation request no test and no witness, show a witness request no body of
  the code, no test, no mutant and no kill, show a scope request the spec and the
  witnesses alone, change the template digest exactly when the prompt text or the
  response schema changes, and carry at most 64 questions in a closed request.
- **SP-174~2**: WHEN an executor's output is read THE SYSTEM SHALL give the
  generated mutants, the judged answer with its confidence and witness, an
  equivalence answer other than EQUIVALENT as undetermined, the screen answers by
  question id, the witness set of a writer only when it holds 3 to 6 examples and
  1 to 3 properties with distinct names, and the scope verdicts only when every
  witness shown is answered, and a vote that states no confidence SHALL count as
  full confidence.
- **SP-2162~1**: WHEN a `generate.promises` prompt is built THE SYSTEM SHALL show
  each source under its name with its exact text, show a change the paths of its
  changed files and the public signatures of its changed functions and nothing
  else of its code, never a hunk or a body, change the template digest exactly
  when the prompt text or the response schema changes, and give a response
  schema that requires a non-empty quote and a source of the closed set of
  source names for each of at most 8 promises, with no other member.
- **SP-2163~1**: WHEN the output of a `generate.promises` request is read THE
  SYSTEM SHALL give the promises as quote and source in the order stated, and
  refuse a promise whose source the request did not show, a promise with an empty
  quote and a list of more than 8 promises.
- **SP-2164~1**: WHEN a `judge.labels` prompt is built THE SYSTEM SHALL show each
  promise under its opaque id with its quote and each function of the generation
  view under its opaque id with its path, signature and code, change the template
  digest exactly when the prompt text or the response schema changes, and give a
  response schema that maps promise ids to lists of distinct function ids, with
  no other member.
- **SP-2165~1**: WHEN the output of a `judge.labels` request is read THE SYSTEM
  SHALL refuse a function id outside the view and a promise id outside the input,
  and otherwise give for every promise of the input the functions it labels, each
  once and in the order of the view, a promise the response leaves out labelling
  none.

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
- **SP-182~2**: WHEN an honest host answers every request THE SYSTEM SHALL block a
  criterion whose weak test lets a VALID_e mutant survive and pass criteria whose
  tests kill every VALID_e mutant, writing one Statement per criterion.
- **SP-183~1**: WHEN the host forges, omits, replays with a stale nonce or breaks the
  schema of its answers THE SYSTEM SHALL never pass a criterion.
- **SP-184~2**: WHEN any assignment of validity and scope answers is considered,
  including the most favourable one, THE SYSTEM SHALL never block or credit a
  mutant that fails no witness and no example, never clear a survivor on a
  judgment alone, and never turn a block into a pass when a witness failure is
  added.
- **SP-185~1**: WHEN the same votes are consumed in any order THE SYSTEM SHALL write the
  same Statement, with no nonce, time or consumption order in it.
- **SP-186~1**: WHEN any input of a proof changes THE SYSTEM SHALL give another cache key,
  the inputs being the statement, the test sources, the code tree, the reference
  results, the examples and their bindings, the policy, the prompt templates, the
  mutant set, the models and the host responses.
- **SP-187~1**: WHEN a criterion has no host tickets left THE SYSTEM SHALL leave the
  judgments it could not ask for UNDETERMINED with the reason `budget_exhausted`
  and escalate the criterion, never pass it.
- **SP-188~2**: WHEN a criterion's inputs are unchanged since its last Statement and
  `--changed` selects it THE SYSTEM SHALL write it no new Statement and report its
  stored outcome.
- **SP-189~1**: WHEN a Statement is written THE SYSTEM SHALL validate it against the
  embedded schemas, write it under `.spec-proof/cache/discriminating-power`, and
  keep mutant bodies only in the user cache outside the work tree.
- **SP-190~3**: WHEN no differential search can run, because the ecosystem cannot
  probe calls or the project binds no operation, THE SYSTEM SHALL still ask for the
  witnesses, the scope verdicts, the generated mutants and the validity votes of
  the survivors no witness settles, report the skipped searches as findings, and
  escalate every criterion that has no credit-bearing mutant killed.

## Configuration and command line

- **SP-191~1**: WHEN the project configuration names operations and policy parameters
  THE SYSTEM SHALL bind each operation per ecosystem or language, apply the
  parameters over the defaults, refuse an invalid parameter, and write both back
  in a fixed order.
- **SP-192~2**: WHEN `request`, `systemone` or `doctor` runs THE SYSTEM SHALL list,
  show and submit tickets, export closed requests and import their answers from a
  relay under its source `system-one:<model>@<endpoint digest>`, refusing another form, report the checks of the project, and exit 1 for a refusal of the
  protocol and 2 for a tool failure.
- **SP-193~1**: WHEN `prove` is given conflicting flags or unexpected arguments THE
  SYSTEM SHALL refuse them before it runs anything.
- **SP-1610~1**: WHEN the binary is run with `-h`, `--help` or `help`, a command
  with `-h` or `--help`, or `help` followed by a command THE SYSTEM SHALL print the
  commands of the binary, or the flags and the synopsis of that command, on the
  output stream and exit 0; WHEN a command that takes a verb is asked for help
  THE SYSTEM SHALL list its verbs; WHEN a verb of `request`, `systemone` or `host`
  is asked for help THE SYSTEM SHALL list exactly the flags that verb defines and
  refuse, as a usage error that prints the usage of the verb, a flag another verb
  owns.
- **SP-1611~1**: WHEN `prove` prints a line of a criterion that has no result yet
  THE SYSTEM SHALL print no trailing space on it.

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
- **SP-1938~1**: WHEN `hook pre-tool --harness codex` is run on a spawn of one of the
  module's executor agents THE SYSTEM SHALL deny the spawn unless it asks for a child
  with none of the main agent's history, with a reason that tells the agent to spawn
  it again with `fork_turns` set to `"none"`, journal nothing for it, give no
  decision for a blind spawn or a spawn of another agent, and the committed Codex
  hook entries SHALL select the spawn tool under each of its names beside the
  executor tools.
- **SP-1874~3**: WHEN the fake-host personas of Copilot CLI drive the module
  through its hooks and its executor server THE SYSTEM SHALL journal the
  allowlist and bare-launch observations for an executor whose session log shows
  the pinned CLI, exactly the two executor tools, only the fetch and submit of
  its own ticket, submitted output matching the stored response, a bare launch
  with the shipped agent body and a bare receipt; credit that unmarked answer
  as `restricted`, never `restricted-attested`; and leave the wide-selection,
  stale-cli and padded-launch answers `restricted-unverified`, journaling their
  failures and retaining every mark.
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

- **SP-243~2**: WHEN `init` runs THE SYSTEM SHALL require a Shallnot whose report
  schema is 1.2 or later, write the module's configuration with the commented
  example of the operations block and ignore its local state (the statement cache,
  the verified set and the pending files), install the files of each selected harness (`auto`, `none` or a list of
  Claude Code, Codex and Copilot) by merging into every file another tool shares
  and writing the files it owns, plan no change on a second run, and leave
  Shallnot's configuration, its block of AGENTS.md and its hooks as they were.
- **SP-244~2**: WHEN `enable` or `disable` runs THE SYSTEM SHALL turn the proof on or
  off in the module's configuration, add or remove its kind in `proofs.require` of
  `shallnot.yaml` and its block of AGENTS.md by line edits that keep every other
  byte, set `proofs.trust` to `local` and list the source roots of the project in
  `proofs.code` when `shallnot.yaml` sets neither, so that disabling removes what
  enabling added to `proofs.require` and to AGENTS.md and leaves the trust and the
  code, refuse a project with no `shallnot.yaml`, and refuse a layout it cannot
  extend without rewriting.
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
- **SP-249~2**: WHEN `prove --ci` runs THE SYSTEM SHALL draw the seed of the head
  commit from the first CI run, record it in `.spec-proof/seeds/<head sha>.json`,
  outside the cache of statements, and reuse it on every later run of the
  commit, produce the Statements as CI, serve the tickets it is asking for from the
  bundle given with `--import-host` through the same checks as any answer, ignore
  what the bundle holds that it does not ask for, leave the other tickets pending,
  mark again what the pushing host marked, refuse a bundle of another version that
  left requests unanswered, and leave the requests that remain in
  `.spec-proof/ci-issued/discpower`, where the CI job uploads them from.

## End to end

- **SP-250~2**: WHEN a project is equipped with Shallnot and the module, edited and
  proved THE SYSTEM SHALL have the end-of-turn hook of Shallnot hold the turn with
  `awaiting_host` naming the module's skill while the host owes answers, let a
  simulated Claude Code host answer every ticket through the real hooks and the
  real executor server until `prove` ends with a pass for the criterion whose tests
  kill every VALID_e mutant, an escalation for the criterion whose survivors no
  witness fails and no execution excludes, and a block for the weak one, journal
  the executors as started, fetched, accepted, attested and reported with no leak,
  exposure or conflict, deny a main-session call of an executor tool, and have
  `shallnot check` refuse the project with a surviving valid mutant of the weak
  criterion and an escalated mutant of the escalating one, and no finding for the
  passing one.
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
- **SP-407~2**: WHEN a Statement lists its inputs THE SYSTEM SHALL name the `spec`
  input `ID~REV` with the statement digest, list as `test_source` descriptors the
  files that hold a test bound to the criterion and those that hold a test whose
  execution reaches the footprint under the reference coverage (every file that
  holds a bound test when no footprint restricts the scope), and list as
  `code_tree` descriptors the files of the footprint, each by its path relative to
  the directory holding `shallnot.yaml` and the SHA-256 of its exact bytes.
- **SP-408~1**: WHEN a screen request is built THE SYSTEM SHALL build its question
  set and its response schema through the host protocol's closed question type, with
  one choice question per requirement whose options carry their criteria text, and
  WHEN a relay imports the response of a System One endpoint THE SYSTEM SHALL
  translate it with the question set of the ticket and refuse a response that
  leaves a question unanswered or picks no option.
- **SP-902~1**: WHEN `tau_judge` or `tau_screen` is set THE SYSTEM SHALL accept a
  bare number or a mapping of `default` and of confidence sources to numbers, as
  Shallnot's `proofs.thresholds` does, hold the host judges to the threshold of
  `host-votes(<judge_k>)` and the screen answers to the threshold of the source of
  their probabilities (`host-votes(<k>)` for k votes, `system-one:<model>@<endpoint
  digest>` for a relayed answer), resolving the exact source, then its kind, then
  the default, refuse a value outside [0,1] and a key that names no source, and
  record the threshold a mutant's decision was held to in
  `extensions.decision.threshold`.

## Portable plugin

- **SP-1710~1**: WHEN the portable plugin of the module is generated THE SYSTEM SHALL
  give it the manifest, the server of `spec-proof-dp`, the fulfil skill and the agents and
  hooks of Copilot CLI that the shared generator produces, the root server naming
  no harness and the agents' servers named apart from it, and the committed tree
  SHALL equal what the generator produces; the Copilot tree under `plugin/` is the
  portable one.
- **SP-1713~1**: WHEN `mcp --role executor` runs with no `--harness` THE SYSTEM
  SHALL serve the portable harness: the two executor tools with no `drain`, the
  project learned from the client's roots, and a refusal that says so for a
  directory with no project of the module.

## Release version

- **SP-1851~1**: WHEN `spec-proof-dp` is run with the lone argument `--version` THE SYSTEM
  SHALL print the bare version, the one a release build linked with
  `-X main.version`, else the one `go install` recorded, else `dev`, and the
  `version` command SHALL print the binary name, that version and the linked
  commit when the build has one.

## Claude Code classification

- **SP-1910~1**: WHEN `systemone classify-plan` runs with a model, or with voters
  whose counts sum to five, THE SYSTEM SHALL list for each closed request that
  waits for its first vote its ticket and nonce, the confidence source
  `host-classify(5,rot1):<voters>`, the digest of its question set and, for each
  question and each of the five calls, the model that answers it, the text with
  the options in the planned order, the labels in that order and the seed input
  of the order; and SHALL refuse a call that names neither or both of a model and
  voters, and voters that do not make a valid source.
- **SP-1916~1**: WHEN the switch is `auto` and the admission table holds an entry
  for the module THE SYSTEM SHALL ask the first vote of a screen request on its
  own so that a classification can answer it; WHEN the classification flags a
  survivor THE SYSTEM SHALL keep the survivor in the pool and record the
  classification in the operator screen; WHEN it lets a survivor go but is not
  credited by the isolation floor, or its exact source has no threshold in
  `tau_screen`, THE SYSTEM SHALL reject it and let the host's deciders answer the
  request; and WHEN the response is credited and its source has a threshold of its
  own THE SYSTEM SHALL let it screen a survivor out.
- **SP-1917~1**: WHEN the configuration names `host.classify.witness_dir` THE
  SYSTEM SHALL read it, keep it when the configuration is written again, and check
  the classifications that answered a request against the raw API log in that
  directory when a run resumes.
- **SP-1930~1**: WHEN the committed Claude plugin of the module is read THE SYSTEM
  SHALL ship the hooks module that carries the classifier calls of the host,
  naming the binary of the module, and the validator of Claude Code SHALL accept
  the plugin.

## Scan verdicts

- **SP-2020~2**: WHEN the runs of a test are settled THE SYSTEM SHALL answer
  `not_started` when exactly three runs all did not start, `killed` when exactly
  three runs all failed, `survived` when exactly three runs all passed, and
  `inconclusive` for any other runs, a timeout, an error, a missing run, runs
  that disagree and any mix with a run that did not start included; SHALL record
  `killed`, `survived` and `inconclusive` as the outcome of a change and never
  `not_started`; SHALL hold a test green only when exactly three runs on the
  reference all passed; and SHALL never count a run that did not start as a
  failure on a sentinel.
- **SP-2021~1**: WHEN a test kills a mutant THE SYSTEM SHALL take the kill as a
  `killed_by_test` route only when the test is green, the kill is settled as
  `killed`, and the test failed no run on any sentinel.
- **SP-2022~1**: WHEN the differential search found a difference on a mutant THE
  SYSTEM SHALL take it as an `f3_difference` route only when it names the
  operation it called and an input, was observed on a return value, a raised exception or a mutated argument,
  gives two different results, reproduced, and the reference agreed with itself
  on the input.
- **SP-2023~1**: WHEN a witness failed on a mutant THE SYSTEM SHALL take it as a
  `witness_failure` route only when the witness is admitted, its writer was
  blind, and the failure reproduced; SHALL never prove a sentinel, a mutant
  identical to the reference in compiled form or a mutant that mutates no line;
  and SHALL list each route of a mutant once, in the format's order.
- **SP-2024~2**: WHEN the pairs of a test are read THE SYSTEM SHALL take each
  proven mutant one of whose mutated lines is among the lines the test's run
  executed, start-up included, with the test's settled outcome on it; SHALL list
  as a change, with its site, the first mutated line by file then number, a
  pair settled `killed` or `inconclusive`, and a pair settled `survived` only
  when the test owns one of the mutant's mutated lines; SHALL list apart, as a
  mutant that stopped the test from starting, a pair settled `not_started` on a
  line the test owns; SHALL ignore every other pair; and SHALL order the changes
  by site then mutant id and the mutants that stopped the test by id.
- **SP-2025~1**: WHEN a test is classified THE SYSTEM SHALL classify a test that
  is not green as `not_green` and a green test the scan did not draw as
  `not_sampled`; and a sampled green test, in order, as `unproven` with the
  reason `kills_sentinel` when it failed a run on a sentinel, as `caught` when it
  kills one of its changes, as `unproven` with the reason `inconclusive_outcome`
  when one of its changes is inconclusive, as `verifies_nothing` when it survives
  at least 5 changes on at least 2 sites, and as `unproven` with the reason
  `too_few_changes` otherwise.
- **SP-2026~1**: WHEN a scan is classified THE SYSTEM SHALL count every test of
  the suite, those with reference runs and those sampled, in exactly one of the
  five classes, so that the judged tests are the sampled green ones and the
  tests not sampled are the other green ones.
- **SP-2027~1**: WHEN a scan is classified THE SYSTEM SHALL rank the verdicts by
  class, verifies nothing first, then by number of changes, most first, then by
  test id byte-wise; SHALL give the same result whatever the order of the
  sampled tests, the mutants, their lines and differences and the covered lines;
  and SHALL refuse two mutants with one id.
- **SP-2028~1**: WHEN a test kills one more mutant THE SYSTEM SHALL never move
  that test into `verifies_nothing`; and WHEN an unproven mutant that is not a
  sentinel is removed from a scan THE SYSTEM SHALL change no verdict and no
  count.
- **SP-2029~2**: WHEN the verdicts of a scan are written as scan tests THE SYSTEM
  SHALL write the sampled green tests in test id order, each change, and apart
  from them each mutant that stopped the test from starting, in mutant id
  order, so that a scan statement holding them, the counts and the routes
  validates in the format.
- **SP-2030~1**: WHEN an edit set is written into a scan statement THE SYSTEM
  SHALL write its edits in canonical order with the target of a definition
  omitted, so that their scan digest is its mutant id.
- **SP-2133~2**: WHEN a scan is classified THE SYSTEM SHALL change no class, no
  reason, no change, no count and no route by a survival on a line a test ran
  only at start-up or by runs that did not start; SHALL count among the changes
  of a test that verifies nothing exactly its survivals of proven mutants on
  lines it owns; SHALL hold a test with an inconclusive outcome on a line it ran
  only at start-up unproven, and a test that kills a change on such a line
  caught; SHALL hold a test whose coverage names no owned line as owning
  none, and a line outside the lines a test ran as one it does not own; SHALL
  create no kill, no route, no inconclusive outcome and no test that verifies
  nothing when a test owns fewer lines; and SHALL give the same judgement
  whatever the order of the lines a test ran and owns.

## Scan

- **SP-2100~4**: WHEN the line coverage of a pytest suite is recorded THE SYSTEM
  SHALL run the suite once under a coverage plugin placed only on the import path
  of the workspace, or, for a recording of each test alone, one session per
  test that selects it and no other test as SP-2102 selects tests, with
  `--continue-on-collection-errors --maxfail=0`, record
  line events over the setup, call and teardown of each test and never during
  collection, and give per test, by node id, the lines of the project's source
  files it ran, never a line of a test file; SHALL give as owned only the lines
  among them that ran on the thread that runs the test's protocol, in its setup
  or call and never its teardown, outside the setup of a fixture whose scope is
  wider than the function, and outside the body of a module whose import began
  on that thread during the test's protocol and has not returned or unwound,
  following every module body to its end whatever its file; SHALL stop a code
  location from reporting for the rest of a test only once it recorded it as
  owned; and SHALL derive from the same run the functions of SP-161, with
  `sys.monitoring` where the interpreter has it and `sys.settrace` otherwise,
  both giving the same owned lines.
- **SP-2101~4**: WHEN the line coverage of Go tests is recorded THE SYSTEM SHALL
  build the test binary of each package once in the workspace with coverage of
  every package of the module in atomic mode; run from its package directory,
  three times each, on the scan's workers and once for every leaf that needs
  it, each baseline of a binary: its start-up, with no test, and each distinct
  ancestor of a recorded leaf, alone with no subtest; run each test alone from
  its package directory, anchored to its subtest path, on the same workers; and
  give every line of each block its cover profile counts as run, summing a
  block the profile lists more than once, as paths relative to the module root,
  never a line of a file outside the module; SHALL hold a block stable when the
  three runs of the leaf's immediate baseline, its parent's or the start-up
  for a top-level test, and of every baseline above it give it the same count,
  and let a leaf own a stable block it ran more often than its immediate
  baseline; SHALL hold that a block holds code on a line when its span there,
  read from the source, has a character other than whitespace or a brace, and
  give as owned a line on which the leaf owns a block holding code and owns
  every block holding code there that it ran; SHALL give as owned the line of
  a `case` or `default` clause header of a switch, type switch or select that
  is written whole on one line, from its keyword to its colon, that no block
  of the profile holds code on and that no `fallthrough` of the previous
  clause enters, when the leaf owns the block that starts at the colon, and
  never a line of a header written on several lines; and SHALL give a leaf no
  owned line when one of its baselines left no profile, exited non-zero or
  timed out.
- **SP-2102~4**: WHEN a scan runs a selection of tests, or the whole suite, a
  number of times THE SYSTEM SHALL address each test by its runner-native id, a
  pytest node id or, for Go, the test file and the subtest path of a leaf test,
  never a parent; run each run of pytest in a session of its own, or as SP-2142
  gives, bounded by the timeout of one run, a selection naming the distinct test
  files of its tests in order, the plugin keeping the selected node ids alone
  and deselecting the others, with `--continue-on-collection-errors
  --maxfail=0`; run the runs of each Go package first in one `go test -count`
  whose `-run` expression selects exactly the selected subtest paths, bounded
  by that timeout times the number of runs, then as SP-2134 and SP-2141 give;
  list the collected tests with their files for a run of the whole suite; give
  each test one result per run, a pass only for a pass and a failure only for a
  failure, not started only as SP-2134 and SP-2135 give, and errored for a skip,
  an error or a test a run did not report otherwise; let a test module that does
  not import leave the other tests of the run running; and give every run a
  timeout stopped before it reported a test timed out, starting no later
  pytest run.
- **SP-2103~1**: WHEN a definition a mutant edits is bound for the differential
  search of a scan THE SYSTEM SHALL bind a top-level Python function to
  `package.module:function`, a `src/` layout read from below `src`, and an
  exported top-level Go function to `import/path:Function`, and SHALL bind no
  method, nested function, unexported Go function or test; and SHALL give, for
  each test, the command line that runs it alone from the project root.
- **SP-2104~1**: WHEN a scan samples tests THE SYSTEM SHALL draw up to the sample
  size, by default 40, stratified by file: the files and the tests of each file
  shuffled by the seed, one test of each file in turn, so that no file gives a
  second test before every file gave one; every test when there are fewer; and
  the same sample, sorted by test id, for the same seed whatever the order of
  the tests.
- **SP-2105~1**: WHEN a scan draws THE SYSTEM SHALL derive the operator seed of
  every draw from the scan seed under a domain only scans use, so that a scan
  seed gives one operator seed, never the seed of a local proof nor the scan
  seed itself.
- **SP-2106~1**: WHEN the lines of a mutant are computed THE SYSTEM SHALL give
  every line from the one that holds the first byte of its mutated span to the
  one that holds its last, the line of the insertion for an empty span, and no
  line for a span outside the file.
- **SP-2107~2**: WHEN a scan chooses what it may mutate THE SYSTEM SHALL refuse
  test files and the files of `tests`, `test`, `testdata`, `__tests__`,
  `__mocks__` and `__fixtures__` directories, vendored and installed code,
  generated code marked `Code generated ... DO NOT EDIT.` or `@generated`, and
  migrations; and SHALL refuse a site with a line that calls a print, a log, a
  `console` method or a metric.
- **SP-2108~2**: WHEN a repository is scanned THE SYSTEM SHALL refuse a work tree
  whose tracked files differ from the head commit before running anything; copy
  the work tree, scan reports aside, into a workspace outside it and build it,
  failing when the reference does not build or collects no test; run the whole
  suite three times there; and record the line coverage of every green test, or,
  for a repository scan whose coverage scope is its sample, of the sampled tests
  only.
- **SP-2109~1**: WHEN a scan plans what it judges THE SYSTEM SHALL, without a
  base, sample the green tests by the seed and admit for mutation the lines the
  sampled tests ran; and, with a base, take the green tests that ran a line of a
  function whose lines differ from the base, sample them, and admit only the
  lines of those functions the sampled tests ran.
- **SP-2110~3**: WHEN a scan draws mutants THE SYSTEM SHALL keep only the operator
  and sentinel candidates whose mutated lines include an admitted line, before
  screening and selection; draw at most the requested number of operator
  mutants, by default 150, by the allocation of SP-2125, and keep the sentinels
  by the rule of SP-2126; give each mutant the digest of its edits as its id and
  the lines of its mutated span; run the sentinels against the same tests as
  the mutants, never proving one; and order the mutants to start the sentinels
  first, then the operator mutants in the order the allocation drew them, the
  order SP-2139 starts them from.
- **SP-2111~3**: WHEN a scan runs a mutant of its draw THE SYSTEM SHALL run it, on at most
  the requested number of workers, by default those of SP-2130, against every
  green test whose recorded coverage holds one of its mutated lines, three times, each run bounded by
  three times the duration of one reference run of the whole suite and at least
  30 seconds; run no test of a mutant other than a sentinel whose every edited
  site compiles to the reference's form, a sentinel always running; drop and
  count a mutant that does not splice or build; and
  start no mutant of the draw once the budget has passed since the scan began, and stop a
  mutant still running then, counting it as not run, marking the scan partial
  with the counts reached.
- **SP-2112~1**: WHEN no test killed a mutant that is not a sentinel THE SYSTEM
  SHALL search for a difference without a spec: call each function the mutant
  edits that the ecosystem binds and whose every parameter has a known type, on
  inputs drawn by the seed from the literals of the drawn code and of every test
  file, twice on the reference and twice on the mutant, and report the smallest
  input on which each side agreed with itself and the two differed, with the
  operation, the input, the two results and the channel, an exception when
  either side raised and the return value otherwise; a function the probe cannot
  call yields no difference and no error.
- **SP-2113~1**: WHEN a scan is recorded THE SYSTEM SHALL classify every test with
  the rules of the scan verdicts, write a scan statement of the head commit with
  the file-set digest of its tracked files, scan reports aside, holding the
  module, the producer, the mode, the base, the runner, the seed, the partial
  mark, the counts, every mutant that ran with its edits, lines, origin, family
  and routes, and every sampled green test, encode it canonically, and give one
  finding per test that verifies nothing, in rank order, with the first of its
  changes a difference proves, else its first change, the route, the files the
  change edits before and after, and the command line that runs the test.
- **SP-2114~2**: WHEN a finding is replayed THE SYSTEM SHALL refuse a work tree
  that is dirty or at another commit, and re-execute with no model, on a copy of
  the work tree, that the test passes the reference and the mutant three runs out
  of three, that the mutant builds, that a killing test passes the reference and
  fails the mutant three runs out of three, the test and the killing test each
  in a selection of its own, and that the input of a difference
  gives each recorded result twice on its side; and SHALL report every claim
  that does not hold.
- **SP-2115~4**: WHEN a scan is printed for a terminal THE SYSTEM SHALL print the
  repository, the short commit, the runner, the tests and the green ones, the
  sampled ones and the seed; on the next line the depth, the budget or "no
  budget", the time, how many of the green tests were sampled and the workers,
  and, for a quick scan, that `--full` judges the whole suite; the counts of verify nothing, caught at least one
  change and unproven, each over the sampled green tests, and not sampled over
  the green tests; the unproven tests by reason; what their lines lack, as in
  SP-2129; the line that this is not a pass; for each other judged test that k
  proven changes, k at least one, stopped from starting, in test id order, its id
  and "k proven changes in lines it ran stopped it from starting; they count
  neither way", in the singular for one; for each test that verifies nothing its
  rank, its id and "passes on all N proven changes in lines it ran", followed,
  when k proven changes stopped it from starting, by "; k more stopped it from
  starting", the site and the change, with the input and both results of a
  difference, what proves it and its repro script; a note when the scan is
  partial; and the mutants that ran and were proven, with those identical when
  compiled, those that did not build or start, the sentinels that stopped a
  test binary and the mutants not run within the budget, the time with the
  time of each phase, "no model used" and "promises not checked"; and SHALL
  never print "bug", "PASS" or a score.
- **SP-2116~3**: WHEN a scan is written as Markdown THE SYSTEM SHALL write the
  same header and depth line, the four counts as a table, the unproven reasons,
  what their lines lack with one row per unproven test, the line that this is
  not a pass, one item per other judged test that proven changes stopped from
  starting with the line of SP-2115, each finding with its sentence, change,
  proof and repro script and its patch as a diff, and the closing line, never
  "bug", "PASS" or a score.
- **SP-2117~1**: WHEN the reports of a scan are written THE SYSTEM SHALL write,
  under `.spec-proof/scan/<short commit>/` and in place of the earlier reports of
  that commit, `report.md`, the canonical statement as `scan.json` and, per
  finding, `mutant.patch`, a unified diff with one hunk that `git apply` applies
  to the commit, an executable `repro.sh` that extracts the commit into a
  temporary directory, links the project's dependency directories, applies the
  patch and runs the one test, quoted, so that its exit status is the test's,
  and `finding.json`, which names the commit, the runner, the test, the mutant
  with its edits and the route.
- **SP-2119~3**: WHEN `spec-proof-dp scan` runs THE SYSTEM SHALL need no
  configuration, scan the first pytest, Go, Jest or Vitest suite it detects,
  quickly unless
  `--full` is given (SP-2131), `--sample`, `--mutants` and `--budget` replacing
  what the depth presets, write the reports,
  print the headline, the Markdown report with `--format md` or the scan statement
  with `--format json`, and exit 0 whatever the counts; SHALL fail, writing no
  report, on a tree with no such suite, a dirty tree, an unknown format or an
  extra argument; and `spec-proof-dp scan replay <dir>` SHALL replay the finding
  of a finding folder or every finding of a scan folder and exit 1 when a claim
  does not hold.
- **SP-2120~1**: WHEN the git state of a scanned project is read THE SYSTEM SHALL
  resolve the head and a base revision to full commit ids, list the tracked
  files, report the tree dirty only for a changed tracked file outside the scan
  reports, give the lines of the head that differ from a base, a pure deletion
  marking the line it follows, and name the repository `owner/name` from its
  origin, in URL or scp-like form, or by its directory.
- **SP-2122~3**: WHEN `spec-proof-dp scan --budget 0` runs on a pytest, a Go, a
  Jest or a Vitest project committed in its own repository, one of whose tests
  runs a function
  and asserts nothing THE SYSTEM SHALL name that test as the first that verifies
  nothing and exit 0, leave the work tree with only its reports added, and give a
  repro script that passes the test on the changed code; and `spec-proof-dp
  scan` with no flag SHALL run quickly, print its depth line and exit 0.
- **SP-2123~2**: WHEN a scan of the same project that no budget stopped is
  recorded THE SYSTEM SHALL produce the same canonical statement whatever the
  number of workers, accepted by the scan schema and refused by every reader of
  proofs.
- **SP-2125~2**: WHEN a scan allocates its operator mutants THE SYSTEM SHALL draw
  them in rounds over the sampled tests, each round giving a turn, in an order
  shuffled by the seed, to every test that has a mutant left among the admitted
  lines it owns and no more drawn mutants among them than the fewest such a test
  had when the round began, first among the tests with fewer than six, then
  among all; on its turn a test SHALL draw, among the mutants of its lines not
  drawn yet, the one that gives it a second site while it has fewer than two,
  then the one run by the most tests with fewer than six, then one on a site it
  has not, then one of a family it has not, ties broken by an order shuffled by
  the seed, a site being the first mutated line of a mutant; a drawn mutant SHALL
  count for every sampled test that owns one of its lines; and THE SYSTEM SHALL
  draw no mutant that no sampled test owns a line of, give every sampled test at least the
  fewer of the mutants of its lines and the budget divided by the number of
  sampled tests with a mutant, draw every mutant on a line a sampled test owns when the
  budget allows, and make the same draw for the same seed whatever the order of
  the mutants.
- **SP-2126~1**: WHEN a scan keeps its sentinels THE SYSTEM SHALL keep at most
  twelve, stratified by file: the files and the sentinels of each file shuffled
  by the seed, one sentinel of each file in turn, so that no file gives a second
  sentinel before every file gave one; every sentinel when there are fewer; and
  the same sentinels, sorted by id, for the same seed whatever their order.
- **SP-2127~1**: WHEN a scan records the line coverage of the reference THE SYSTEM
  SHALL bound each run of tests under coverage, the whole pytest suite or one Go
  test, by the time `--coverage-timeout` gives, or else by ten times the
  duration of one reference run of the whole suite, at least 2 minutes and at
  most 30 minutes; and SHALL fail on a run past its bound with an error that
  names the bound and `--coverage-timeout`.
- **SP-2129~3**: WHEN a scan is recorded THE SYSTEM SHALL explain each unproven
  test by its changes, the drawn operator mutants on the lines it ran and, for
  each of them no route proves, why it proves nothing: not run within the
  budget, did not build or start, identical when compiled, killed only by a
  test that fails a sentinel, or, when
  no test kills it, no differential search in the ecosystem, no function the
  search can bind, a bound function it cannot call, or a search that found no
  difference; SHALL count the sentinels on which a run of a test did not
  start; and SHALL time each phase of the scan: the reference, the coverage, the
  draw, the mutants and the confirmation of SP-2144.
- **SP-2130~1**: WHEN a scan is given no number of workers THE SYSTEM SHALL read
  the processors of the machine and the share of its memory that is free, with
  `sysctl` and `memory_pressure` on macOS and /proc/meminfo elsewhere, and run
  one worker per two processors and per GiB of memory above the 40 % the memory
  guard keeps free, the fewer of the two, at least one and at most four; and
  the default workers when the capacity cannot be read.
- **SP-2131~3**: WHEN a scan runs quickly THE SYSTEM SHALL sample 12 green tests,
  draw 6 operator mutants for each, record the line coverage of the sampled
  tests only, so that only they run a mutant or a sentinel, unless the runner
  records every test in one run of the suite, as pytest does, when it records
  every green test; and run no mutant of the draw once 90 seconds have passed since the
  scan began, confirming its claims as SP-2144 gives once they stop; WHEN a scan runs in full it
  SHALL sample 40 tests, draw 150 mutants, record the coverage of every green
  test and have no budget; and a Go suite SHALL build the coverage binary of
  each package and run its tests under coverage on the scan's workers.
- **SP-2132~1**: WHEN a scan in which K of the N sampled green tests verify
  nothing, K at least one, is printed for a terminal or written as Markdown THE
  SYSTEM SHALL end it, after the line that counts the mutants, with "K/N sampled
  tests stay green on every proven change in lines they ran: on those lines,
  their green is no evidence the code is right."; and SHALL end a scan in which
  no sampled test verifies nothing on the line that counts the mutants.
- **SP-2128~1**: WHEN a command names a file for its standard output THE SYSTEM
  SHALL write the whole output there, however long, and leave it out of the
  captured output; and a scan of a Go suite SHALL read every test event of a run
  from such a file, so that no test goes unreported however much the tests log.
- **SP-2134~1**: WHEN the `go test -count` process of a Go package exits
  non-zero without timing out before a test of the package had an end event for
  every run THE SYSTEM SHALL count that process as one observation of each test
  of the package, its first end event, and run the package again in fresh
  processes of one run each, each bounded by the timeout of one run, until every
  test has as many observations as runs, starting no process after one timed
  out; and SHALL give a test of a process that is one observation its first end
  event, timed out when the process timed out, not started only when the process
  exited non-zero without timing out, the package's binary ended in a failure,
  no event of the package names a test and no event reports a build failure,
  and errored otherwise.
- **SP-2135~1**: WHEN a pytest session does not report a test THE SYSTEM SHALL
  give it a run that did not start only when a failed collect report of the
  session, or a `conftest.py` that failed to import before the session began,
  covers its module, a `conftest.py` or `__init__.py` in a directory on its path,
  or such a directory, and errored otherwise, a session that ended in a usage,
  internal or interruption error without such a report included; and SHALL give
  a test whose fixture or setup failed errored.
- **SP-2136~2**: WHEN `spec-proof-dp scan --budget 0` runs on a Go project, one
  of whose packages calls a helper from a test-file variable and from its test
  bodies, another of which calls it from its test bodies, and a third only at
  start-up, or on a pytest, a Jest or a Vitest project, one of whose test
  modules calls a helper on import and in its test bodies, THE SYSTEM SHALL
  list, for each test whose binary or module a change of the helper stops from
  starting, that change
  apart and count it neither for nor against the test; settle the change as
  usual for a Go test whose binary starts; record no survival on the helper for
  the Go test that runs it only at start-up; print how many changes stopped each
  such test and keep the sentence of the test that verifies nothing; write a
  statement the format accepts; and replay its finding with every claim held.
- **SP-2137~1**: WHEN the compiled forms of sites that all lie in one Go package
  are computed THE SYSTEM SHALL compile the package's files in the workspace
  against the listing and the import configuration of its dependencies read
  the first time that package was compiled so for the same comparison, and
  SHALL read both afresh for sites that span several packages, a dependency
  among them possibly changed, so that each form is the one a fresh compile of
  the workspace gives.
- **SP-2138~1**: WHEN a scan follows the outcomes of its finished mutants THE
  SYSTEM SHALL hold a sampled green test settled only when it failed a run on a
  sentinel, or when every sentinel has finished, it failed a run on none of
  them, and it killed, every run failing, a finished mutant that is no sentinel
  and not identical when compiled, with a mutated line among the lines it ran;
  SHALL hold every other sampled green test open, whatever it survived; and
  SHALL give a settled test the class and reason its classification gives
  whatever the outcomes of the mutants that have not finished, and the same
  settled tests whatever the order the mutants finished in.
- **SP-2139~1**: WHEN a scan starts its next mutant THE SYSTEM SHALL start, among
  the mutants not started yet, the first in the order of SP-2110 that is a
  sentinel or has a mutated line among the lines, start-up included, that an
  open sampled test of SP-2138 ran, and only when no such mutant is left the
  first other one; SHALL read the progress of SP-2138 with every finished
  mutant recorded, in one place that no running mutant reads; and SHALL leave
  out no mutant, so that a scan that runs every mutant runs the same ones
  whatever the order they finish in.
- **SP-2140~1**: WHEN `spec-proof-dp scan --budget 0` runs on a Go project of
  several packages, once on one worker and once on four, THE SYSTEM SHALL write
  the same scan statement; and WHEN `spec-proof-dp scan` runs with no flag on a
  Go project, one of whose tests runs a function and asserts nothing, THE
  SYSTEM SHALL name that test as the first that verifies nothing.
- **SP-2141~1**: WHEN a Go process of a scan gives a leaf test a pass THE
  SYSTEM SHALL read it against the end events its ancestors gave in the same
  iteration of `-count`: keep the pass when every ancestor passed; give the run
  errored, or timed out when the process timed out, when an ancestor failed or
  gave no end event in that iteration while no other test below it, neither the
  leaf nor one of the leaf's ancestors, failed in that iteration, never a
  failure; and otherwise, unless a process of the package timed out, run the
  leaves of the package that had such a pass and failed no run again, together
  and without the others, in one fresh `go test -count` process of as many runs,
  bounded by the timeout of one run times the number of runs, which gives them
  every run under the same reading, a pass whose ancestors fail there again
  being errored, and which, when it exits non-zero without timing out before a
  leaf had an end event for every run, gives each leaf its first end event and
  errored runs.
- **SP-2142~1**: WHEN a pytest session of selected tests stops before
  collection because a `conftest.py` it loads first failed to import THE SYSTEM
  SHALL run, within the same run, a session of the selected tests that no
  failure of that run covers yet, and again while such a session stops so,
  until a session runs, no test is left or a session covers no more of them; and
  SHALL give each test of the run one result: not started when a failed collect
  report of one of its sessions covers it as SP-2135 gives, the outcome of the
  session that ran it otherwise, timed out when its last session timed out, and
  errored for a test absent from a module that collected.
- **SP-2143~3**: WHEN `spec-proof-dp scan --budget 0` runs on a Go project whose
  only survivals of a test lie in the setup its parent test runs before its
  subtests or in the header of a clause that a `fallthrough` enters, or on a
  pytest project whose only survivals of a test lie in the teardown of a
  fixture, or on a Jest or a Vitest project whose only survivals of a test lie
  in an `afterEach` hook it shares, THE SYSTEM SHALL make no claim that the
  test verifies nothing; WHEN it runs on a pytest project in which a change
  stops a test
  module and a `conftest.py` from importing, THE SYSTEM SHALL settle the tests
  outside them as usual and list that change apart for the tests it stops; and
  SHALL write a statement the format accepts and replay its findings with
  every claim held.
- **SP-2144~1**: WHEN a scan has classified a test as verifying nothing THE
  SYSTEM SHALL, before it reports the test, confirm the claim on the changes
  the test cites: keep a change only when the test owns one of its mutated
  lines both in the recording of the scan and in a recording of the test in a
  run that selects it alone, recording it so where the runner records every
  test in one run of the suite and reading the scan's recording where it
  records each test alone; keep a change that only kills prove only when one
  of its killers, run alone, passes the reference and fails the mutant three
  runs out of three; run the test alone three times on the mutant of each
  change left and record a change it does not pass on all three as
  inconclusive; classify the test again on those changes, so that the count it
  prints is the confirmed one, and hold it unproven when its recording alone
  fails, each of its changes inconclusive; run every test of the confirmation
  in a selection of its own, after the mutants of the draw, whether or not the
  budget has passed, bounded by the changes the tests cite; change no other
  verdict, no route and no count but the test's own; show in a finding a kill
  only by a killer the confirmation upheld; and write the scan statement in
  the same form.
- **SP-2145~2**: WHEN `spec-proof-dp scan --budget 0` runs on a pytest project
  one of whose tests owns lines only once an earlier test of the session set
  the state it reads, on a Go project one of whose tests passes the changes it
  runs only beside the test that seeds the state it reads, or on a Jest or a
  Vitest project one of whose tests runs a function only once an earlier test
  of its file set the state it reads, THE SYSTEM SHALL
  make no claim that the test verifies nothing; SHALL keep the claim of a test
  that verifies nothing alone and print the time of the confirmation; and
  SHALL write a statement the format accepts and replay its findings with
  every claim held.
- **SP-2146~1**: WHEN mutants are generated for a file in which another unit of
  the same kind shares the qualified name of a definition or module assignment
  THE SYSTEM SHALL draw no operator mutant and no sentinel on that unit, for
  every engine, count the units left out of the run, report that count in the
  footer of a scan, and still fail the run on any other mutant that does not
  splice.

## Scan of Jest and Vitest suites

- **SP-2292~1**: WHEN a scan runs a Jest or a Vitest suite THE SYSTEM SHALL run
  each run as one process of the runner, Jest in band under the shipped test
  environment and reporter, Vitest in one fork under the shipped runner and
  configuration, coverage, bail and retries off and test files in path order;
  name each test `file::titles`, its describe titles and its own joined by
  ` > `; keep the tests of a selection alone, skipping every other test and
  every describe block left with none; give a test whose test function fails
  a failure, and give errored to a test whose hook fails, to a test that was
  retried, to a test whose id another test shares, or for Jest equals another
  ignoring case, and, in a run of a selection, to a test only the reporter saw;
  give a test not started only when the run's global setup failed or the
  test's file stopped before any of its tests began; and give the tests of a
  process that timed out, and of every later run, timed out.
- **SP-2293~1**: WHEN a scan records the coverage of a Jest or a Vitest test it
  reads the owned lines of THE SYSTEM SHALL record the test alone in three
  processes, each with V8 precise coverage through an inspector session of
  its own; open the test's window before its first `beforeEach` hook and close
  it when its test function settles; cut out of the window, as slices of
  foreign origin, each callback whose async resource was created by setup
  files, module evaluation, describe bodies or the `beforeAll`, `afterAll` and
  `afterEach` hooks, and each callback of a fake timer pending when the window
  opened or scheduled by such a slice; map each code unit of the generated
  code past the runner's wrapper to a line of the project through the source
  map that code carries, a unit that is whitespace or a brace holding no code;
  own a line only when, in each of the three processes, every code unit of it
  that ran counted in the window and in no slice of foreign origin, with the
  same window counts in all three; and own nothing for a recording whose
  window evaluated a module, a module's own function counting or a module
  first counting there, whose window did not hold the start and the settling
  of the test function, or in which the test did not pass.
- **SP-2294~1**: WHEN a process of a Jest or a Vitest recording lacks a
  precondition of SP-2293 THE SYSTEM SHALL give its fallback: without an
  inspector session, record the test again through `NODE_V8_COVERAGE` and give
  it every line of each project file one of whose functions counted, owning
  nothing; without synchronous takes, open the window at the test function, so
  that `beforeEach` is not owned, and read the window as without async hooks
  that see the realm's promises, against three hollow runs of the test whose
  test function does nothing, owning a line only when it ran more often in the
  window than in the hollow runs, which agree with each other, and, when they
  do not, owning nothing for a window in which a callback of foreign origin
  ran; without a take that shows a module evaluated in the window, or with
  function counts that may vanish between takes, own nothing; without
  enumerable pending fake timers, own nothing for a test whose realm fakes
  timers when its window opens; without the generated code and source map of a
  module, map only a module whose code is its file byte for byte; and without
  windows that bound a concurrent test, own nothing for a concurrent test.
- **SP-2295~1**: WHEN a scan records the coverage of Jest or Vitest tests whose
  owned lines it does not read THE SYSTEM SHALL run them in one process of
  their files in path order and give each the lines its file's segment of that
  process ran, from the file's start to the next file's, the process's
  start-up included, owning none; SHALL report no line of a test file; and
  SHALL read the counts of a process alike whether its takes reset the counts
  or count from the start of the process.
- **SP-2296~1**: WHEN a scan records the units the witnesses of a Jest or a
  Vitest project run THE SYSTEM SHALL run each witness alone under the
  coverage of SP-2293 and name the innermost definition of the project holding
  each line it ran inside its window, a test file's aside.
- **SP-2297~1**: WHEN a scan binds a definition of a Jest or a Vitest project
  to the differential search THE SYSTEM SHALL bind a top-level function of a
  source module to `path/file.ts:function` and no method, nested function,
  declaration file or test file; SHALL write a repro that runs the one test
  with `npx --no-install`, by its file and an anchored pattern over its full
  title that accepts ` > ` or a space between titles; and SHALL refuse a
  Vitest project that runs its tests in a browser, naming the reason.
- **SP-2298~1**: WHEN a repository scan records the coverage of its tests THE
  SYSTEM SHALL name the sampled tests as the owners whose owned lines it
  reads, and a change scan, which samples once the coverage is recorded, SHALL
  name every test it records.
- **SP-2299~1**: WHEN a scan runs the jobs of a recording on its workers THE
  SYSTEM SHALL run each job once on at most the given number of workers, at
  least one, start no job once one has failed, and return that failure once
  every started job has ended.
