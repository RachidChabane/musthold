# alignment (`musthold-alignment`)

Alignment: does each test verify what its requirement states? The module shows a
blind judge each test with the requirements under opaque tokens and the test's name
and comments masked, combines the verdict with the test's kills in the
discriminating-power proof when one exists, and issues ALIGNED, MISALIGNED or
UNDETERMINED. The verdict is judged evidence: it escalates in the gate and never
blocks alone.

## Alone

Works with Shallnot alone (report schema 1.2 or later). Without a
discriminating-power proof the judge alone may issue ALIGNED, marked
`uncorroborated`. Tests in Python, Go, JavaScript and TypeScript (Jest, Vitest),
Java and Kotlin (JUnit 5) are masked. It calls no model.

## Install

```
nice -n 10 go install -C alignment -p 1 ./cmd/musthold-alignment
```

Run from the repository root; the binary lands in `$GOBIN`, or
`$(go env GOPATH)/bin`. In Claude Code, install the plugin with
`/plugin marketplace add RachidChabane/musthold` and
`/plugin install musthold-alignment@musthold`.

## Commands

```
musthold-alignment init
musthold-alignment enable
musthold-alignment prove --changed
musthold-alignment request list --session prove
musthold-alignment prove --resume
musthold-alignment doctor
```

`prove` exits 3 while tickets wait for the host. The full command set is in
[commands](../docs/usage/commands.md).

## Documentation

- [Alignment](../docs/usage/alignment.md)
- [Configuration](../docs/usage/configuration.md#mustholdalignmentyaml)
- [Getting started](../docs/usage/getting-started.md)
- [Project README](../README.md)
