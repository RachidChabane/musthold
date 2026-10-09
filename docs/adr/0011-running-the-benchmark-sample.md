# ADR 0011: Running the benchmark sample

Status: accepted. Implements ADR 0007 ("Benchmark by samples") on top of ADR 0004,
with the two deviations listed under Decisions 3 and 4.

## Context

The benchmark core (`bench/`) holds the domain, pacing, construction, sealing,
oracle runners and analysis, but nothing that drives a real coder session or
gate-host session. Ten design choices were left open. Each was decided by a
debate between two advocates, with a blind judge who checked the decisive claims
against the code and the installed CLIs.

## Decisions

1. **Host port per session.** `GateHost.run_gate(tree, arm, record_dir, mode)`
   takes `LIVE` or `REPLAY` and returns a `GateOutcome`.
   - The outcome carries the report, the status (complete, paused or
     `harness_misconfigured`), the session id, the executions and the usage per
     ticket.
   - ADR 0007 defines `IsolatedCliHost` as one session running `prove-all`. A
     session per request would double the executions and bypass the shipped
     fulfil skill.
   - `HostExecutor` remains only for single-shot replays (judge replay, local
     models). An import contract keeps it out of the trunk and branch engine.
   - The clean room lives at `runs/<run>/cleanroom/<task>/<arm>/<tree-digest>/`,
     so a killed gate resumes from its pending tickets.
2. **Budget unit: one model execution, reserved before each live gate.**
   - The ledger holds charged and reserved executions per night and harness. A
     gate is admitted only if charged plus reserved plus its reservation fits
     the budget.
   - Both the reservation and the charge belong to the night of admission. A
     killed session settles from its streamed events.
   - The pilot's `claude` key is read as `claude-code`.
   - The manifest records `budget_unit: model-execution`.
3. **Operator mutants come from the product's engine, through a bench-only
   binary.**
   - `discpower/cmd/spec-proof-dp-opmutants --base DIR --merge DIR --json` covers
     every touched line with the product's own screen, without sampling. Its
     output is sorted and versioned.
   - It is never released and never on a session `PATH`. Its digest is in the
     manifest.
   - **Deviation**, logged at the start of the run: `protocol | arms: none |
     Operator mutants for verify-task step 5 and the op cell of SB_coder come
     from spec-proof-dp-opmutants, a model-free binary built from the
     discriminating-power module's operator engine at the pinned commit,
     outside the release set; its digest is in manifest.json.`
4. **Trunks and branched arms are gated after each session and resumed with
   Shallnot's message.**
   - Steps:
     1. The session ends.
     2. The tree is snapshotted by digest.
     3. The trunk runs its tests, or the arm runs its gate.
     4. A synthesized Stop payload goes to Shallnot's real stop hook.
     5. On a block, the session resumes with the message verbatim: `--resume`,
        adding `--fork-session` on the first branch step, or `codex exec
        resume`.
   - ALL-native and the cheat arm keep the in-session Stop hook of the plugin
     host.
   - **Deviation**, logged at the start of the run: `protocol | arms: N, T, TR,
     AL, DP, CO, ALL, PL | Stop hooks fire under claude -p and codex exec, yet
     trunks and branched arms are gated after each session ends and resumed
     with Shallnot's stop-hook message verbatim, so that every attempt is a
     keyed resumable call, pauses never fall inside a hook, and no gate-host
     session nests inside a coder session; ALL-native receives Shallnot's
     message through its in-session Stop hook instead of a resume prompt, so E5
     measures branching, host and feedback channel together.`
5. **Codex branches use `codex exec fork <session> [prompt]`**, the primary path
   of ADR 0004.
   - Neither `fork` nor `resume` takes `-s` or `-C`. The sandbox, network and
     web search are set with `-c` flags.
   - A canary checks before branching starts: a new session id, an unchanged
     parent rollout, no network, and the frozen model. On a canary failure, ADR
     0004's own fallback (a fresh session seeded with the S_T snapshot and the
     findings) is logged as its deviation.
6. **Spec-gap data is part of construction, before the first seal.**
   - The spec author records, per criterion, a `dimension` and its
     `implementingHunks`. A deterministic validator requires the hunks to lie in
     the non-test merge diff.
   - Two gap criteria are drawn by seed at sealing.
   - Sealed bundles move to schema version 2, and `seal()` refuses older ones.
     The guard lands before construction's first night.
