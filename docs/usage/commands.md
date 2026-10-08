# Commands shared by the proof modules

`spec-proof-dp` (discriminating power), `spec-proof-alignment` and
`spec-proof-completeness` have the same command line. This page uses
`spec-proof-dp`; replace the binary name for the other modules. The parts that
differ are in the page of each module: [discriminating power](discpower.md),
[alignment](alignment.md) and [completeness](completeness.md).

Run the binary with no argument to print the list of commands on standard error
(exit 2), or with `-h`, `--help` or `help` to print it on standard output (exit 0).
`<command> -h` or `--help`, and `help <command>`, print the synopsis and the flags
of that command and exit 0; for a command that takes a verb, such as `request`,
`systemone` and `host`, `-h` lists its verbs and `<command> <verb> -h` lists
exactly the flags of that verb. A flag a command does not define is a usage error
(exit 2) that prints the failure and the usage of the command on standard error.
Flags are accepted as `--flag` or `-flag`. The other binaries (`spec-proof-ledger`,
`spec-proof-verify`, `spec-proof-s1`) answer the same help forms.

```
spec-proof-dp <version|prove|request|systemone|doctor|init|enable|disable|mcp|hook|host>
```

Every command that works on a project takes `--root DIR` (default: the working
directory), except `init`, `enable` and `disable`, which take `--dir DIR`.

## version

```
spec-proof-dp version
```

Prints `<binary> <version>`.

## init, enable, disable

```
spec-proof-dp init [--dir DIR] [--hooks auto|none|claude,codex,copilot] [--check]
spec-proof-dp enable [--dir DIR] [--check]
spec-proof-dp disable [--dir DIR] [--check]
```

All three refuse to run when the Shallnot found (`SPEC_PROOF_SHALLNOT`, else
`shallnot` on `PATH`) speaks a report schema below 1.2. Each prints one line per
file it creates, updates or leaves `unchanged`, then the next steps. With
`--check` nothing changes and the exit code is 1 when a file would change.

- `init` writes the module's seed of `.spec-proof/config.yaml` (every proof off,
  the ecosystems detected, and a commented `operations` example; the example is a
  comment because a binding is valid only once it names a callable that exists, so
  you uncomment it and edit it), ignores `.spec-proof/cache/`,
  `.spec-proof/verified/` and `.spec-proof/pending/` in `.gitignore`, and installs the files of each selected
  harness by merging into files other tools share and writing the files it owns.
  `--hooks auto` installs Claude Code's files and those of every harness whose
  configuration the project holds. For Codex, `init` and `enable` keep the
  module's executor server as one marked table of `.codex/config.toml`, replaced in
  place and never touching another line. A second run changes nothing. Shallnot's
  configuration, its `AGENTS.md` block and its hooks are never touched.
