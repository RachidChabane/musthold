---
name: prove-all
description: Prove a change with every spec-proof module that is installed, in the order discriminating power, alignment, completeness, answering each module's requests with its blind executors. Use before finishing a change to code, tests or specs, and when Shallnot reports awaiting_host, awaiting_proof or awaiting_host_judgment.
allowed-tools: Bash(spec-proof-dp doctor), Bash(spec-proof-dp prove *), Bash(spec-proof-dp request list), Bash(spec-proof-alignment doctor), Bash(spec-proof-alignment prove *), Bash(spec-proof-alignment request list), Bash(spec-proof-completeness doctor), Bash(spec-proof-completeness prove *), Bash(spec-proof-completeness request list), Bash(spec-proof-s1 drain *)
---

# Prove with every module

This skill holds no logic of its own. Each module runs its own loop and names its own steps in its fulfil skill; this skill only orders the modules and interleaves their waves. Never answer a request yourself and never read what a request asks.

Modules, in this order, each with the skill that holds its loop:

- `spec-proof-dp`, whose loop is the skill `spec-proof-dp-fulfil`.
- `spec-proof-alignment`, whose loop is the skill `spec-proof-alignment-fulfil`.
- `spec-proof-completeness`, whose loop is the skill `spec-proof-completeness-fulfil`.

1. Run `spec-proof-dp doctor`, `spec-proof-alignment doctor` and `spec-proof-completeness doctor`. A module whose binary is not on `PATH` is missing: name every missing module to the user and go on with the others; a missing module never counts as proved. If a module that is present fails its `doctor`, report it and leave that module out. If no module is present, stop.
2. For each present module, in the order above, run `<binary> prove --changed` once.
3. While any module exited with code 3, run one wave for each module that exited with code 3, in the order above, before any module starts its next wave: a wave is the steps its fulfil skill gives for answering the tickets, which is the relay when `spec-proof-s1` is on `PATH` and an endpoint is configured, then one foreground blind executor per ticket, then `<binary> prove --resume`. Alignment's judge tickets do not wait for the discriminating-power proof, so no module's wave waits for another's.
4. A module whose `prove` exits with code 0 or 1 is done: report its verdict lines. For exit code 2, or a `host_unresponsive` finding, report it to the user and stop that module. Do not loop.
5. Report, per module, its last exit code and verdict lines, and the modules that were missing.
