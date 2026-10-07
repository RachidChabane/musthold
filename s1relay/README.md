# s1relay (`spec-proof-s1`)

The optional relay of closed questions to a System One endpoint, and the only
program of the project that opens a connection to one. It exports the closed
requests a proof module is waiting for, posts them to the endpoint one at a time and
imports the answers under the source `system-one:<model>@<endpoint digest>`. Without
it, the proof modules take host votes instead.

## Alone

Needs a proof module binary on `PATH` and an endpoint, from `--endpoint` or from
`systemone.endpoint` in `.spec-proof/config.yaml`.

## Install

```
nice -n 10 go install -C s1relay -p 1 ./cmd/spec-proof-s1
```

Run from the repository root; the binary lands in `$GOBIN`, or
`$(go env GOPATH)/bin`. In Claude Code, install the plugin with
`/plugin marketplace add RachidChabane/spec-proof` and
`/plugin install spec-proof-s1@spec-proof`.

## Commands

```
spec-proof-s1 drain --module dp
spec-proof-s1 drain --module alignment --max-requests 10 --mem-guard
spec-proof-s1 version
```

Exit codes: 0 no request unanswered, 1 some unanswered, 2 tool failure, 4 the memory
rule refused.

## Documentation

- [System One and the relay](../docs/usage/system-one.md)
- [Project README](../README.md)
