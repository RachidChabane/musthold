# ADR 0008: Input descriptors and staleness

Status: accepted. Refines ADR 0001 ("Common predicate core", `inputs[]`), ADR 0002
("Shallnot changes", the file-set digest rule) and ADR 0003 ("Cost controls and
cache keys").

## Context

A proof is valid only while the inputs it was computed from are unchanged, and
Shallnot recomputes the input digests it can derive from the tree before it
admits a proof. ADR 0002 specifies one file-set digest over the configured
`proofs.code`, `tests` and `specs` globs. ADR 0003 keys the expensive work of
discriminating power to a criterion's footprint, one digest per footprint file,
so that a change outside a criterion's footprint costs that criterion nothing.
The two rules disagree: a whole-tree set digest makes every proof stale on any
change anywhere, which turns "only criteria whose inputs changed are re-judged
or re-mutated" (ADR 0001) into "every criterion, on every change", while the
per-file form gives Shallnot nothing it currently recomputes.

## Decisions

### One descriptor per file

`code_tree` and `test_source` inputs are per-file resource descriptors:
`{role, name: <path relative to the repository root, POSIX>, digest: {sha256:
<SHA-256 of the file's exact bytes>}}`, sorted by role, then name.

- `code_tree` lists the criterion's footprint files (ADR 0003: functions reached
  by the bound tests and examples, the diff, and their static closure) plus the
  module-level files their imports reach.
- `test_source` lists every test file that holds a test bound to the
  criterion, plus every test file whose tests execute footprint code under the
  reference coverage.
- `spec` is the criterion's statement digest under the canonicalization
  recorded in the predicate, named `<ID>~<REV>`; it duplicates the subject on
  purpose, so that every input role follows one rule.
- `proof` is named by the consumed kind (`discriminating-power`), carries the
  criterion `ID~REV` it is about in `annotations.criterion`, and its digest is the
  SHA-256 of the consumed Statement file's exact bytes, the digest the verified
  set's manifest lists. A proof consumes at most one Statement of a kind; the
  inputs are strictly ascending by role then name. The cache key covers the role,
  name, digest and globs of an input and not the criterion annotation, which a
  reader that does not know it ignores.
- Every other role (`junit_result`, `mutant_set`, `prompt_template`,
  `decision_policy`, `example_binding`, `host_response`) keeps the form ADR 0001,
  0003 and 0005 give it.

The file-set digest and the `annotations.globs` comparison of ADR 0002 are
not used for proof inputs.

### What Shallnot checks

A proof is stale when any of these holds:

1. a `code_tree` or `test_source` descriptor names a file that is absent or
   whose bytes no longer have the recorded digest;
2. a `code_tree` descriptor names a file outside the configured `proofs.code`
   globs, or a `test_source` descriptor a file outside the configured `tests`
   roots;
3. for a discriminating-power proof, a test file that holds a test Shallnot
   binds to the criterion is not listed among the `test_source` descriptors;
   for an alignment proof, which concerns one test, the file holding that
   test is not listed;
4. the `spec` descriptor differs from Shallnot's own statement digest for the
   criterion;
5. a `proof` input names a Statement that is not in the verified set
   (ADR 0005).

Checks 1 to 4 read files only; Shallnot stays without network and model.

### Who decides the footprint

The module computes the footprint; Shallnot does not, because it parses no
source language. A footprint that omits a file the criterion depends on would
let a proof survive a change it should not: locally, that is the advisory
fast-feedback path ADR 0001 already scopes; in CI, the module recomputes the
footprint from the commit, so an omission in a developer's local proof never
reaches a signed proof. Check 3 closes the one omission Shallnot can see
without parsing source: a new or moved bound test.

## Consequences

- A change outside a criterion's footprint leaves its proofs valid, so local
  `prove --changed` re-runs only the criteria it touches, and CI re-runs only
  those criteria too.
- Footprint precision is the module's responsibility and its main staleness
  risk: dynamic dispatch, reflection, data files or plugins reached at run time
  are invisible to a static closure. The benchmark measures it by editing code
  outside each footprint and checking that the criterion's oracle verdict is
  unchanged (limit 9 and limit 1 attribution).
- The format library defines the descriptor form once; Shallnot's
  `proofset` adapter and every module's proof writer are tested against the
  same golden Statements.
