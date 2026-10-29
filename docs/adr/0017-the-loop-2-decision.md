# ADR 0017: The loop-2 decision

Status: accepted. Frozen by the tag `plan-loop-2` and an RFC 3161 timestamp of its
commit before any loop-2 session runs. The pre-registration
`experiments/loop-2/PREREG.md` is frozen later, before the calibration starts. It
sets only what "Left to the pre-registration" lists, by the rules given there; any
other change to this record needs a superseding ADR, frozen the same way before the
calibration starts.

## Context

`loop-1` ran 36 sessions of a coding agent (Claude Code, `claude-sonnet-5-5`) on six
small fixture tasks, under a gate that requires discriminating power and alignment
against a gate that requires traceability alone. The oracle found no defective final
change in either arm (0/18 each): the study could not decide its hypotheses. It also
showed three defects of the in-session product:

- **4/18 false rejections.** The treatment gate refused 4/18 oracle-clean changes,
  each time because the session ended without a verified proof set. In the records:
  - `prove` outlived the 120 s tool timeout and went to the background;
  - `host_unresponsive` came back as a fresh verdict;
  - the pending count disagreed with the ticket listing;
  - the end-of-turn hook gave up while proof waves still advanced;
  - one edited test file made every criterion's proofs stale, since descriptors
    digest whole files (ADR 0008).
- **A control gate that acted on proofs it did not require.** It ran the proof
  modules on its own and was held by findings it did not require.
- **A blind spot of discriminating power in the agent loop.** A spec witness that
  fails the change under review is a `reading_conflict`, which never blocks
  (ADR 0013). That reading holds when the reference is known correct. In the loop,
  the reference is the agent's own change, so a change that is wrong where the spec
  states a rule in prose and gives no example passes when the agent's tests agree
  with it.

The project needs a study whose every likely result is decisive: a demonstrated gain,
or a demonstration that the product brings no useful gain in the regime tested.

## Decisions

### Outcome

- **Escape.** A run whose final change its arm's gate accepts and whose oracle judges
  defective on the task criteria.
- **ARR.** The control escape risk minus the treatment escape risk, with tasks as
  strata.
- **Margin δ = 0.15.** In `loop-1` the gate cost 1.82 times as much per accepted
  change. The gain is called useful when it removes at least about one escape in
  seven changes.
- **Guard G.** The treatment gate refuses at most 15 % of its oracle-clean changes.
- **Guard Y.** The treatment's share of runs ending in an accepted oracle-clean
  change is at most 0.10 below the control's.
- **Direction and magnitude.** Direction comes from `loop-1`'s exact stratified
  permutation test (`p_less`, `p_more`, one-sided α = 0.05). Magnitude comes from
  the stratified Miettinen–Nurminen score interval [L, U] of ARR at level 0.90,
  with weights n₁n₂/(n₁+n₂).

The outcome is the first row that holds:

| # | Outcome | Rule | Reading |
|---|---|---|---|
| 1 | HARM | `p_more` ≤ 0.05 | the product makes things worse |
| 2 | REJECTION-ONLY | `p_less` ≤ 0.05, G or Y fails | fewer escapes, bought by refusing correct work |
| 3 | USEFUL GAIN | `p_less` ≤ 0.05, L ≥ δ | gain shown, and it is useful |
| 4 | GAIN BELOW USEFUL | `p_less` ≤ 0.05, U < δ | a real gain, too small for its cost |
| 5 | GAIN | `p_less` ≤ 0.05 | gain shown; whether it reaches δ stays open |
| 6 | NO USEFUL GAIN | U < δ | any gain is below what would be useful |
| 7 | INCONCLUSIVE | otherwise | |

- **Reading.**
  - Rows 3 and 5 demonstrate a gain.
  - Rows 1, 2, 4 and 6 demonstrate that the product, as tested, brings no useful
    gain.
- **Operating characteristics.** `experiments/loop-2/script/power table` gives them
  for 24 tasks and 8 runs per task and arm, under these assumptions:
  - per-task control rates follow Beta(κ = 4);
  - clean treatment changes are refused at 0.08;
  - half of the caught changes are corrected;
  - each cell is 2,000 draws with seed 20261029.

