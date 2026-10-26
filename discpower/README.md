# discpower (`musthold-dp`)

Discriminating power: do the tests bound to a requirement fail on code that
violates it? The module builds mutants of the code a requirement depends on, runs
the bound tests against them by execution and records which tests kill which
mutants. A requirement whose tests let a valid mutant survive blocks the gate.

## Alone

Works with Shallnot alone (report schema 1.2 or later). Supports pytest, Go, Jest,
Vitest and JUnit 5 with Maven or Gradle (Java and Kotlin). It calls no model: the
harness's model, in an isolated subagent, writes host mutants and judges validity,
and execution decides.

## Install

```
nice -n 10 go install -C discpower -p 1 ./cmd/musthold-dp
```

Run from the repository root; the binary lands in `$GOBIN`, or
`$(go env GOPATH)/bin`. In Claude Code, install the plugin with
`/plugin marketplace add RachidChabane/musthold` and
`/plugin install musthold-dp@musthold`.

## Commands

```
musthold-dp init
musthold-dp enable
musthold-dp prove --changed
musthold-dp request list --session prove
musthold-dp prove --resume
musthold-dp doctor
musthold-dp scan
```

`prove` exits 3 while tickets wait for the host. `scan` needs no spec and no
configuration: it names the tests of any pytest, Go, Jest or Vitest repository that still pass
when the code they run is broken ([scan](../docs/usage/discpower.md#scan)). The full command set is in
[commands](../docs/usage/commands.md).

## Documentation

- [Discriminating power](../docs/usage/discpower.md)
- [Configuration](../docs/usage/configuration.md)
- [Getting started](../docs/usage/getting-started.md)
- [Project README](../README.md)
