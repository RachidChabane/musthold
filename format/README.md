# format

A Go library, not a program: the proof format shared by every module and the host
protocol every proof module speaks. It is the only package the modules share.

- the three Statements (alignment, discriminating power, completeness) as in-toto
  Statement v1, their JSON Schemas, RFC 8785 canonicalization, cache keys, input
  descriptors and pending files;
- the scan statement (`scan/v1`), the executed record of `musthold scan`,
  which no reader of proofs admits;
- `hostproto`: requests, tickets, responses, the lifecycle, capabilities, hook
  payload decoders for Claude Code, Codex and Copilot, and the executor server;
- golden vectors that every module and Shallnot's reader run.

The packages other than test support import no `os`, `io/fs`, `os/exec` or `net`.

## Use

Module path: `github.com/RachidChabane/musthold/format`. Test it with
`script/test format` from the repository root.

## Documentation

- [Format](../docs/usage/format.md)
- [Architecture](../docs/architecture.md)
- [Project README](../README.md)
