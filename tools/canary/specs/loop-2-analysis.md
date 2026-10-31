# loop-2 study tooling: analysis, calibration and report lint

The `analyze`, `calibrate` and `report-lint` verbs of the loop-2 study
(`docs/adr/0017-the-loop-2-decision.md`). `analyze` reads the records of the run
phase and writes `results/summary.json` and `results/REPORT.md`; `calibrate` reads
the calibration results, selects the coder and records the reach of the
confirmatory design; `report-lint` checks the rendered report. They read files and
run `experiments/loop-2/script/power`; they make no network or model call.

The rules that loop-2 keeps from loop-1 are loop-1's code and requirements: the
exact stratified permutation test (SP-2595), the Clopper–Pearson interval
(SP-2597), the exact binomial and sign tests (SP-2599, SP-2600), the facts of a
counted run (SP-2587 to SP-2594), the run states of the run phase (SP-2580,
SP-2582), the deviation timestamps (SP-2584 to SP-2586), S1 to S8 (SP-2600,
SP-2601) and the rules of the report lint (SP-2606 to SP-2609).

## Record inputs

A counted run's records directory, `runs/<run>-<attempt>/records/` of the work
directory, holds every file loop-1's facts read (`run.json`, `stream.jsonl`,
`stream-times.jsonl`, `hooks.jsonl`, `stops/<n>/report.json`, `gate-after.json`,
`oracle.json`, `heldout.json`, `detectors.json`), plus:

- `first-stop-oracle.json`: the oracle's grade of the project's tree at the first
  end-of-turn call, in the format of `oracle.json`. It is absent when the session
  made no end-of-turn call.
- `adjudication.json`, for every escape: the spec sentence the escape breaks and
  each judge's verdict, blind to the arm.

  ```json
  {
    "criterion": "FARE-7",
    "sentence": "A rider under 12 travels free on weekends.",
    "judges": [
      {"judge": "gpt-5.6-terra", "defective": true, "reason": "..."},
      {"judge": "claude-opus-5-5", "defective": false, "reason": "..."}
    ]
  }
  ```

The calibration input `calibration.json` holds one entry per coder and
calibration phase (`C1`, `C2`):

```json
{
  "coders": [
    {
      "coder": "claude-sonnet-5-5",
      "phase": "C1",
      "runs": [
        {"run": "fare-control-1", "task": "fare", "arm": "control",
         "accepted": true, "defective": true, "refusal": "none"}
      ],
      "positiveControl": [
        {"task": "fare", "violation": "FARE-7/2", "criterion": "FARE-7",
         "fixtureTestsPass": true, "refused": true, "namesCriterion": true}
      ],
      "replay": {
        "loop1Trees": 36, "loop1Firings": 0,
        "references": [{"task": "fare", "firings": 0, "passes": true}]
      }
    }
  ]
}
```

A run's `refusal` is its refusal cause (SP-2801): `none`, `cap`, `finding`,
`pending` or `traceability`.

`analyze` reads the order, the tags' commits and the tasks from the inputs freeze
`experiments/loop-2/freeze/inputs.json`, in the format of loop-1's inputs freeze,
and the times of the tags from the RFC 3161 tokens beside it: `plan-commit.tsr`
with `plan-commit.txt`, `prereg-commit.tsr` and `inputs-commit.tsr`.

- **SP-2800~1**: WHEN a counted run's loop-2 record is read THE SYSTEM SHALL compute
  loop-1's facts from its records, read `first-stop-oracle.json` as the grade of
  the tree at the first end-of-turn call and record no grade when the file is
  absent, and read `adjudication.json` for an escape, refusing an escape without
  it.
- **SP-2801~1**: WHEN the refusal cause of a run's final change is derived THE
  SYSTEM SHALL call it none when the change is accepted, pending when the only
  blocking proof findings of `gate-after.json` are `stale_proof` or pending
  reasons, cap when the session otherwise ended at a cap, finding when a blocking
  finding comes from a proof's verdict, and traceability otherwise, and SHALL keep
  the blocking findings with their code, criterion and module.

## Magnitude: the Miettinen–Nurminen score

- **SP-2802~1**: WHEN the restricted maximum-likelihood rates of a task are computed
  under a risk difference δ, control minus treatment, THE SYSTEM SHALL take the
  closed-form root of the Miettinen–Nurminen cubic with θ = n₂/n₁, clamp the
  control rate to [0, 1], and set the treatment rate to the control rate minus δ
  clamped to [0, 1].
- **SP-2803~1**: WHEN the stratified score at δ is computed THE SYSTEM SHALL sum,
  over the tasks whose two arms each hold a counted run, the weight n₁n₂/(n₁+n₂)
  times the control rate minus the treatment rate minus δ, divide it by the square
  root of the sum of each squared weight times the task's restricted variance
  with the factor N/(N−1), give +∞, −∞ or 0 by the sign of the numerator when that
  variance is zero, and agree with `experiments/loop-2/script/power` within a
  relative 1e-8.