| Control → treatment | USEFUL GAIN | GAIN | GAIN BELOW USEFUL | REJECTION-ONLY | NO USEFUL GAIN | HARM | INCONCLUSIVE |
|---|---|---|---|---|---|---|---|
| 0.50 → 0.25 | 0.70 | 0.30 | 0.00 | 0.00 | 0.00 | 0.00 | 0.00 |
| 0.40 → 0.10 | 0.95 | 0.05 | 0.00 | 0.00 | 0.00 | 0.00 | 0.00 |
| 0.40 → 0.20 | 0.32 | 0.68 | 0.00 | 0.00 | 0.00 | 0.00 | 0.00 |
| 0.30 → 0.15 | 0.06 | 0.88 | 0.03 | 0.00 | 0.03 | 0.00 | 0.00 |
| 0.40 → 0.30 | 0.00 | 0.66 | 0.01 | 0.00 | 0.30 | 0.00 | 0.03 |
| 0.20 → 0.10 | 0.00 | 0.58 | 0.29 | 0.00 | 0.13 | 0.00 | 0.00 |
| 0.40 → 0.40 | 0.00 | 0.03 | 0.00 | 0.00 | 0.92 | 0.04 | 0.01 |
| 0.20 → 0.20 | 0.00 | 0.01 | 0.03 | 0.00 | 0.93 | 0.04 | 0.00 |
| 0.10 → 0.10 | 0.00 | 0.00 | 0.04 | 0.00 | 0.93 | 0.04 | 0.00 |
| 0.20 → 0.30 | 0.00 | 0.00 | 0.00 | 0.00 | 0.22 | 0.78 | 0.00 |

USEFUL GAIN needs a large effect at a high base rate: with the treatment halving the
control rate, it needs a control rate of about 0.5. Below that, a true gain reads as
GAIN.

### Secondary analyses, reported whatever the outcome

- **H2, correction.** The share of treatment runs with a finding block that are
  corrected and then accepted.
  - SUPPORTED when the exact binomial p ≤ 0.05 for a share above 0.5.
  - NOT SUPPORTED when the 95 % upper bound is below 0.5.
  - NOT TESTABLE with fewer than 10 such runs.
- **Catches and attribution.**
  - A catch is a treatment run whose tree at its first stop is oracle-defective and
    whose final change is oracle-clean and accepted, or whose defective change the
    gate refuses with a finding naming a violated task criterion.
  - Refusals for stale or pending proofs or for a cap are never catches.
  - Each catch is attributed to the finding that preceded the correction:
    `spec_witness_failed`, a failed spec example, `surviving_valid_mutant`,
    alignment, or none.
- **Mutation-only effect.** The decision rule is applied again with every catch
  attributed to `spec_witness_failed` counted as an escape. When a gain is shown and
  at least two thirds of the catches are `spec_witness_failed`, the report says that
  the gain comes mainly from the spec-witness check, and the product claim names it.
- **Instructions against the gate.** The oracle also grades every run's tree at its
  first stop, in both arms. The difference at the first stop is the effect of the
  instructions and the in-turn proofs. The difference between the first stop and the
  final change is the effect of the gate.
- **Re-adjudication.** Each escape is listed with the spec sentence it breaks. It is
  re-adjudicated by `gpt-5.6-terra` and `claude-opus-5-5`, blind to its arm, and a
  sensitivity analysis drops the disputed escapes.
- `loop-1`'s S1–S8 are kept, with a sign test over tasks as a sensitivity analysis.

### Calibration, readiness and the base-rate stop

- **C1.** 8 *hard* tasks × 2 coders (`claude-sonnet-5-5`, `claude-haiku-5-5`) ×
  2 arms × 2 repetitions: 64 runs.
- **Readiness of a coder.** All of these hold:
  - the treatment gate refuses at most 1 of that coder's oracle-clean C1 treatment
    changes, and never for stale or pending proofs alone;
  - **positive control:** the treatment gate, on that coder's model, refuses at least
    three quarters of the C1 tasks' held-out violations that the fixture's tests
    pass, each with a finding naming the violated criterion;
  - **P3 replay:** `spec_witness_failed` never fires on the 36 final trees of
    `loop-1` or on any C1 reference, and each C1 reference with its tests passes the
    treatment gate (8/8).
- **Coder selection, mechanical.**
  1. `claude-sonnet-5-5` if it is ready and has at least 5/16 control escapes.
  2. Otherwise `claude-haiku-5-5` under the same rule.
  3. Otherwise **C2**: 8 fresh *max* tasks, 64 more runs, the same readiness rules.
     - If every ready coder has 0/32 control escapes pooled over C1 and C2, the
       programme stops with **NO USEFUL GAIN** for this regime.
     - Otherwise it takes the ready coder with the most pooled control escapes,
       `claude-sonnet-5-5` on a tie.
