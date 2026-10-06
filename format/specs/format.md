# format requirements

The requirements of the `format` module, the proof format shared by every
module, in Shallnot's Markdown spec format. `shallnot gate` traces them to the
Go tests of the module in CI.

## Statement envelope

- **SP-1~1**: WHEN a statement is decoded THE SYSTEM SHALL read the in-toto
  Statement v1 envelope of the three predicate types of the format, report a
  `statement_type` or `predicate_type` violation for a wrong type URI, and
  refuse a predicate type outside the format.
- **SP-2~1**: WHEN a statement carries a member the format does not define THE
  SYSTEM SHALL ignore it, so that minor versions stay additive.
- **SP-3~1**: WHEN a claim is built THE SYSTEM SHALL give a judged claim a
  confidence in [0,1] and an executed claim none.
- **SP-4~1**: WHEN a decoded claim is judged without a confidence, executed with
  a confidence, or carries a confidence outside [0,1] THE SYSTEM SHALL report
  `judged_without_confidence`, `executed_with_confidence` or `confidence_range`.
- **SP-5~1**: WHEN a claim has a class other than the one its predicate field
  allows THE SYSTEM SHALL report `wrong_class`.
- **SP-6~1**: WHEN a value is serialized THE SYSTEM SHALL produce RFC 8785
  canonical bytes, and canonicalizing canonical bytes SHALL leave them
  unchanged.
- **SP-7~1**: WHEN the members of a JSON object are written in another order THE
  SYSTEM SHALL produce the same canonical bytes.

## Digests and keys

- **SP-8~1**: WHEN a file set is digested THE SYSTEM SHALL hash `path NUL
  content NUL` over the files sorted by path, regardless of the order given, and
  refuse a path that is not relative POSIX or that occurs twice.
- **SP-9~1**: WHEN a cache key is computed THE SYSTEM SHALL hash the canonical
  form of the subject and the inputs, regardless of the order of the inputs, and
  give another key when any input changes.
- **SP-10~1**: WHEN the statement is an alignment or a discriminating-power
  statement THE SYSTEM SHALL require exactly one subject named `ID~REV` after
  the requirement of the predicate, with the statement digest.
- **SP-11~1**: WHEN the statement is a completeness statement THE SYSTEM SHALL
  require exactly one subject named by a relative POSIX spec path, with the hash
  of the file.
- **SP-12~1**: WHEN an array of a predicate is not strictly ascending in
  byte-wise order of its sort key THE SYSTEM SHALL report `unsorted`.
- **SP-13~1**: WHEN an input role, a verdict, a validity, an outcome, a producer
  or a decision basis is outside its closed set, or a digest is not a SHA-256
  hex string, THE SYSTEM SHALL report `unknown_value` or `bad_digest`.
- **SP-14~1**: WHEN the recorded `cacheKey` differs from the digest of the
  subject and the inputs THE SYSTEM SHALL report `cache_key_mismatch`.
- **SP-21~1**: WHEN a `junit_result` digest is computed THE SYSTEM SHALL hash
  the canonical map from test identifier to outcome and never the raw XML.

## Predicates

- **SP-15~1**: WHEN an alignment predicate is checked THE SYSTEM SHALL require a
  judged verdict and a blind judge.
- **SP-16~1**: WHEN a discriminating-power predicate is checked THE SYSTEM SHALL
  require an executed `correctCodePasses`, a judged validity per mutant and an
  executed outcome per test result.
- **SP-17~1**: WHEN a mutant is a sentinel THE SYSTEM SHALL require its validity
  to be EQUIVALENT.
- **SP-18~1**: WHEN a discriminating-power predicate lists example results THE
  SYSTEM SHALL require each to be an executed `passed` or `failed`.
- **SP-19~1**: WHEN a completeness predicate is checked THE SYSTEM SHALL require
  `advisory` to be true and SHALL refuse a verdict field in its schema.
- **SP-20~1**: WHEN a predicate carries `extensions.decision` or
  `extensions.operatorScreen` THE SYSTEM SHALL keep them through decoding and
  encoding and check the decision's basis and confidences.

## Executors and corroboration

- **SP-111~1**: WHEN a predicate or a judged claim carries an executor THE
  SYSTEM SHALL require a channel, an isolation level and a model source of their
  closed sets, a named harness, a named model unless its source is `unknown`,
  and a SHA-256 prompt digest; and, for a `restricted-attested` executor, an
  agent identifier, a cap digest and a transcript digest; and SHALL report
  `unknown_value`, `missing_field` or `bad_digest`, and `executed_with_executor`
  for an executed claim that carries one.
- **SP-112~1**: WHEN a judged mutant validity is VALID THE SYSTEM SHALL require
  its `corroboratedBy` list, whose entries are of a closed set of kinds, carry a
  SHA-256 evidence digest and are strictly ascending, and SHALL report
  `uncorroborated_claim` for an empty list and `executed_with_corroboration` for
  an executed claim that carries one.
- **SP-113~1**: WHEN a mutant is unattributed or off-target THE SYSTEM SHALL
  require its validity to be NOT_VIOLATING and SHALL refuse a mutant that is
  both, reporting `inconsistent_attribution`.
- **SP-114~1**: WHEN an input names a host response or a consumed proof THE
  SYSTEM SHALL accept the roles `host_response` and `proof`.
- **SP-115~1**: WHEN a predicate schema is listed THE SYSTEM SHALL state its
  minor version, equal to the minor version of its kind.
- **SP-116~1**: WHEN isolation levels are compared THE SYSTEM SHALL order
  `restricted-attested`, `restricted`, `restricted-unverified`, `unrestricted`
  and `inline` from the strongest to the weakest, and rank an unknown level
  below every floor.

## Schemas and golden vectors

- **SP-22~1**: WHEN the embedded schemas are listed THE SYSTEM SHALL give each
  as a JSON Schema 2020-12 document whose `$id` is its predicate type URI, and
  resolve each predicate type URI of the format to the schema it identifies.
- **SP-24~1**: WHEN a golden vector is decoded and validated THE SYSTEM SHALL
  give the outcome and the violation codes its manifest states.
