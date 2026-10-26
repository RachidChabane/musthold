---
name: prove-all
description: Prove a change with every musthold module that is installed, in the order discriminating power, alignment, completeness, answering each module's requests as its fulfil skill gives. Use before finishing a change to code, tests or specs, and when Shallnot reports awaiting_host, awaiting_proof or awaiting_host_judgment.
---

# Prove with every module

This skill holds no logic of its own. Each module runs its own loop and names its own steps in its fulfil skill; this skill only orders the modules and interleaves their waves.

Modules, in this order, each with the skill that holds its loop:

- `musthold-dp`, whose loop is the skill `musthold-dp-fulfil`.
- `musthold-alignment`, whose loop is the skill `musthold-alignment-fulfil`.
- `musthold-completeness`, whose loop is the skill `musthold-completeness-fulfil`.

1. Run `musthold-dp doctor`, `musthold-alignment doctor` and `musthold-completeness doctor`. A module whose binary is not on `PATH` is missing: name every missing module to the user and go on with the others; a missing module never counts as proved. If a module that is present fails its `doctor`, report it and leave that module out. If no module is present, stop.
2. For each present module, in the order above, run `<binary> prove --changed` once.
3. While any module exited with code 3, run one wave for each module that exited with code 3, in the order above, before any module starts its next wave: a wave is the steps its fulfil skill gives for answering the tickets, which is the relay when `musthold-s1` is on `PATH` and an endpoint is configured, then the answer to each ticket, as that skill gives it, then `<binary> prove --resume`. Alignment's judge tickets do not wait for the discriminating-power proof, so no module's wave waits for another's.
4. A module whose `prove` exits with code 0 or 1 is done: report its verdict lines. For exit code 2, or a `host_unresponsive` finding, report it to the user and stop that module. Do not loop.
5. When every present module is done, publish the local proofs once, where Shallnot's gate reads them: run `musthold-verify --accept-local .musthold/cache --out .musthold/verified`. When `musthold-verify` is not on `PATH`, tell the user: the proofs stay unpublished and the gate reports them stale. Report a rejection by the verifier with its message.
6. Report, per module, its last exit code and verdict lines, the modules that were missing and whether the proofs were published.