- **No ready coder** sends the programme back to the product fixes (see "Steps and
  stops").
- **Committed before any confirmatory task is built:**
  - the selection;
  - the output of `script/power reach` at the chosen coder's control escapes, which
    states in advance whether USEFUL GAIN is reachable.
- Calibration data never enter the confirmatory analysis. The shakedown is never
  oracle-graded before the confirmatory report is published.

### Tasks and oracle

- **Shape.** A pytest project of 250–600 lines (400 or more for *max*). It has 3–6
  operations, shared helpers, 9–14 criteria with inline examples, and existing tests.
  The task asks for two new criteria, or for one criterion revised plus one new one.
- **Levers.** Declared per task and checked by `accept`: at least 3 for *hard*, all
  5 for *max*.

| Lever | Check |
|---|---|
| L1 interacting rules | a violation of another criterion fails an oracle test of a task criterion |
| L2 boundaries outside the examples | at least two boundary values stated in prose that no example uses; the oracle tests both sides |
| L3 shared helper | the reference changes a function that at least two implemented criteria reach |
| L4 revision | the fixture's tests fail on the reference |
| L5 size | at least 250 lines, 400 for *max* |

- **Construction.** `gpt-5.6-terra` writes the task, the reference, the violations,
  the refactors and the oracle, the oracle from the spec alone. `loop-1`'s A1–A9
  apply, plus:
  - **A10.** A second, independent terra oracle passes on the reference.
  - **A11.** `claude-opus-5-5`, without tools, cites the spec sentence behind each
    oracle assertion. An uncited assertion is repaired once, then the task drops.
  - **A12.** Every L2 boundary has a violation at that boundary, and the oracle
    kills it.
- **No solvability filter.** No construction check runs a coder or the gate.
- **Supply.**
  - A seeded list of 60 business-rule domains supplies the 2 shakedown tasks, the
    calibration tasks, and the 24 confirmatory tasks of the calibration's tier.
  - Pytest only; no mined pull requests.
  - Each treatment fixture carries the verified proof set of its starting state,
    published at construction by the product's executors on the chosen coder's
    model. A publication failure is a product failure, never a task drop.

### Arms and models

- **Treatment.**
  - Shallnot, discriminating power, alignment, completeness (advisory), and the
    umbrella plugin.
  - `proofs.require: [discriminating-power, alignment]`.
  - The product's agent instructions, and the fixture's proof set.
- **Control.**
  - Shallnot alone: its gate, plugin and instructions.
  - No Musthold binary, plugin or instruction.
  - The image differs from the treatment's only by the product's absence.
  - A detector, K7, flags any attempt to reach Musthold.
- **Executors** inherit the coder's model, the product's default.
- **Identical in both arms:** prompt, caps, egress and isolation, as in `loop-1`.

### Sample

- 24 tasks × 2 arms × 8 runs = 384 runs.
- Seeded interleaved order, tasks as strata, no interim look.
- A quota stop voids its attempt, which reruns in its place in the order.

### Product fixes that release the study

| Fix | Change | Accepted when |
|---|---|---|
| P1 proofs reach publication | every `prove` call returns within 90 s with its state persisted; waves advance only on answers; an abandonment is never cached as a verdict; one store query serves the count and the listing, and orphan tickets are reissued; a hook attempt counts only when no ticket was answered since the last block | property tests per cause; the four refused `loop-1` trees, resumed, publish a verified set; a 14-criterion fake-host run publishes; the shakedown has no stale or pending refusal in 6 sessions on *max* tasks, with `prove-all` p90 ≤ 30 min; the hook fix ships in a pinned Shallnot release |
| P5 staleness by unit (amends ADR 0008) | descriptors digest the bound test functions, the footprint functions with their static closure, the module-level statements those read, and the fixtures, `conftest.py` and test helpers they reach | golden tests: an edit outside a proof's units leaves it fresh; an edit inside its footprint, or to a fixture, `conftest.py` or helper it reaches, makes it stale; Shallnot's staleness check ships in the same release |
| P3 a failed spec witness escalates (amends ADR 0013) | the scope judges also judge witnesses in reading conflict; a witness that fails the change in 3/3 executions and that every judge holds fixed by the spec raises `spec_witness_failed` with the clause, the input and the expected output, and fails the gate under `escalation: error` | fake-host: a change wrong at a prose boundary with silent tests fails with the finding, the correct change passes, a witness judged not fixed never escalates; and the P3 replay of the readiness rules |
| P2 an unrequired proof never acts on the gate (Shallnot) | a present, failing, unrequired proof warns and exits 0 | golden tests; the control arm does not load the product, so P2 does not gate the study |

The harness gains:

- the lever checks, A10–A12, fixture publication, the control image, K7, and
  first-stop grading;
- catch attribution, the MN interval and the decision table, tested on
  hand-computed cases;
- a `freeze-check` that asserts the pre-registration tag sits on the commit adding
  `PREREG.md`.

### Steps and stops

| Step | Exit |
|---|---|
| 0. Declare `loop-1`'s §13.1 form deviation (the prereg tag sits on the commit adding `freeze/SHA256SUMS`) with its timestamp | entry published |
| 1. P1, P3, P5, P2 and the harness | acceptance criteria and the full local gate green |
| 2. Shakedown: 2 *max* tasks, 6 sessions | P1's shakedown criterion holds; P3 fires on a seeded wrong change |
| 3. Freeze `prereg-loop-2` with its timestamp | `freeze-check` passes |
| 4. C1, then C2 if called for; commit the selection and the reach report | a coder is chosen, or the base-rate stop applies |
| 5. Build the 24 tasks, publish their fixtures, freeze the inputs, run the canary | 24 accepted tasks, every fixture published |
| 6. The 384 runs | done, or the cap is reached |
| 7. Oracle, first-stop grading, re-adjudication, detectors, analysis, report lint | published as is |

**Programme stops.**

- **Loop budget.** Three returns to step 1 in all, whatever the cause: a failed
  shakedown, no ready coder, or a fixture that cannot publish. At the fourth, the
  programme publishes "the in-session gate cannot finish its proofs at this task
  size", which is a demonstration that the product, as it is, brings no useful gain
  in this regime.
- **Base rate.** The C2 rule: NO USEFUL GAIN for this regime.
- **Calendar.** 45 days for step 6, 12 weeks from the tag `plan-loop-2` for the
  programme.
  - At the step 6 cap with at least 20 tasks having at least 6 counted runs per arm,
    the analysis runs on what was counted.
  - Below that, the result is INCONCLUSIVE, with its reason.
  - At the programme cap before step 6, the programme publishes where it stopped and
    why.
- **Money.** Any real spend stops everything.

### Left to the pre-registration

| Item | Rule that sets it |
|---|---|
| domain list and seed | 60 business-rule domains, drawn before step 2 |
| prompts | `loop-1`'s, plus the task's revision wording |
| caps | wall max(7,200 s, 2 × the C1 treatment p95 of the chosen coder), 1,500 messages, `--max-budget-usd` as in `loop-1` |
| pins | Claude Code, Shallnot and the product at the commit tagged `loop-2-inputs` |
| pace | at most 12 runs a day, one session container at a time, plus at most one Codex or oracle job |
| detectors | `loop-1`'s K1–K6, plus K7 |
| run order | seeded interleaving of task × arm × repetition |

## Consequences

- **Every likely result answers the product question.** Under the stated assumptions,
  INCONCLUSIVE stays at or below 0.03 in every simulated scenario. A shown gain is
  reported with its attribution; a negative result names its regime: fully specified
  business-rule pytest tasks of 250–600 lines, on the chosen coder.
- **The product claim follows the result.** A gain that comes mainly from the
  spec-witness check is published as such. A negative result is published as such,
  and the project's claims are narrowed to what the evidence holds.
- **P1, P3 and P5 change the product** whatever the outcome: P3 removes a blind spot,
  and P1 and P5 remove the false rejections `loop-1` measured.
- **Cost on the shared subscription.**
  - 6 shakedown sessions, 64 (or 128) calibration runs and 384 confirmatory runs.
  - About 40 executor-only containers and about 400 terra calls.
  - About $6 notional per Sonnet run, about a third of that per Haiku run.
  - At the paced rate, the confirmatory phase takes about 32 days.
- **Limits stated in advance.** The tasks are synthetic, and the oracle is
  model-written and checked by a second oracle and a citation pass. The arms differ
  by the whole product, not by the requirement alone. The result covers one
  ecosystem and one coder.
