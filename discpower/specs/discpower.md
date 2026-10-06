# discpower requirements

The requirements of the `discpower` module, which measures the discriminating
power of tests by executing them against mutants, in Shallnot's Markdown spec
format. `shallnot gate` traces them to the Go tests of the module in CI.

## Schemas

- **SP-110~1**: WHEN the embedded schemas are loaded THE SYSTEM SHALL compile
  each from its embedded bytes.
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
