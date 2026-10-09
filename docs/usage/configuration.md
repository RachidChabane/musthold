# Configuration

spec-proof reads four files and one environment surface.

| File | Written by | Read by | Purpose |
|---|---|---|---|
| `.spec-proof/config.yaml` | `init`, `enable`, `disable`, you | every module, the relay | enabled proofs, ecosystems, operations, discriminating-power policy, host settings, System One endpoint |
| `.spec-proof/alignment.yaml` | you | `spec-proof-alignment` | decision policy of the alignment module |
| `.spec-proof/completeness.yaml` | you | `spec-proof-completeness` | decision policy of the completeness module |
| `shallnot.yaml` | `shallnot init`, `enable`, you | Shallnot | specs, tests, results and the `proofs:` block that governs the gate |
| `.spec-proof/trust-policy.yaml` | you | `spec-proof-verify` | pinned keys and signers for signed proofs |

Unknown keys are refused wherever a module owns the file. An absent member keeps
its default. Every parameter reaches the policy digest, so changing one makes the
affected proofs stale.

## `.spec-proof/config.yaml`

```yaml
proofs:
  alignment: true
  discriminating-power: true
  completeness: true
ecosystems:
  - pytest
operations:
  line_total:
    python: parcel.rates:line_total
policy:
  tau_judge: 0.70
host:
  sampling: on
  require_attestation: true
systemone:
  endpoint: http://localhost:11434
  model: example-model
```

| Key | Type | Default | Meaning |
|---|---|---|---|
| `proofs.<kind>` | boolean | `false` | Whether the proof is on. Kinds: `alignment`, `discriminating-power`, `completeness`. Set by `enable` and `disable`. |
| `ecosystems` | list | detected by `init` | Any of `pytest`, `jest`, `vitest`, `maven`, `gradle`, `go`. |
| `operations` | mapping | none | Binds the operation names of spec examples to code; see below. `init` writes a commented example of it. |
| `policy` | mapping | defaults below | Decision policy of discriminating power. |
| `host.sampling` | `on` | off | Lets the executor server offer `drain` to a client that declares MCP sampling. |
| `host.require_attestation` | boolean | `true` | When `true`, an executor answer without a capability issued by the harness hooks is refused. `false` records such answers as `restricted`. |
| `systemone.endpoint` | URL | none | Endpoint the relay posts closed questions to. |
| `systemone.model` | string | none | Model named in each relayed request. |
| `systemone.classify` | `auto`, `shadow`, `off`, or a mapping of `default` and proof kinds to those | `auto` | Classification by the host's mod: `auto` serves only the admitted entries, `shadow` journals and never answers, `off` does nothing. See [System One](system-one.md#classification-by-the-host). |
| `host.classify.witness_dir` | path | none | The directory of the user's own raw API log (`OTEL_LOG_RAW_API_BODIES=file:<dir>`), against which classifications are checked at `--resume`. The product never sets the log. |

The `alignment`, `completeness` and `systemone` sections of the file are
tolerated by the modules that do not own them. `spec-proof-dp` refuses any other
unknown top-level key.

### Operations

A spec example `line_total(unit_price=4, quantity=3) => 12` names an operation.
`operations` maps each operation name to the code it calls, under an ecosystem
name or its language:

| Ecosystem | Language key | Target form |
|---|---|---|
| `pytest` | `python` | `package.module:function` |
| `go` | `go` | `import/path:Function` or `import/path:Type.Method` |
| `jest`, `vitest` | `typescript` | `path/file.ts:function` or `path/file.ts:Class.method` |
| `maven`, `gradle` | `java` | `package.Class:method` |

```yaml
operations:
  line_total:
    python: parcel.rates:line_total
  quote:
    go: example.com/parcel/rates:Quote
```

An ecosystem key wins over its language key. A target must name an exported or
public callable that exists; an overloaded JVM method is refused.

### Discriminating-power policy

`policy` accepts these members; each default holds when the member is absent.

