# spec-proof requirements

The requirements of the spec-proof product, in Shallnot's Markdown spec format.
`shallnot gate` traces them to the Go tests of this repository in CI.

## Statement envelope

- **SP-1~1**: WHEN a statement is decoded THE SYSTEM SHALL read the in-toto
  Statement v1 envelope of the three predicate types of the format, report a
  `statement_type` or `predicate_type` violation for a wrong type URI, and
  refuse a predicate type outside the format.
- **SP-2~1**: WHEN a statement carries a member the format does not define THE
  SYSTEM SHALL ignore it, so that minor versions stay additive.
- **SP-3~1**: WHEN a claim is built THE SYSTEM SHALL give a judged claim a
  confidence in [0,1] and an executed claim none.
- **SP-4~1**: WHEN a decoded claim is judged without a confidence, executed
  with a confidence, or carries a confidence outside [0,1] THE SYSTEM SHALL
  report `judged_without_confidence`, `executed_with_confidence` or
  `confidence_range`.
- **SP-5~1**: WHEN a claim has a class other than the one its predicate field
  allows THE SYSTEM SHALL report `wrong_class`.
- **SP-6~1**: WHEN a value is serialized THE SYSTEM SHALL produce RFC 8785
  canonical bytes, and canonicalizing canonical bytes SHALL leave them
  unchanged.
- **SP-7~1**: WHEN the members of a JSON object are written in another order
  THE SYSTEM SHALL produce the same canonical bytes.

## Digests and keys

- **SP-8~1**: WHEN a file set is digested THE SYSTEM SHALL hash `path NUL
  content NUL` over the files sorted by path, regardless of the order given,
  and refuse a path that is not relative POSIX or that occurs twice.
- **SP-9~1**: WHEN a cache key is computed THE SYSTEM SHALL hash the canonical
  form of the subject and the inputs, regardless of the order of the inputs,
  and give another key when any input changes.
- **SP-10~1**: WHEN the statement is an alignment or a discriminating-power
  statement THE SYSTEM SHALL require exactly one subject named `ID~REV` after
  the requirement of the predicate, with the statement digest.
- **SP-11~1**: WHEN the statement is a completeness statement THE SYSTEM SHALL
  require exactly one subject named by a relative POSIX spec path, with the
  hash of the file.
- **SP-12~1**: WHEN an array of a predicate is not strictly ascending in
  byte-wise order of its sort key THE SYSTEM SHALL report `unsorted`.
- **SP-13~1**: WHEN an input role, a verdict, a validity, an outcome, a
  producer or a decision basis is outside its closed set, or a digest is not
  a SHA-256 hex string, THE SYSTEM SHALL report `unknown_value` or
  `bad_digest`.
- **SP-14~1**: WHEN the recorded `cacheKey` differs from the digest of the
  subject and the inputs THE SYSTEM SHALL report `cache_key_mismatch`.
- **SP-21~1**: WHEN a `junit_result` digest is computed THE SYSTEM SHALL hash
  the canonical map from test identifier to outcome and never the raw XML.

## Predicates

- **SP-15~1**: WHEN an alignment predicate is checked THE SYSTEM SHALL require
  a judged verdict and a blind judge.
- **SP-16~1**: WHEN a discriminating-power predicate is checked THE SYSTEM
  SHALL require an executed `correctCodePasses`, a judged validity per mutant
  and an executed outcome per test result.
- **SP-17~1**: WHEN a mutant is a sentinel THE SYSTEM SHALL require its
  validity to be EQUIVALENT.
- **SP-18~1**: WHEN a discriminating-power predicate lists example results THE
  SYSTEM SHALL require each to be an executed `passed` or `failed`.
- **SP-19~1**: WHEN a completeness predicate is checked THE SYSTEM SHALL
  require `advisory` to be true and SHALL refuse a verdict field in its
  schema.
- **SP-20~1**: WHEN a predicate carries `extensions.decision` or
  `extensions.operatorScreen` THE SYSTEM SHALL keep them through decoding and
  encoding and check the decision's basis and confidences.