- **SP-2804~1**: WHEN the 0.90 score interval [L, U] is computed THE SYSTEM SHALL
  find L where the score falls to z = 1.6448536269514722 and U where it falls to
  −z, by bisection on [−1, 1], with any run counts per task.

## The H1 decision

- **SP-2805~1**: WHEN guard G is evaluated THE SYSTEM SHALL hold it when the
  treatment gate refuses at most 3/20 of the treatment's oracle-clean changes,
  and hold it when the treatment has no oracle-clean change.
- **SP-2806~1**: WHEN guard Y is evaluated THE SYSTEM SHALL hold it when the
  control's share of counted runs ending in an accepted oracle-clean change minus
  the treatment's share is at most 1/10, each share over its own arm's counted
  runs.
- **SP-2807~1**: WHEN the outcome is decided THE SYSTEM SHALL take the first row
  that holds, in this order, at one-sided α = 0.05 and δ = 0.15: HARM when
  `p_more` ≤ α; REJECTION-ONLY when `p_less` ≤ α and G or Y fails; USEFUL GAIN
  when `p_less` ≤ α and L ≥ δ; GAIN BELOW USEFUL when `p_less` ≤ α and U < δ; GAIN
  when `p_less` ≤ α; NO USEFUL GAIN when U < δ; INCONCLUSIVE otherwise; deciding
  L ≥ δ as score(δ) ≥ z and U < δ as score(δ) < −z, as
  `experiments/loop-2/script/power` does.
- **SP-2808~1**: WHEN an outcome is read THE SYSTEM SHALL read USEFUL GAIN and GAIN
  as a shown gain, HARM, REJECTION-ONLY, GAIN BELOW USEFUL and NO USEFUL GAIN as a
  shown absence of useful gain, and INCONCLUSIVE as undecided, each with the
  reading of ADR 0017's table.
- **SP-2809~1**: WHEN the primary analysis runs THE SYSTEM SHALL take as strata the
  tasks with at least two counted runs in each arm, count an escape when the
  final change is accepted and oracle-defective, compute `p_less` and `p_more` by
  loop-1's exact stratified permutation test, the score at δ and [L, U] on the
  same strata, the guards on the runs of those tasks, and report each arm's
  escapes with their exact interval, the escapes per arm of each task, and
  whether [L, U] reads δ as score(δ) does, which can fail where small unequal
  arms make the score rise locally.

## Steps and stops

- **SP-2810~1**: WHEN the end of the loop-2 run phase is judged THE SYSTEM SHALL
  apply loop-1's run states and end the phase when every run of the order is
  counted or blocked, or 45 days after the earliest start of any attempt.
- **SP-2811~1**: WHEN the run phase ended at its calendar cap THE SYSTEM SHALL run
  the analysis on the counted runs when at least 20 tasks hold at least 6 counted
  runs in each arm, and otherwise decide INCONCLUSIVE with a reason that names
  how many tasks do; WHEN it ended without the cap THE SYSTEM SHALL run the
  analysis.

## Secondary analyses

- **SP-2812~1**: WHEN H2 is decided THE SYSTEM SHALL take the treatment runs with a
  finding block and the share whose last finding block is corrected by loop-1's
  rule, and decide NOT TESTABLE with fewer than 10 such runs, SUPPORTED when the
  one-sided exact binomial p of a share above 0.5 is at most 0.05, NOT SUPPORTED
  when the upper bound of the exact 95 % interval is below 0.5, and INCONCLUSIVE
  otherwise.
- **SP-2813~1**: WHEN the catches are counted THE SYSTEM SHALL call a treatment run
  a catch when its tree at the first stop is oracle-defective and its final
  change is oracle-clean and accepted, or when its final change is
  oracle-defective and refused with the refusal cause finding by a blocking
  finding that names a criterion the oracle finds violated; a refusal for stale
  or pending proofs or for a cap is never a catch, and a run without a
  first-stop grade is a catch only by its refusal.
- **SP-2814~1**: WHEN a catch is attributed THE SYSTEM SHALL read the findings of
  the run's last finding block for a corrected catch and the refusing findings
  that name a violated criterion for a refused catch, and attribute it to the
  first class those findings hold in this order: `spec_witness_failed`,
  `example_failed` (a failed spec example), `surviving_valid_mutant`, alignment (a
  finding of the module `musthold-alignment`), other; and to none when they hold
  no finding.
- **SP-2815~1**: WHEN the outcome shows a gain and at least two thirds of at least
  one catch are attributed to `spec_witness_failed` THE SYSTEM SHALL state that
  the gain comes mainly from the spec-witness check, and name the check in the
  product claim.
- **SP-2816~1**: WHEN the mutation-only effect is computed THE SYSTEM SHALL count
  every catch attributed to `spec_witness_failed` as a run whose final change is
  accepted and oracle-defective, and decide the outcome again by SP-2805 to
  SP-2809 on those runs, guards included.
