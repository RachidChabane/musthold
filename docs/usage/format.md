# Format (`format`)

`format` is a Go library, not a program. It defines the proof format that is the
only contract between the modules and the gate, and the host protocol every proof
module speaks. A module reads and writes proofs through it; you read this page when
you consume a proof file or write a module.

Module path: `github.com/RachidChabane/spec-proof/format`.

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

## Layout of proofs

| Form | Path |
|---|---|
| bare statement | `<module>/<subject key>/<cacheKey>.json` |
| signed proof | `proofs/<module>/<subject key>/<cacheKey>.dsse.json` |

The subject key is the subject name with `%`, `/` and `\` percent-encoded.

## Pending files

A module's pending file is `.spec-proof/pending/<module>.json`: a module name and
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