## Schemas and golden vectors

- **SP-22~1**: WHEN the embedded schemas are loaded THE SYSTEM SHALL compile
  each JSON Schema 2020-12 document from its embedded bytes, with the
  predicate type URI as its `$id`.
- **SP-23~1**: WHEN a statement is validated against its schema THE SYSTEM
  SHALL require a confidence for a judged claim and forbid one for an executed
  claim.
- **SP-24~1**: WHEN a golden vector is decoded and validated THE SYSTEM SHALL
  give the outcome and the violation codes its manifest states.
- **SP-25~1**: WHEN a golden vector is validated against the schemas THE
  SYSTEM SHALL give the schema result its manifest states.
- **SP-26~1**: WHEN a generated predicate is checked THE SYSTEM SHALL accept it
  against the schemas exactly when the schema-expressible domain invariants
  hold.
- **SP-27~1**: WHEN a schema references a resource that is not embedded THE
  SYSTEM SHALL fail to compile it rather than fetch it.
- **SP-28~1**: WHEN a document is not JSON, or its predicate type is outside
  the format, THE SYSTEM SHALL refuse it before schema validation.
- **SP-29~1**: WHEN a spec path is judged by the subject rule THE SYSTEM SHALL
  agree between the Go check and the schema pattern.

## Domain

- **SP-30~1**: WHEN a mutant run is classified THE SYSTEM SHALL answer
  `killed` when a bound test failed, `survived` when every bound test passed,
  and `error` when the build check failed, collection failed, the run timed
  out, or a bound test is absent or skipped.
- **SP-31~1**: WHEN a span is built THE SYSTEM SHALL refuse negative or
  inverted bounds and slice only inside the source.

- **SP-35~1**: WHEN project settings are created THE SYSTEM SHALL disable
  every proof, and refuse an unknown proof or ecosystem.

## Architecture

- **SP-36~1**: WHEN the packages `format` and `internal/domain` are inspected
  THE SYSTEM SHALL import no adapter, `os`, `io/fs`, `os/exec` or `net`.
- **SP-37~1**: WHEN the package `internal/app` is inspected THE SYSTEM SHALL
  import no adapter.
- **SP-38~1**: WHEN an adapter is inspected THE SYSTEM SHALL import no other
  adapter.
- **SP-39~1**: WHEN the `spec-proof-verify` binary is linked THE SYSTEM SHALL
  contain neither `os/exec` nor an adapter of the model, ecosystem, process,
  git or workspace families.
- **SP-40~1**: WHEN either binary is run with the `version` argument THE
  SYSTEM SHALL print the product version.

## Adapters

- **SP-41~1**: WHEN a Python, Java, Kotlin, TypeScript, TSX or JavaScript
  source is located THE SYSTEM SHALL return every function and method
  definition with its byte span.
- **SP-42~1**: WHEN a definition is nested in a class or a function THE SYSTEM
  SHALL name it by its qualified path, such as `Class.method`.
- **SP-43~1**: WHEN a program links one grammar through the build-tag subset
  THE SYSTEM SHALL stay below the recorded size budget and parse that
  language, and a program linking the full registry SHALL be larger.
- **SP-44~1**: WHEN a file has an extension no grammar covers THE SYSTEM SHALL
  refuse it.
- **SP-45~1**: WHEN a project configuration is decoded from YAML THE SYSTEM
  SHALL read the proofs and the ecosystems, and refuse an unknown proof.
- **SP-46~1**: WHEN glob patterns are expanded over a tree THE SYSTEM SHALL
  collect the matching files so that their file-set digest equals the digest of
  the same files given directly.
- **SP-47~1**: WHEN mutant bytes are encrypted to an age recipient THE SYSTEM
  SHALL let the holder of the identity recover them and nobody else.
- **SP-48~1**: WHEN a statement is wrapped for signing THE SYSTEM SHALL carry
  the exact canonical bytes as the payload of a DSSE envelope inside a bundle.

