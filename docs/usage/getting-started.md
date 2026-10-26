# Getting started

This guide takes a project from nothing to a local proof run with Claude Code.
Codex and Copilot CLI use the same commands; the differences are listed in
[harnesses](#other-harnesses).

## Requirements

- Go, to build the binaries from a clone.
- Claude Code, to run the plugins.
- Shallnot whose JSON report schema is 1.2 or later. Check it with
  `spec-proof-dp doctor`; the `shallnot-schema` line reports the result. Name a
  specific Shallnot binary with the `SPEC_PROOF_SHALLNOT` environment variable
  when several are installed.
- The test runner of your project, configured to write JUnit XML.
- For discriminating power on the JVM: a Maven or Gradle project (the module uses
  `./mvnw` or `./gradlew` when present, else `mvn` or `gradle` from `PATH`).
  For Node: `node` and `npx`, and for TypeScript projects the project's own
  TypeScript tooling (`tsx`, `ts-node` or `typescript`).

## 1. Install the binaries

The plugins call binaries by name, so each binary must be on `PATH`.

```
git clone https://github.com/RachidChabane/spec-proof
cd spec-proof
nice -n 10 go install -C discpower -p 1 ./cmd/spec-proof-dp
nice -n 10 go install -C alignment -p 1 ./cmd/spec-proof-alignment
nice -n 10 go install -C completeness -p 1 ./cmd/spec-proof-completeness
nice -n 10 go install -C verify -p 1 ./cmd/spec-proof-verify
```

`go install` writes to `$GOBIN`, or to `$(go env GOPATH)/bin` when `GOBIN` is
unset. `-p 1` keeps the build to one compiler job and `nice` keeps the machine
responsive. Add `ledger` (`spec-proof-ledger`) for CI signing, `s1relay`
(`spec-proof-s1`) for [System One](system-one.md) and `umbrella` (`spec-proof`)
for the [dispatcher](umbrella.md#the-spec-proof-dispatcher) the same way.

Check the install:

```
spec-proof-dp version
```

## 2. Install the plugins

In Claude Code:

```
/plugin marketplace add RachidChabane/spec-proof
/plugin install spec-proof@spec-proof
```

The `spec-proof` plugin declares Shallnot, the three module plugins and the
System One relay as dependencies and holds two skills, `prove-all` and `scan`. To install
modules one at a time use `/plugin install spec-proof-dp@spec-proof`,
`spec-proof-alignment@spec-proof` or `spec-proof-completeness@spec-proof`. A
module installed alone works with Shallnot alone.

## 3. Write requirements and bind tests

Requirements live in a Shallnot spec, one requirement per list item:

```markdown
# Parcel pricing

- **PAR-1~1**: WHEN an order has line items THE SYSTEM SHALL total each line as
  its unit price times its quantity.
  - Example: line_total(unit_price=4, quantity=3) => 12
```

Tests cite a requirement with `[verifies PAR-1~1]` in the form each runner
reports, for example `@pytest.mark.verifies("PAR-1~1")` for pytest, the test
title for Jest and Vitest, `@DisplayName` for JUnit 5 and the subtest name for
Go. Shallnot's [binding documentation](https://github.com/RachidChabane/shallnot/blob/main/docs/binding.md)
gives the setup per runner; `shallnot init` writes it for the runners it detects.

Inline `Example:` lines are optional and strongly useful: an example is a
concrete call with its outcome. The discriminating-power module executes
examples against mutants, and a criterion with examples reaches a pass on
executed evidence on every harness. The grammar is in Shallnot's
[spec format](https://github.com/RachidChabane/shallnot/blob/main/docs/spec-format.md#inline-examples).
A spec can also declare what it deliberately leaves out with
`Out of scope (<dimension>): <reason>`, which the completeness module honours.

## 4. Equip the project

```
shallnot init
spec-proof-dp init
spec-proof-dp enable
spec-proof-alignment init
spec-proof-alignment enable
spec-proof-completeness init
spec-proof-completeness enable
```

- `shallnot init` writes `shallnot.yaml`, `AGENTS.md` and the Shallnot skills, and
  installs the end-of-turn hook.
- `<module> init` writes `.spec-proof/config.yaml` with every proof disabled, the
  ecosystems it detects and a commented `operations` example, ignores
  `.spec-proof/cache/`, `.spec-proof/verified/` and `.spec-proof/pending/`, and
  enables the module's plugin in `.claude/settings.json`. It changes nothing of
  Shallnot's.
- `<module> enable` sets the proof to `true` in `.spec-proof/config.yaml`, adds
  its kind to `proofs.require` in `shallnot.yaml`, sets `proofs.trust: local` and
  fills `proofs.code` with the source roots of the project when `shallnot.yaml`
  sets neither, and adds its block to `AGENTS.md`. Completeness writes none of
  these into `shallnot.yaml`: Shallnot refuses it in `proofs.require` because it is
  advisory.
- `<module> disable` removes the proof's `proofs.require` entry and its block of
  `AGENTS.md` and turns the proof off; it leaves `proofs.trust` and `proofs.code`.
- `init`, `enable` and `disable` take `--check` (change nothing, exit 1 if a file
  would change) and are idempotent.

## 5. Complete the configuration

`enable` leaves `shallnot.yaml` ready for a local run; check what it wrote against
[configuration](configuration.md):

```yaml
proofs:
  require:
    - discriminating-power
    - alignment
  trust: local
  code:
    - src
```

`trust: local` tells Shallnot to read local proofs, where its default `signed`
admits only CI-signed proofs; the CI configuration of the gate sets `signed` (see
[CI](ci.md#repository-settings)). `code` names the files a proof may name as code,
taken from the layout of the project; without it, a proof that names a source file
is stale. When `enable` finds no source root it says so, and you list them
yourself. A value you set before `enable` is kept.

In `.spec-proof/config.yaml`, bind the operation names your spec examples use to
functions of your code. `init` left a commented example at the end of the file;
uncomment it and edit it (see [configuration](configuration.md#operations)):

```yaml
operations:
  line_total:
    python: parcel.rates:line_total
```

Without `operations`, no example runs and no differential search can compare
behaviour: the module reports the skipped searches as findings and escalates
every criterion that has no valid mutant.

## 6. Run the proof loop

Work as usual. When the agent ends a turn, Shallnot's hook runs the gate. A
module that owes work shows up as `awaiting_host` (or `stale_proof` when no proof
exists), and the agent runs the `prove-all` skill, which:

1. runs `doctor` of each module and names modules that are missing;
2. runs `prove --changed` of each present module, in the order discriminating
   power, alignment, completeness;
3. while a module exits with code 3, answers its tickets: one foreground blind
   executor subagent per ticket (at most 4 in one message), then
   `prove --resume`;
4. reports each module's last exit code and verdict lines.

You can run a module by hand at any time:

```
spec-proof-dp prove --changed
spec-proof-dp request list --session prove
spec-proof-dp prove --resume
```

`prove` exits with code 3 while tickets wait, 1 when a criterion blocks or
escalates, 0 when it is clean and 2 on a tool failure. Every exit code and
pending reason is in [troubleshooting](troubleshooting.md).

A module's statements are written under `.spec-proof/cache/`. The tickets, the
secret and the journal live outside the work tree, under
`$XDG_STATE_HOME/spec-proof/<module>/<repo-digest>/host/` (default
`~/.local/state`), and mutant bodies under `$XDG_CACHE_HOME/spec-proof/`
(default `~/.cache`).

## 7. Publish the local proofs and run the gate

Shallnot reads `.spec-proof/verified`, which `spec-proof-verify` writes. For local
use it accepts the unsigned statements of the cache. The `*-fulfil` skills and
`prove-all` run this step themselves when their loop ends with exit code 0 or 1,
so Shallnot's end-of-turn hook sees the proofs; by hand it is:

```
spec-proof-verify --accept-local .spec-proof/cache --out .spec-proof/verified
shallnot check
```

Run the verify step after each proof run you make by hand. `.spec-proof/pending/`
holds the pending files of the alignment and completeness modules, which `verify`
folds into the manifest when `--out` is `.spec-proof/verified`.

A criterion passes when traceability and every required proof pass. A blocking
finding names the command that resolves it.

## Other harnesses

Statements about a harness below hold as of Claude Code 2.1.295, Codex CLI 0.155.1 and
Copilot CLI 1.0.92 (`format/hostproto/harness/harness.go`).

`spec-proof-dp init --hooks claude,codex,copilot` installs the files of several
harnesses; `--hooks auto` (the default) installs Claude Code's and those of every
harness whose configuration the project already holds, and `--hooks none`
installs none.

- **Codex CLI.** `init` writes `.codex/agents/*.toml`, `.codex/hooks.json`,
  `.codex/config.toml` and `.agents/skills/<module>-fulfil/SKILL.md`. The
  `config.toml` holds the module's executor server as one table
  (`[mcp_servers.<module>-exec]`, tools approved) between `# <module>:begin` and
  `# <module>:end` comment lines; every other line of the file stays as it is, and
  `disable` removes the table. Codex executors keep the shell, so isolation is
  `unrestricted`. Codex reads these files only after the two reviews in
  [Codex: trust the project and its hooks](#codex-trust-the-project-and-its-hooks).
- **Copilot CLI.** `init` writes `.github/agents/*.agent.md`,
  `.github/hooks/<module>.json` and `.github/skills/<module>-fulfil/SKILL.md`.
  An answer is stored `restricted-unverified` and credited `restricted` when the
  stop hook verifies the answering executor's session log: pinned CLI, exactly
  the two executor tools, only the fetch and submit of its own ticket, submitted
  output matching the stored response, a bare receipt, and a bare ticket prompt
  with the shipped agent body as the sole system message (the CLI's appended
  sections are allowed). Both observation events must name the same agent and
  no mark may be present. Other Copilot answers stay `restricted-unverified`;
  Copilot never reaches `restricted-attested`. A verified run meets the default
  `min_isolation: restricted` floor and can earn judged pass credit with executed
  corroboration without lowering it. Unverified runs earn no judged pass credit
  at that floor; marked answers earn none under any floor.
- **Any other harness.** The main agent answers each ticket itself:
  `spec-proof-dp request list --session prove`, `spec-proof-dp request show <ticket>`, then
  `spec-proof-dp request submit <ticket> --nonce <nonce>` with the JSON answer on
  standard input, then `spec-proof-dp prove --resume`. Such answers have the
  isolation `inline` and are marked `exposed`: they block and escalate, and earn no
  pass credit under any floor.

Judged answers from harnesses below the isolation floor keep their power to block
and escalate but earn no pass credit; see [trust model](trust-model.md#harness-isolation).

### Codex: trust the project and its hooks

As of Codex CLI 0.155.1, Codex loads a project's `.codex/agents/`, `.codex/config.toml` and
`.codex/hooks.json` only from a trusted project, and runs a project hook only after you review it.

1. **Trust the project.** Accept the prompt Codex shows the first time it starts
   in the project, or add the project to `~/.codex/config.toml`:

   ```toml
   [projects."/absolute/path/to/project"]
   trust_level = "trusted"
   ```

2. **Review the hooks.** Start `codex` interactively in the project. The "Hooks need
   review" screen lists the hooks of `.codex/hooks.json`: read each command (every
   one runs a spec-proof binary with `hook`) and trust them. Codex stores the hash of
   each trusted hook under `[hooks.state]` in `~/.codex/config.toml`, so a hook whose
   command changes, for instance after `init` or `enable` rewrites the file, needs
   a new review.

To check the result, look for one `[hooks.state."<project>/.codex/hooks.json:…"]`
table with a `trusted_hash` per hook in `~/.codex/config.toml`.

As of Codex CLI 0.155.1, an untrusted hook does not run, in interactive sessions and under `codex exec`
alike, and neither does any hook of a `codex exec --ignore-user-config` run.
Without the hooks, the executor tool guard and the subagent records do not run.
`codex exec
--dangerously-bypass-hook-trust` runs the hooks without the review for one
invocation; keep it for automation that vets the hook commands itself.