| Key | Default | Meaning |
|---|---|---|
| `tau_judge` | `0.70` | Confidence at which a judged verdict about a mutant has power. A number or a per-source mapping. |
| `judge_k` | `2` | Blind judge tickets per judgment; all must reach `tau_judge`. |
| `tau_screen` | `0.20` | Probability of "violates" at which a surviving mutant joins the pool. A number or a per-source mapping. |
| `decide_k` | `3` | Host votes of a closed question when no System One endpoint answers. |
| `min_valid` | `3` | Valid mutants the search needs before it stops. |
| `min_sites` | `2` | Distinct mutation sites those mutants must cover. |
| `max_rounds` | `3` | Generation rounds. |
| `k_op` | `400` | Operator mutants kept per run. |
| `max_host_tickets` | `40` | Host tickets one criterion may use; an exhausted budget escalates with `budget_exhausted`. |
| `signed_pass_basis` | `corroborated` | `executed-only`, `corroborated` or `judged`; see [trust model](trust-model.md#claim-power). |
| `min_isolation` | `restricted` | Isolation floor below which a judged answer earns no pass credit. |

### Thresholds per confidence source

A threshold is a bare number, which applies to every source, or a mapping of
`default` and sources to numbers in [0,1]:

```yaml
policy:
  tau_judge:
    default: 0.70
    host-votes: 0.80
    "host-votes(3)": 0.85
    system-one: 0.90
```

A source is a kind (`execution`, `host-votes`, `host-classify`, `system-one`) or
one exact source: `host-votes(<k>)`, `host-classify(<k>,<scheme>):<voters>` or
`system-one:<model>@<endpoint digest>`. The exact source wins over its kind, and
the kind over `default`. A classification counts as the closed signal, or screens
a survivor out, only when its exact source has an entry of its own.

## `.spec-proof/alignment.yaml`

```yaml
policy:
  align_k: 5
  modal_min: 0.80
```

| Key | Default | Meaning |
|---|---|---|
| `tau_route` | `0.90` | Confidence a System One answer needs for the two-signal ALIGNED verdict. A number or a per-source mapping. |
| `align_k` | `5` | Blind judge tickets per alignment request. |
| `modal_min` | `0.80` | Modal share a judge verdict needs. A number or a per-source mapping. |
| `theta_own` | `0.50` | Share of its criterion's valid mutants a test must kill to be SPECIFIC rather than PARTIAL. |
| `theta_other` | `0.20` | Largest share of kills outside its criterion a SPECIFIC test may have. |
| `theta_none` | `0.20` | Share of unattributed mutants among a test's kills above which it is marked as pinning off-spec behaviour. |
| `shadow_rate` | `0.10` | Share of two-signal ALIGNED verdicts the judge audits. |
| `max_reasoning_calls` | `40` | Judge and decider tickets one test may use. |
| `decide_k` | `3` | Host votes of a closed question when no System One endpoint answers. |
| `two_signal` | `true` | Enables the System One plus matrix ALIGNED verdict; when `false` only the judge issues ALIGNED. |
| `judge_alone` | `true` | Lets a judge verdict issue ALIGNED without a SPECIFIC or OVERBROAD matrix; when `false` such a verdict is UNDETERMINED. |
| `min_isolation` | `restricted` | Isolation floor for judged answers. |

## `.spec-proof/completeness.yaml`

```yaml
policy:
  tau_adv: 0.50
  max_hunks: 50
```

| Key | Default | Meaning |
|---|---|---|
| `tau_adv` | `0.50` | Confidence at or above which a closed answer raises a question. A number or a per-source mapping. |
| `decide_k` | `3` | Host votes of a closed question when no System One endpoint answers. |
| `max_hunk_bytes` | `6000` | Bound on the text of a changed function shown to an executor. |
| `max_hunks` | `50` | Changed units asked about per spec, in file and line order; the rest are reported. |

## `shallnot.yaml`: the `proofs` block

Shallnot reads proofs only when `shallnot.yaml` has a `proofs` block. Unknown keys
are refused. Shallnot's
[configuration reference](https://github.com/RachidChabane/shallnot/blob/main/docs/configuration.md#the-proofs-block)
is authoritative; the keys are:

| Key | Default | Meaning |
|---|---|---|
| `verified_dir` | `.spec-proof/verified` | Directory of the verified set, with its `manifest.json`. |
| `require` | `[]` | Kinds every covered requirement must hold a valid proof of: `alignment`, `discriminating-power`. `completeness` is refused. |
| `code` | `[]` | Files, directories or globs a proof's `code_tree` inputs may name. A proof that names a file outside them is stale. `enable` fills it from the layout of the project when it is empty. |
| `trust` | `signed` | `signed` admits only statements produced by CI, in a set whose manifest the verifier wrote after checking signatures; `local` also admits unsigned local statements. `enable` sets `local` when the key is absent; the CI configuration of the gate sets `signed`. |
| `thresholds.validity` | `0.7` | Confidence at which a judged claim about a mutant has power. Number or per-source mapping. |
| `thresholds.alignment` | `0.7` | The same for alignment verdicts. |
| `escalation` | `error` | `error` fails the verdict on an escalating finding; `warning` only reports it. |
| `signed_pass_basis` | `corroborated` | `executed-only`, `corroborated` or `judged`. |
| `min_isolation` | `restricted` | Isolation floor for judged pass credit. |
| `max_host_exclusions` | `2` | Survivors a criterion may exclude on a judgment alone. |

```yaml
proofs:
  trust: signed
  code:
    - src/**
  require:
    - discriminating-power
    - alignment
  thresholds:
    validity: 0.70
    alignment:
      default: 0.70
      host-votes: 0.80
      system-one: 0.90
  signed_pass_basis: corroborated
  min_isolation: restricted
```

`signed_pass_basis` exists in the discriminating-power policy and in
`shallnot.yaml`; `min_isolation` exists in the policy of each proof module and in
`shallnot.yaml`. A module applies its own value when it decides; Shallnot applies
its own value when it composes the result. Keep them equal.

## `.spec-proof/trust-policy.yaml`

`spec-proof-verify` reads the trust policy from the directory that holds `--out`.
Version 1 pins Ed25519 public keys (base64 raw keys) and, optionally, a Sigstore
issuer with the build signers allowed:

```yaml
version: 1
ed25519:
  - publicKey: <base64 raw Ed25519 public key>
sigstore:
  issuer: https://token.actions.githubusercontent.com
  signers:
    - uri: https://github.com/<owner>/<repo>/.github/workflows/<workflow>.yml@refs/heads/main
      digest: <lowercase hex commit digest>
```

A policy without a trust anchor, or with a malformed or repeated key or signer,
is refused. The SHA-256 of the file's exact bytes becomes `policyDigest` in the
manifest.

## Environment

| Variable | Read by | Meaning |
|---|---|---|
| `SPEC_PROOF_SHALLNOT` | the three proof modules | Path of the Shallnot binary to use instead of `shallnot` on `PATH`. |
| `XDG_STATE_HOME` | the three proof modules | Root of the host store. Used when absolute; default `~/.local/state`. |
| `XDG_CACHE_HOME` | the three proof modules | Root of the mutant and decision cache. Used when absolute; default `~/.cache`. |
| `CLAUDE_PROJECT_DIR` | `hook`, `mcp` | Project root Claude Code exports to hooks and servers. |
| `SPEC_PROOF_CI_RUN_ID`, `GITHUB_RUN_ID` | `prove --ci` | The CI run that draws the seed of a commit; the first that is set wins. |
| `SPEC_PROOF_ED25519_KEY` | `spec-proof-ledger sign` | PKCS#8 PEM Ed25519 private key; excludes `--key-file`. |
| `SIGSTORE_ID_TOKEN` | `spec-proof-ledger sign --identity sigstore` | Identity token; GitHub Actions' token service is used when it is absent. |
| `MEM_GUARD_MIN_FREE_PCT` | `spec-proof-s1 drain --mem-guard` | Minimum free memory percentage on macOS (default 40). |
