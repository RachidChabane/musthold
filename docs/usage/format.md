# Format (`format`)

`format` is a Go library, not a program. It defines the proof format that is the
only contract between the modules and the gate, and the host protocol every proof
module speaks. A module reads and writes proofs through it; you read this page when
you consume a proof file or write a module.

Module path: `github.com/RachidChabane/musthold/format`.

## Statements

Every proof is one [in-toto Statement v1](https://github.com/in-toto/attestation)
whose predicate is one of three types:

| Kind | Predicate type | Subject |
|---|---|---|
| `alignment` | `https://rachid-chabane.com/attestations/alignment/v1` | the requirement `ID~REV` and its statement digest, for one test |
| `discriminating-power` | `https://rachid-chabane.com/attestations/discriminating-power/v1` | the requirement `ID~REV` and its statement digest |
| `completeness` | `https://rachid-chabane.com/attestations/completeness/v1` | a spec file path and the SHA-256 of the file |

Each predicate has a JSON Schema 2020-12, embedded in the library, whose `$id` is
its predicate type URI. A reader ignores a member it does not know, so minor
versions are additive; the major version is in the URI.

Serialization is RFC 8785 canonical JSON: UTF-8, no timestamps, relative POSIX
paths, arrays sorted by a fixed key. Canonicalizing canonical bytes leaves them
unchanged, and a statement that is not its own canonical form is refused.

A predicate carries:

- `module {name, version}`, `producedBy` (`local` or `ci`) and `cacheKey`, the
  SHA-256 over the canonical form of the subject and the inputs;
- `inputs[]`, sorted descriptors with a role: `spec`, `test_source`, `code_tree`,
  `junit_result`, `mutant_set`, `prompt_template`, `decision_policy`,
  `example_binding`, `host_response` and `proof`. `code_tree` and `test_source`
  are one descriptor per file, named by the path relative to the directory holding
  `shallnot.yaml`, with the SHA-256 of the file's exact bytes. A `proof` input names
  the kind it consumes, carries the requirement in `annotations.criterion` and the
  SHA-256 of the consumed statement file;
- claims `{value, class, confidence?}`, where `class` is `executed` or `judged`; a
  judged claim has a confidence in [0,1] and an executed claim has none;
- an `executor` for judged claims: channel, harness, isolation, attestation, model
  and prompt digest;
- `corroboratedBy[]` for a judged `VALID` mutant: the executed evidence that backs
  it;
- `extensions.decision` with `confidenceSource` (`execution`, `host-votes(<k>)`,
  `host-classify(<k>,<scheme>):<model>*<n>[+<model>*<n>]` or
  `system-one:<model>@<endpoint digest>`), the id of the mapping that derived the
  confidence (`host-classify.share-isotonic@<table digest>` for a classification),
  the deciders and the threshold applied. A classification also lists, in
  `classify`, every call it was made of, the seed input of each question's label
  order and the version of the host that made the calls.

A completeness predicate has `advisory: true` and no verdict field.

The library reports a violation with a code such as `judged_without_confidence`,
`executed_with_confidence`, `confidence_range`, `wrong_class`, `unsorted`,
`unknown_value`, `bad_digest`, `cache_key_mismatch`, `uncorroborated_claim` or
`input_descriptor`. Golden vectors under `format/testdata/golden` fix the outcome
and codes for valid and invalid statements; every module and Shallnot's reader run
them.

## Scan statements

`musthold scan` records what it ran in a scan statement: an in-toto Statement v1
with the predicate type `https://rachid-chabane.com/attestations/scan/v1`
([ADR 0014](../adr/0014-scan.md)). It is not a proof. It has no `requirement`, its
subject is the scanned commit with the file-set digest of its tree, and every
reader of proofs refuses its predicate type: `Decode` and `Admit` return
`ErrUnknownPredicateType`, the module schema checks do the same, and the gate
ignores it. `DecodeScan`, `AdmitScan` and `ScanSchema` read it.

A scan predicate carries:

- `module`, `producedBy`, `mode` (`repo`, or `pr` with its `base` commit),
  `runner`, the decimal `seed`, `partial` when the budget stopped it, and
  `promisesChecked`;
- `counts` of the suite's tests by class: `verifiesNothing`, `caught`,
  `unproven`, `notSampled` and `notGreen`, adding up to `tests`. The claim is read
  against the sampled green tests, the sum of the first three;
- `mutants[]` with their bodies: `edits` in the wire form of a mutant, whose
  digest is the mutant `id`, the mutated `lines`, the `origin` (`operator` or
  `model`), `sentinel`, and the `routes` that prove the mutant changes behaviour
  (`killed_by_test`, `f3_difference`, `witness_failure`);
- `tests[]`, one per sampled green test: its class, the reason when it is
  unproven, its outcome on each proven change in the lines it ran, and the
  promises that label it;
- with a host, `promises[]` with their quote and source digest, `witnesses[]`,
  `brokenPromises[]` and `questions[]`.

`CheckQuote` accepts a promise only when its source has the recorded digest and
holds the quote byte for byte. Golden vectors under `format/testdata/scan` fix the
violation codes and the schema result of scan statements.

## Layout of proofs

| Form | Path |
|---|---|
| bare statement | `<module>/<subject key>/<cacheKey>.json` |
| signed proof | `proofs/<module>/<subject key>/<cacheKey>.dsse.json` |

The subject key is the subject name with `%`, `/` and `\` percent-encoded.

## Pending files

A module's pending file is `.musthold/pending/<module>.json`: a module name and
entries, each a requirement `ID~REV` with a reason of the closed set
`awaiting_host`, `awaiting_proof`, `awaiting_host_judgment`, `host_unresponsive`,
strictly ascending. See [troubleshooting](troubleshooting.md#pending-reasons).

## Host protocol

`format/hostproto` is the protocol between a proof module and the harness that
answers its model work. It defines the request, ticket and response types and their
JSON Schemas, the request id, the ticket lifecycle, the capability check, decoders
for the hook payloads of Claude Code, Codex and Copilot, and the executor MCP
server over standard input and output. It imports no `os`, `net` or `os/exec`.
The lifecycle is described in [architecture](../architecture.md#host-protocol).