- `enable` sets the module's proof to `true` in `.spec-proof/config.yaml`, adds its
  kind to `proofs.require` in `shallnot.yaml` and adds its block to `AGENTS.md`,
  by line edits that keep every other byte. In `shallnot.yaml` it also sets
  `proofs.trust: local`, so that Shallnot reads the proofs made on the machine,
  when no trust is set, and fills `proofs.code` with the source roots of the
  project when it lists none: `**/*.go` for a Go module, `src` for a Node project
  that has it, `src/main` for a Maven or Gradle project that has it, and `src` or
  each top-level package for a Python project. A value that is already set is never
  changed, and `enable` says so when it finds no source root. The CI configuration
  of the gate sets `proofs.trust: signed` (see [CI](ci.md#repository-settings)). It
  refuses a project with no `shallnot.yaml` and a layout it cannot extend without
  rewriting. Completeness writes no `proofs.require`, `proofs.trust` or
  `proofs.code` entry and works without a `shallnot.yaml`.
- `disable` turns the proof off and removes its `proofs.require` entry, its
  block of `AGENTS.md` and its executor server table in `.codex/config.toml`
  (the file too when that table was its only content). It leaves `proofs.trust`
  and `proofs.code`, which the other modules rely on.

## prove

```
spec-proof-dp prove [--changed | --all] [--resume] [--record DIR | --replay DIR] [--root DIR]
spec-proof-dp prove --ci [--import-host DIR] [--root DIR]
```

| Flag | Meaning |
|---|---|
| `--changed` | Prove what changed since its last proof. The default. |
| `--all` | Prove everything again. Excludes `--changed`. |
| `--resume` | Continue a run that is awaiting the host. Refused when no request was ever emitted. |
| `--record DIR` | Keep each consumed answer and its executor under the request id in `DIR`. |
| `--replay DIR` | Answer every pending ticket from the recordings in `DIR`. A request without a recording is a hard error. Excludes `--record`. |
| `--ci` | Run as CI does: produce statements as CI and leave unanswered requests in `.spec-proof/ci-issued/<module>/`, one directory per module, for the CI job to upload. Discriminating power also draws the seed of the head commit and needs `SPEC_PROOF_CI_RUN_ID` or `GITHUB_RUN_ID`. Excludes `--record` and `--replay`. |
| `--import-host DIR` | With `--ci`: serve the tickets the run asks for from the bundle a `host push` published. |
| `--root DIR` | Project root. |

Completeness adds `--base REF`, the reference the change is measured from.

`prove` is a resumable state machine. A run does the deterministic work it can,
emits tickets for what needs a model, prints them as `awaiting_host`, and exits
with code 3. Each run, including `--resume`, advances one wave. Running `prove`
again without answering gives the same requests.

The first line is `wave N`. A final `awaiting_host N tickets` line is printed while
tickets wait. Lines in between are specific to each module. Findings are printed
as `  finding <name>` under a criterion and `module finding <name>` for the run.

Exit codes: 0 clean; 1 blocked or escalated (the meaning is per module), or a
refusal such as `--ci` without a CI run, a bundle of another module version that left
requests unanswered, or a bundle that cannot be decoded; 2 tool failure or usage
error such as conflicting flags; 3 `awaiting_host`.

## request

```
spec-proof-dp request list [--json] [--root DIR]
spec-proof-dp request show <ticket> [--cap CAP] [--root DIR]
spec-proof-dp request submit <ticket> --nonce NONCE [--cap CAP] [--root DIR]
```

A ticket is `<requestId>.<vote>.<emission>`.

- `list` prints `<ticket> <role>` per pending ticket; with `--json` it prints the
  listing, including each ticket's nonce and view.
- `show` prints what an executor receives: the ticket, its nonce, the prompt, the
  response schema and, for closed requests, the question set. Showing a ticket
  outside an executor is journaled as `exposed`.
- `submit` reads the JSON answer from standard input and prints a receipt. A
  schema violation is reported back and one correction is allowed. A different
  second answer for an accepted ticket is refused as `resubmission_conflict`.

While attestation is required, `show` and `submit` need a capability that only a
harness hook issues. These commands exist for scripts, the relay, test fakes and
harnesses with no executor support. A refused answer exits 1.

## systemone

```
spec-proof-dp systemone export [--root DIR]
spec-proof-dp systemone import <ticket> --nonce NONCE --source SOURCE [--root DIR]
```

`export` prints the closed requests that wait for their first vote, as JSON, with
each ticket, nonce and question set. `import` reads the endpoint's answers from
standard input and submits them under `SOURCE`, which must have the form
`system-one:<model>@<endpoint digest>`. The relay `spec-proof-s1 drain` calls
both; see [System One](system-one.md).

## doctor

```
spec-proof-dp doctor [--root DIR]
```

Prints one `ok` or `fail` line per check: Shallnot found and its report schema
(accepts 1.2 and refuses 1.1), then the checks that depend on the module (the
ecosystem, the version control tool, the policy, the harness files the project
holds), then the host store and its journal chain. Exits 2 when a check fails.

## mcp and hook

```
spec-proof-dp mcp --role executor [--harness claude|codex|copilot] [--root DIR]
spec-proof-dp hook <pre-tool|subagent-start|subagent-stop|post-tool> [--harness claude|codex|copilot] [--root DIR]
```

Harness plumbing, started by the plugin files and not by you.

- `mcp --role executor` serves the executor tools `fetch_request` and
  `submit_response` over standard input and output until the client closes the
  stream. It adds `drain` only when the client declares MCP sampling and
  `host.sampling: on` is set.
- `hook` answers one harness hook from the payload on standard input. It does
  nothing in a project that has no configuration of the module, never blocks
  when it cannot read its payload (it exits 1), and denies a call of the module's
  own tools that it cannot check.

An unknown role, harness or hook exits 1.

## host

```
spec-proof-dp host push [--remote NAME] [--root DIR]
spec-proof-dp host pull --ci-run ID [--repo OWNER/NAME] [--artifact NAME] [--root DIR]
```

- `push` publishes every answered ticket, with the profile it is credited with and
  the marks against it, as a tree under the ref
  `refs/spec-proof/host/<module>/<head sha>`, using git plumbing. It touches
  neither the working tree nor the current branch. `--remote NAME` also pushes the
  ref. It warns when a tracked file differs from the head commit, refuses a store
  whose journal chain is broken, and exits 1 when nothing is answered. `<module>`
  is `discpower`, `alignment` or `completeness`.
- `pull` downloads, with `gh run download`, the requests a CI run left for the
  host (`--artifact` defaults to `spec-proof-host-<module>`) and makes tickets of
  them once. It reads them from the root of the artifact, or from the directory of
  the module below it when the artifact holds `.spec-proof/ci-issued` as a whole.

See [CI](ci.md) for the workflow.