7. **A `night-jobs sample` slot at 21:37, a machine-wide limit of two slots, and
   one budget computation.**
   - The slots are `fcntl` locks, so a dead holder frees its slot.
   - `script/with-slot` wraps the bash jobs: the pilot holds two slots, the
     canaries one and construction one.
   - `bench/script/night-budget <job>` hands out the nightly cap in priority
     order: pilot, canary reserve (the canaries' measured executions),
     construction while any selected task is unsealed, then the sample.
8. **The cheat arm's CI import goes through `--import-host` directories fed by
   the real `host push`.**
   - The workspace's `origin` is its own `.git`, so no write leaves it.
   - The bench exports each module's ref and runs `prove --ci --import-host` in
     generate order.
   - It runs `host pull` through a `gh` shim the session cannot reach.
9. **The bench's settings live outside the workspace.** In the plugin-host arms,
   the product's own files stay where `init` puts them.
   - **Claude:** each trunk has its own `CLAUDE_CONFIG_DIR` under the run, with
     `--settings` from the run. Honest arms use `--setting-sources user`, which
     is that bench-owned directory, so project settings an agent writes never
     load on resume. Plugin-host arms use `user,project`. The credential is
     `CLAUDE_CODE_OAUTH_TOKEN`, as on the canary's isolated path. A session
     sharing the owner's configuration could read the owner's own instructions
     (`~/.claude/CLAUDE.md`), which would confound every arm.
   - **Codex:** sessions use the owner's `CODEX_HOME`, never a copy of its
     credential. A copied `auth.json` that refreshes its token can invalidate
     the owner's own login.
     - The project's trust comes from `-c projects."<ws>".trust_level="trusted"`.
     - The table Codex persists from it is removed after each session, as the
       canary does.
     - `--ignore-user-config` is never passed, because it disables every hook.
   - **Neutral home:** every coder session, on both harnesses, runs with `HOME`
     set to an empty directory the bench owns, and with `GIT_CONFIG_GLOBAL` set
     to `/dev/null`. Codex keeps the owner's `CODEX_HOME` as an absolute path.
     - With the owner's home, Codex lists the owner's user skills and the
       account's connector apps in the model's prompt. `codex debug
       prompt-input` shows that the neutral home leaves only the bundled system
       skills and the workspace's project skills.
     - Every Codex invocation (fresh, resume and fork) also passes `--disable
       apps` and `-c include_apps_instructions=false`.
   - **Toolchains:** the toolchain install directories are resolved once per run
     (`script/resolve-tool-dirs`, shared with the night jobs) and recorded, then
     placed first in the session's `PATH`. No shims directory is ever on it, and
     the caches point at the prepared task environment.
   - **Tamper checks:** at every stop the bench takes the digests of `.claude/`
     and `.codex/` and scans the events. In the cheat arm a change is tampering;
     in ALL-native it marks the session `contaminated`.
10. **ALL-native is accepted by a clean-room re-gate that replays its own
    responses.**
    - The responses are harvested with `host push` and converted to the
      `--record` layout.
    - A replay miss triggers a reserved live gate, flagged
      `acceptance_host: isolated-fallback`. E5 is reported with and without
      these fallbacks.
    - The in-session verdict is kept, and the concordance between the two
      verdicts is reported.
11. **Oracle tree layout.** H and O are overlaid at their bundle-relative paths
    rather than in a separate directory, because the oracle manifests name tests
    relative to the tree root.
    - A path of H or O that would overwrite a file of the agent's tree, other
      than an upstream test it replaces by design, is an error. It is never a
      silent overwrite.
    - **Deviation**, logged at the start of the run: `protocol | arms: none | H
      and O are overlaid at their bundle-relative paths in the oracle tree
      instead of a separate directory, because the oracle manifests name tests
      relative to the tree root; a colliding path is an error.`

12. **Every clean-room gate is signed, with a simulated CI round trip and one
    CI seed per tree.**
    - Users merge under `trust: signed`, where local trust is advisory. Local
      trust is also not the same across arms: with no discriminating-power proof
      to corroborate it, an ALIGNED verdict clears under local trust and
      escalates under signed. Local gating would therefore change the AL-versus-
      ALL contrast itself.
    - `prove --ci --import-host` draws CI-issued discriminating-power requests
      that no pushed bundle holds.
    - **Seed:**
      - The clean room commits the agent's tree alone, then each arm's setup on
        top.
      - The CI run id is `bench-<tree digest>`.
      - The seed record of the final head is restored with the seed of the
        agent-only commit, so every arm draws the same CI requests on the same
        tree.
    - **Settle step:**
      1. It replays the recordings once. The local report is kept as a secondary
         readout.
      2. It clears `.spec-proof/ci-issued/` and runs `prove --ci --import-host`
         on the archived bundles. A module that exits 3 is pending.
    - **Round trips** (at most three, named after `MaxHostWaits`):
      1. A pending round returns `PAUSED` with `ci_round_pending`.
      2. The bench reserves one round, runs `host pull` through a `gh` shim no
         session can reach, and has the gate-host session answer.
      3. It runs `host push` and archives the round under `record_dir/ci/<k>/`.
    - **Replay** stays byte-identical at zero executions.
    - **Cost:** TR, AL and CO cost no extra executions. DP, ALL and PL pay one
      session plus their executor launches per round. Rounds at S_T are shared by
      the arms on the same tree.
    - **Disclosure**, logged at the start of the run: `protocol | arms: TR, AL, DP,
      CO, ALL, PL, ALL-native, cheat | The clean-room gate's CI run is simulated:
      SPEC_PROOF_CI_RUN_ID is bench-<tree digest prefix>, and the seed record of
      the head is restored with the seed of the agent tree's own commit, so every
      arm gets the same CI seed on the same tree; CI-issued artifacts reach host
      pull through a gh shim no session can reach.`
    - **Deviation from decision 10**, logged at the start of the run: `protocol |
      arms: ALL-native | The clean-room re-gate of ALL-native returns its
      CI-issued requests to ALL-native's own session, as the cheat arm does (ADR
      0007, Host execution), instead of a reserved isolated-fallback live gate;
      only misses of its harvested local responses use isolated-fallback; the
      extra executions are reported in E5.`
13. **Every Claude Code session of the sample runs one frozen, private copy of
    the CLI.**
    - **Why:** the owner's own `claude` is the native install, and it updates
      itself from interactive sessions. A sample that used it could span two CLI
      versions, and the version a session ran would not be knowable afterwards.
      The owner's installation and configuration are never touched.
    - **Pin:** `script/claude-code.pin` holds the one version. Everything that
      runs or installs the CLI follows the file, and `script/check-claude-code-pin`,
      run by `script/lint`, fails when `ClaudeCodeVersion` in
      `format/hostproto/harness/harness.go` differs from it.
    - **Copy:** `script/claude-code-install` puts the binary at
      `.tools/claude-code/<version>/bin/claude`, from the owner's native binary
      of that version when it exists and otherwise from the npm package at that
      exact version. It refuses a binary whose `--version` does not start with
      the pinned version, and records the SHA-256 of what it installed in
      `.tools/claude-code/<version>/SHA256`.
    - **Resolution:** one port, `PinnedClaude`, gives every launch site its
      binary and its version: coder sessions, gate-host sessions, CI round
      trips, construction calls and the manifest's version probe. Without the
      installed binary a session does not start, and the message names
      `script/claude-code-install`.
    - **Updater:** `DISABLE_AUTOUPDATER=1` is set in the process environment of
      each session and of each probe of the pinned binary, and nowhere else: not
      in the bench's own environment and not in any configuration file. The
      night jobs that call Claude (canaries and construction) install the copy,
      put its directory first on `PATH` and set the same variable, so the
      nightly canary validates the binary the bench runs. The pilot job keeps
      the `PATH` it was pre-registered with.
    - **Record:** the `claude_code_version` of the `system/init` event is kept
      on the session's result and in its call record (`meta.json`,
      `cli_version`). The Codex session records the `cli_version` of its
      rollout. The manifest's `cli_versions.claude` is the pin.
    - **Mismatch:** a session whose init names another version than the pin, or
      none, is `harness_misconfigured` with the reason in its problems. It is
      not archived and it is charged the executions its events show, so the run
      stops at that session and continues once the binary is right. A gate-host
      session is stopped at its init event.

## Consequences

- **Budget:** decisions 1, 2 and 7 combine into one nightly budget, counted in
  executions. Every live gate reserves its share before it starts. A window that
  closes mid-gate settles from the streamed events, and the deterministic clean
  room resumes on the next night.
- **Slots:** no step holds more than one slot. While branch k's gate runs in its
  clean room, branch k+1's coder session uses the workspace.
- **One replay engine** serves ALL-native acceptance, the cheat arm's re-gates
  and the module-alone selection.
- **Blocked:** before any Claude coder or gate-host session can start, the owner
  must create the long-lived credential with `claude setup-token`. Everything up
  to the live smoke run is built and tested against fakes without it.