- **SP-2817~1**: WHEN the instructions are set against the gate THE SYSTEM SHALL
  give per arm the runs with a first-stop grade, their first-stop defect share
  and their escape share with exact intervals, the runs without a grade, the
  stratified permutation test of the first-stop defect, the effect of the
  instructions as the control's first-stop defect share minus the treatment's,
  and the effect of the gate as the treatment's first-stop defect share minus its
  escape share, less the same difference in the control.
- **SP-2818~1**: WHEN the escapes are re-adjudicated THE SYSTEM SHALL list each
  escape with its run, arm, criterion, spec sentence and each judge's verdict,
  refuse an adjudication that does not hold exactly one verdict of
  `gpt-5.6-terra` and one of `claude-opus-5-5`, and call an escape disputed when
  either judge does not hold it defective.
- **SP-2819~1**: WHEN the re-adjudication sensitivity runs THE SYSTEM SHALL decide
  the outcome again by SP-2805 to SP-2809 without the runs of the disputed
  escapes.
- **SP-2820~1**: WHEN loop-1's secondary endpoints are kept THE SYSTEM SHALL
  compute S1 to S8 and the sign tests over tasks on the per-task difference of
  escape and defect shares and on the per-task majority of corrected runs, by
  loop-1's rules on the counted runs.

## Calibration

- **SP-2821~1**: WHEN a coder's readiness is judged on a calibration phase THE
  SYSTEM SHALL call it ready when the treatment gate refuses at most 1 of its
  oracle-clean treatment changes and none with the refusal cause pending; when
  at least 3/4 of the held-out violations that the fixture's tests pass, and at
  least one, are refused with a finding naming the violated criterion; and when
  `spec_witness_failed` fires on none of the 36 final trees of loop-1 and on none
  of the phase's references, and each of its 8 references passes the treatment
  gate.
- **SP-2822~1**: WHEN the coder is selected after C1 THE SYSTEM SHALL return to the
  product fixes when no coder is ready, select `claude-sonnet-5-5` when it is
  ready with at least 5 control escapes, else `claude-haiku-5-5` under the same
  rule, and otherwise call for C2.
- **SP-2823~1**: WHEN the coder is selected after C2 THE SYSTEM SHALL judge
  readiness on C2's runs, return to the product fixes when no coder is ready,
  stop with NO USEFUL GAIN for the regime when every ready coder has no control
  escape pooled over C1 and C2, and otherwise select the ready coder with the
  most pooled control escapes, `claude-sonnet-5-5` on a tie.
- **SP-2824~1**: WHEN a coder is selected THE SYSTEM SHALL run
  `experiments/loop-2/script/power reach` with the coder's pooled control escapes
  and control runs and record its output verbatim beside the selection.

## Results

- **SP-2825~1**: WHEN `analyze` has run THE SYSTEM SHALL write
  `results/summary.json` holding every run of the order with its state and
  attempts, the counted runs' records, the outcome with its evidence, the
  mutation-only outcome, the catches, H2, the instructions against the gate, the
  re-adjudication, S1 to S8, every fraction the report shows, and the SHA-256 of
  `inputs.json` and of every record read.
- **SP-2826~1**: WHEN `REPORT.md` is rendered THE SYSTEM SHALL render it from
  `summary.json` with the provenance, the conclusion filled from the templates of
  this spec, the outcome with its row and reading, every endpoint of SP-2809 and
  SP-2812 to SP-2820, every run of the order, the limits stated in ADR 0017
  verbatim, and an empty exploratory section.
- **SP-2827~1**: WHEN `REPORT.md` is linted THE SYSTEM SHALL apply loop-1's rules
  with this spec's conclusion templates, and fail when the report does not state
  the outcome and the reading of `summary.json`, or states that the gain comes
  mainly from the spec-witness check when `summary.json` does not.
- **SP-2828~1**: WHEN `analyze` starts before the run phase ends, or with a
  protocol or token-exposed deviation not timestamped, THE SYSTEM SHALL refuse
  and write nothing.

### Conclusion templates

- Outcome: "On <T> tasks (<n_c> control and <n_t> treatment runs, one coder, pytest), an accepted change was oracle-defective in <k_t>/<n_t> treatment runs against <k_c>/<n_c> control runs (exact stratified test, one-sided p_less = <p_less>, p_more = <p_more>; 0.90 score interval of the difference [<low>, <high>]); the outcome is <outcome>: <reading>."
- Inconclusive at the calendar cap: "The study cannot decide the outcome (<reason>). This is not evidence of no difference."
- Spec-witness gain: "The gain comes mainly from the spec-witness check: <s>/<c> catches were attributed to spec_witness_failed."
- H2: "After a finding block, the agent corrected its change and the gate then accepted it in <c>/<b> treatment runs (exact 95 % interval [<lo>, <hi>]); <decision>."
