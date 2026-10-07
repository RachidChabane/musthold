# Umbrella (`spec-proof` plugin)

The umbrella installs Shallnot and every spec-proof module and runs them together.
It is a Claude Code marketplace and a thin plugin with no code, no binary, no hook,
no agent, no command and no server.

## Marketplace

The marketplace is named `spec-proof` and lists each plugin once:

| Plugin | Source |
|---|---|
| `shallnot` | the `plugin` directory of `RachidChabane/shallnot` |
| `spec-proof-dp` | `discpower/plugin/claude` of this repository |
| `spec-proof-alignment` | `alignment/plugin/claude` |
| `spec-proof-completeness` | `completeness/plugin/claude` |
| `spec-proof-s1` | `s1relay/plugin/claude` |
| `spec-proof` | `umbrella/plugins/spec-proof` |

Add it with:

```
/plugin marketplace add RachidChabane/spec-proof
```

The file Claude Code reads is `.claude-plugin/marketplace.json` at the root of the
repository.

## The `spec-proof` plugin

```
/plugin install spec-proof@spec-proof
```

The plugin depends on `shallnot`, `spec-proof-dp`, `spec-proof-alignment`,
`spec-proof-completeness` and `spec-proof-s1`, and holds one skill, `prove-all`.
Plugins install no binary: each binary must be on `PATH` (see
[getting started](getting-started.md#1-install-the-binaries)).

## The `prove-all` skill

`prove-all` orders the modules and interleaves their waves; each module's own
`*-fulfil` skill holds the loop that answers its tickets. The skill:

1. runs `doctor` of each module and names every module whose binary is missing. A
   missing module never counts as proved; a present module that fails its `doctor`
   is reported and left out; with no module present it stops;
2. runs `prove --changed` of each present module in the order discriminating power,
   alignment, completeness;
3. while any module exited with code 3, runs one wave for each such module in that
   order before any module starts its next wave: the relay when `spec-proof-s1` is
   on `PATH` and an endpoint is configured, then one foreground blind executor per
   ticket, then `prove --resume`. No module's wave waits for another's;
4. ends a module whose `prove` exits 0 or 1 and reports its verdict lines. For exit
   code 2, or a `host_unresponsive` finding, it reports to the user and stops that
   module without looping;
5. once every present module is done, publishes the local proofs once with
   `spec-proof-verify --accept-local .spec-proof/cache --out .spec-proof/verified`,
   which is the one command besides the module commands it pre-approves, and says
   so when `spec-proof-verify` is not on `PATH`;
6. reports, per module, the last exit code, the verdict lines, the modules that
   were missing and whether the proofs were published.

The skill pre-approves only each module's `doctor`, `prove` and `request list`, the
relay's `drain` and the exact verifier command above. It never answers a request
itself.

## Without the umbrella

Each module plugin works alone. Install `spec-proof-dp@spec-proof` and run its
`spec-proof-dp-fulfil` skill, which ends the same way: when its loop ended with
exit code 0 or 1 it runs the verifier command. The same holds for
`spec-proof-alignment` and `spec-proof-completeness`.
