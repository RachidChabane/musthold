# completeness (`musthold-completeness`)

Completeness: what does the specification leave unstated about what a change does?
The module raises advisory questions: dimensions of a checklist the spec does not
cover, changed code that no example and no ALIGNED test justifies, behaviour that
mutants showed no requirement constrains, and requirements that are ambiguous or
untestable. Its proof has no verdict, so the gate reports the questions and never
blocks on them.

## Alone

Works with Shallnot alone (report schema 1.2 or later), and `enable` works in a
project with no `shallnot.yaml`. It reads the discriminating-power and alignment
proofs when they exist and asks about every changed unit when they do not.
Indexes Python, Go, JavaScript, TypeScript, Java and Kotlin sources. It calls no
model.

## Install

```
nice -n 10 go install -C completeness -p 1 ./cmd/musthold-completeness
```

Run from the repository root; the binary lands in `$GOBIN`, or
`$(go env GOPATH)/bin`. In Claude Code, install the plugin with
`/plugin marketplace add RachidChabane/musthold` and
`/plugin install musthold-completeness@musthold`.

## Commands

```
musthold-completeness init
musthold-completeness enable
musthold-completeness prove --changed
musthold-completeness request list --session prove
musthold-completeness prove --resume
musthold-completeness doctor
```

`prove` exits 3 while tickets wait for the host. The full command set is in
[commands](../docs/usage/commands.md).

## Documentation

- [Completeness](../docs/usage/completeness.md)
- [Configuration](../docs/usage/configuration.md#mustholdcompletenessyaml)
- [Getting started](../docs/usage/getting-started.md)
- [Project README](../README.md)
