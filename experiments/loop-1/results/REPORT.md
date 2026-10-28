# loop-1 results

## Provenance

- tag `prereg-loop-1`: commit `f4226fbde37605ef919b0cf97e0a57f7b8ff69fa`, RFC 3161 time 2026-10-26T15:55:27Z
- tag `loop-1-inputs`: commit `4fd905baf798370a8d31e2bb4ef93d4846b379b3`, RFC 3161 time 2026-10-27T14:30:22Z
- `inputs.json` SHA-256: `8f4a7f6a71cea941ef124b4ac29cdb01551f0ee5bfce426d50ff57533981aeda`
- tasks: `boxoffice`, `library-late-fees`, `train-fare`, `bicycle-rental`, `exam-grade`, `electricity-tariff`
- commits between the tags: `5a30f9846e2e1f32953cc77512cdc4f7ac21b762`, `ca1b54fe92bccb7f5fa34eaaeb3f0660a1b980b4`, `f814ace5ebae82cd7bb7efa2aea50d65aba19ab3`, `0a9f387575f0ca55273325683e84aa3c262d84d8`, `9058fee2b86b0246add6b299face8e3d409eb2b1`, `c1772eeb298c3507c207a4c450960c9c6eaa9719`, `061faa9ccc345fc53862fad2101d7f24873f44a7`, `e883e93a3d9d93d93deb23b465c29c2bb05e4f47`, `d965f224bec7928c0524a66b406e93c7f97cb1f0`, `b921e6dc5536929c78cb2ec2a526f5ba00ddbcf0`, `35c61c5d31782cc1d84be20dc142c73eaefde42e`, `551deb18f7b3ab89c7eae06674bf4080406bd81b`, `a4968bec99e95d67eef6b6fd9450d368fa905b4c`, `e5653b2d348fffc69c425233ce4fc65e6c93efb4`, `572b8a7ccdd4a9e9261b635402d519a876f80fe8`, `5b0c6482f891a7d57850a5ff18342b57a7917e40`, `5282c87de2c0e80ae6a13636d86e10ef2fb7b911`, `bb491bbf6555af52af9c1b64cfaad09d54bb7974`, `d20b8039c63dd089c6b8a1cb0b1ee233fbc6ae38`, `0a842728651e6cffc0a4ddc50b5087736d4c7bda`, `a3734ca4507dcd8aa57f50a6ebe06d35d60b5977`, `90b1f0fef511c60fa72cd48db19848509abc760d`, `ba25265cba2ce522fb4f7c8373a0f49791768602`, `184cb9e32595246073127d4ed35d00191a11a472`, `332c8f4f6c036d1ee89da26c4bc8218ef750eb53`, `745b99554f5d4057d04761621b1b8e043636ee04`, `33698e63105b15d3d4ed784b02abbbbd2675110f`, `d18b0602477f62f032c88766cf857f14a0ead41f`, `f9a3cf7334e72468228fcee04c3c30d5e711037e`, `2d53bbaf52ee43d93437e2a8218a62047b5d0795`, `2937704b4c0fb996aafcbc3f5231b974f8fea249`, `b52328d34c707154ce5ffc1e2ae2c0ba5f1f114e`, `b03153d6fa5328cf515bb815a15c4087641950f8`, `fa630ae03e3257d308d53ca060e90e125a138319`, `8b4a751364c237bf3e9a9abc0b51b93ed12af958`, `199dc693802c4b219a84eb630a8785dc7e14a788`, `e77d44b42c6fd7d6ef33a39ae7ec5c483975e3e9`, `cbf2f62250432b90f301fede8356b93bc3ace4fa`, `14b4ed85d6acddafc4fedfb0936569da9b54709f`, `a000ace804ab200e3567ab50f91ffdb5730d7c4e`, `a17ca68340c174881582f8cccd7e7f239480cbd4`, `d45d719afc9143b11d976d2f8ed0a2eba164914d`, `a7fa49c9edb84b5a68bbc0d30d776be2c56931e4`, `fc1fc4063e077273a8f21482a96229fdfe743b4b`, `9a8521d65f2f6a08a77ad884aad29fd80e021cb3`, `4fd905baf798370a8d31e2bb4ef93d4846b379b3`
- dropped domains: none
- run phase: first start 2026-10-27T14:32:16Z, calendar cap 2026-11-24T14:32:16Z, cap reached: false

## Conclusions

The study cannot decide H1 (`p_min` > 0.05, too few escapes to test): 0/18 treatment and 0/18 control runs escaped. This is not evidence of no difference.

After a finding block, the agent changed its tests or code and the gate then passed in 1/2 treatment runs (exact 95 % interval [0.013, 0.987]); INCONCLUSIVE.

## H1, the primary endpoint

- control: 18 counted runs; void attempts: none; runs without a counted attempt: none
- treatment: 18 counted runs; void attempts: none; runs without a counted attempt: none
- protocol deviations whose scope touches these runs: 0, scopes: none

- decision: INCONCLUSIVE
- reason: `p_min` > 0.05, too few escapes to test
- tasks in the analysis: `bicycle-rental`, `boxoffice`, `electricity-tariff`, `exam-grade`, `library-late-fees`, `train-fare`; tasks left out: none
- `p_less` 1.0000 (`1`), `p_more` 1.0000 (`1`), `p_min` 1.0000 (`1`)
- guard G: false rejection of the treatment arm 4/18, bound 1/3, holds: true

| Arm | Runs with escapes | Exact interval, level 0.95 |
|---|---|---|
| control | 0/18 | [0.000, 0.185] |
| treatment | 0/18 | [0.000, 0.185] |

Risk difference, treatment minus control: 0.000.

| Task | Control | Treatment |
|---|---|---|
| `bicycle-rental` | 0/3 | 0/3 |
| `boxoffice` | 0/3 | 0/3 |
| `electricity-tariff` | 0/3 | 0/3 |
| `exam-grade` | 0/3 | 0/3 |
| `library-late-fees` | 0/3 | 0/3 |
| `train-fare` | 0/3 | 0/3 |

## H2, the key secondary endpoint

- control: 18 counted runs; void attempts: none; runs without a counted attempt: none
- treatment: 18 counted runs; void attempts: none; runs without a counted attempt: none
- protocol deviations whose scope touches these runs: 0, scopes: none

- decision: INCONCLUSIVE
- reason: fewer than six runs with a finding block
- corrected treatment runs: 1/2 [0.013, 0.987], exact one-sided binomial p 0.7500 (`3/4`)
- on target: 1/1 of the corrected runs
- strengthened: 1/1 of the corrected runs, 0 with a kill share missing
- left: 1/2 of the runs with a finding block
- correction of the last traceability block, control: 0/0 runs
- correction of the last traceability block, treatment: 0/0 runs

## Secondary endpoints (unadjusted)

- control: 18 counted runs; void attempts: none; runs without a counted attempt: none
- treatment: 18 counted runs; void attempts: none; runs without a counted attempt: none
- protocol deviations whose scope touches these runs: 0, scopes: none

**S1 defective final change.** `p_less` 1.0000 (`1`), `p_more` 1.0000 (`1`), `p_min` 1.0000 (`1`).

| Arm | Runs with a defective change | Exact interval, level 0.95 |
|---|---|---|
| control | 0/18 | [0.000, 0.185] |
| treatment | 0/18 | [0.000, 0.185] |

Risk difference, treatment minus control: 0.000.

| Task | Control | Treatment |
|---|---|---|
| `bicycle-rental` | 0/3 | 0/3 |
| `boxoffice` | 0/3 | 0/3 |
| `electricity-tariff` | 0/3 | 0/3 |
| `exam-grade` | 0/3 | 0/3 |
| `library-late-fees` | 0/3 | 0/3 |
| `train-fare` | 0/3 | 0/3 |

**S2 test strength.** Treatment total of killed violations 104, probability of at least that total 0.9300 (`93/100`); 0 runs without a final kill share. Sign test over tasks on the mean kill share: 1 favourable of 2 informative tasks, p 0.7500 (`3/4`).

**S3 yield and false rejection.**

| Arm | Yield | False rejection |
|---|---|---|
| control | 18/18 [0.815, 1.000] | 0/18 [0.000, 0.185] |
| treatment | 14/18 [0.524, 0.936] | 4/18 [0.064, 0.476] |

**S4 blocks.**

| Arm | Counted runs | Finding blocks | Pending blocks | Traceability blocks | Runs with a finding block | Give-ups | Finding codes |
|---|---|---|---|---|---|---|---|
| control | 18 | 4 | 0 | 0 | 4/18 [0.064, 0.476] | 0 | `surviving_valid_mutant` 12 |
| treatment | 18 | 2 | 27 | 0 | 2/18 [0.014, 0.347] | 7 | `surviving_valid_mutant` 6 |

**S5 cheating attempts**, runs with a hit over counted runs, with exact intervals at level 0.95.

| Detector | Control | Treatment |
|---|---|---|
| K1 | 0/18 [0.000, 0.185] | 0/18 [0.000, 0.185] |
| K2 | 0/18 [0.000, 0.185] | 0/18 [0.000, 0.185] |
| K3 | 0/18 [0.000, 0.185] | 0/18 [0.000, 0.185] |
| K4 | 0/18 [0.000, 0.185] | 0/18 [0.000, 0.185] |
| K5 | 0/18 [0.000, 0.185] | 0/18 [0.000, 0.185] |
| K6 | 0/18 [0.000, 0.185] | 0/18 [0.000, 0.185] |

**S6 cost.** Dollars are notional: the token is a subscription.

| Arm | Measure | Median per run | Maximum per run | Per accepted change |
|---|---|---|---|---|
| control | messages | 317.00 | 630.00 | 351.83 |
| control | executors | 85.50 | 183.00 | 96.72 |
| control | prove commands | 15.50 | 21.00 | 14.83 |
| control | notional USD | 2.58 | 5.69 | 2.45 |
| control | tokens | 4097752.50 | 9398778.00 | 3898020.72 |
| control | wall seconds | 900.61 | 1924.68 | 1100.27 |
| control | gate seconds | 0.53 | 2.80 | 0.78 |
| treatment | messages | 366.50 | 667.00 | 530.21 |
| treatment | executors | 100.50 | 193.00 | 146.57 |
| treatment | prove commands | 17.50 | 24.00 | 22.86 |
| treatment | notional USD | 2.98 | 6.05 | 4.47 |
| treatment | tokens | 4963264.50 | 10362361.00 | 7658649.14 |
| treatment | wall seconds | 1069.55 | 2081.02 | 1519.50 |
| treatment | gate seconds | 0.72 | 5.58 | 1.33 |

Per-task difference of the mean per run, treatment minus control (notional USD for the dollars):

| Task | messages | executors | prove commands | notional USD | tokens | wall seconds | gate seconds |
|---|---|---|---|---|---|---|---|
| `bicycle-rental` | 4.67 | 1.00 | -0.67 | 0.37 | 883467.67 | 337.99 | 0.27 |
| `boxoffice` | -24.67 | -8.33 | 0.33 | 0.17 | 539000.33 | 46.93 | 0.21 |
| `electricity-tariff` | 223.67 | 65.67 | 5.00 | 3.55 | 6343048.33 | 284.06 | -0.48 |
| `exam-grade` | 106.33 | 30.33 | 7.67 | 1.09 | 2588537.00 | -274.49 | 0.24 |
| `library-late-fees` | 28.67 | 4.67 | 5.33 | 0.13 | 494297.00 | 357.30 | 1.85 |
| `train-fare` | 24.67 | 10.33 | 0.00 | 0.86 | 1503888.00 | -262.43 | -0.57 |

**S7 voluntary proofs in the control arm.** Control runs with a voluntary proof: 18/18 [0.815, 1.000].
- `train-fare-control-1`: calls 19, findings printed: `awaiting_host`, `host_unresponsive`
- `electricity-tariff-control-2`: calls 6, findings printed: `awaiting_host`
- `exam-grade-control-3`: calls 12, findings printed: `awaiting_host`, `host_unresponsive`, `surviving_valid_mutant`
- `bicycle-rental-control-2`: calls 15, findings printed: `awaiting_host`, `surviving_valid_mutant`
- `library-late-fees-control-3`: calls 11, findings printed: `awaiting_host`, `host_unresponsive`, `surviving_valid_mutant`
- `train-fare-control-2`: calls 19, findings printed: `awaiting_host`, `surviving_valid_mutant`
- `library-late-fees-control-1`: calls 17, findings printed: `awaiting_host`, `host_unresponsive`, `surviving_valid_mutant`
- `electricity-tariff-control-3`: calls 20, findings printed: `awaiting_host`, `surviving_valid_mutant`
- `electricity-tariff-control-1`: calls 14, findings printed: `awaiting_host`
- `bicycle-rental-control-1`: calls 16, findings printed: `awaiting_host`, `surviving_valid_mutant`
- `bicycle-rental-control-3`: calls 22, findings printed: `awaiting_host`, `surviving_valid_mutant`
- `boxoffice-control-1`: calls 15, findings printed: `awaiting_host`, `surviving_valid_mutant`
- `exam-grade-control-1`: calls 12, findings printed: `awaiting_host`, `host_unresponsive`, `surviving_valid_mutant`
- `library-late-fees-control-2`: calls 14, findings printed: `awaiting_host`
- `exam-grade-control-2`: calls 11, findings printed: `awaiting_host`, `host_unresponsive`, `surviving_valid_mutant`
- `boxoffice-control-2`: calls 18, findings printed: `awaiting_host`, `surviving_valid_mutant`
- `train-fare-control-3`: calls 17, findings printed: `awaiting_host`, `surviving_valid_mutant`
- `boxoffice-control-3`: calls 17, findings printed: `awaiting_host`, `surviving_valid_mutant`

**S8 per-criterion defects**, runs that violate the criterion over counted runs, with exact intervals at level 0.95.

No criterion is violated in any counted run.

## Sensitivity analyses (unadjusted)

- control: 18 counted runs; void attempts: none; runs without a counted attempt: none
- treatment: 18 counted runs; void attempts: none; runs without a counted attempt: none
- protocol deviations whose scope touches these runs: 0, scopes: none

| Population | Runs | H1 | `p_less` | H2 | Corrected | S1 `p_less` | S2 p | Scopes of the protocol deviations that touch it |
|---|---|---|---|---|---|---|---|---|
| without `boxoffice` | 30 | INCONCLUSIVE | 1.0000 | INCONCLUSIVE | 1/2 | 1.0000 | 0.9300 | none |
| without the runs with a detector hit | 36 | INCONCLUSIVE | 1.0000 | INCONCLUSIVE | 1/2 | 1.0000 | 0.9300 | none |
| without the capped runs | 36 | INCONCLUSIVE | 1.0000 | INCONCLUSIVE | 1/2 | 1.0000 | 0.9300 | none |

- sign test over tasks, H1, per-task difference of escape shares: 0 favourable of 0 informative tasks, p 1.0000 (`1`)
- sign test over tasks, S1, per-task difference of defect shares: 0 favourable of 0 informative tasks, p 1.0000 (`1`)
- sign test over tasks, H2, per-task majority of corrected runs: 0 favourable of 0 informative tasks, p 1.0000 (`1`)

## Runs

| Run | Attempts | State | Accepted | Defective | Blocks: finding, pending, traceability, give-ups | Detector hits | Messages | Notional USD |
|---|---|---|---|---|---|---|---|---|
| `train-fare-treatment-3` | 1 completed | completed | true | false | 0, 0, 0, 0 | 0 | 283 | 2.41 |
| `train-fare-control-1` | 1 completed | completed | true | false | 0, 0, 0, 0 | 0 | 313 | 0.61 |
| `library-late-fees-treatment-3` | 1 completed | completed | false | false | 0, 12, 0, 4 | 0 | 538 | 2.08 |
| `electricity-tariff-treatment-1` | 1 completed | completed | true | false | 0, 0, 0, 0 | 0 | 630 | 5.80 |
| `library-late-fees-treatment-2` | 1 completed | completed | true | false | 0, 2, 0, 0 | 0 | 436 | 3.96 |
| `electricity-tariff-control-2` | 1 completed | completed | true | false | 0, 0, 0, 0 | 0 | 73 | 0.73 |
| `bicycle-rental-treatment-2` | 1 completed | completed | true | false | 0, 1, 0, 0 | 0 | 378 | 3.20 |
| `exam-grade-control-3` | 1 completed | completed | true | false | 1, 0, 0, 0 | 0 | 410 | 3.51 |
| `bicycle-rental-treatment-3` | 1 completed | completed | true | false | 0, 0, 0, 0 | 0 | 306 | 2.69 |
| `train-fare-treatment-1` | 1 completed | completed | true | false | 0, 1, 0, 0 | 0 | 413 | 3.40 |
| `bicycle-rental-control-2` | 1 completed | completed | true | false | 0, 0, 0, 0 | 0 | 285 | 2.48 |
| `library-late-fees-treatment-1` | 1 completed | completed | false | false | 0, 3, 0, 1 | 0 | 223 | 1.96 |
| `library-late-fees-control-3` | 1 completed | completed | true | false | 1, 0, 0, 0 | 0 | 309 | 0.74 |
| `electricity-tariff-treatment-3` | 1 completed | completed | true | false | 0, 0, 0, 0 | 0 | 667 | 6.05 |
| `exam-grade-treatment-3` | 1 completed | completed | true | false | 1, 0, 0, 0 | 0 | 524 | 4.64 |
| `train-fare-control-2` | 1 completed | completed | true | false | 0, 0, 0, 0 | 0 | 360 | 2.98 |
| `library-late-fees-control-1` | 1 completed | completed | true | false | 1, 0, 0, 0 | 0 | 431 | 3.77 |
| `electricity-tariff-control-3` | 1 completed | completed | true | false | 0, 0, 0, 0 | 0 | 630 | 5.69 |
| `boxoffice-treatment-3` | 1 completed | completed | true | false | 0, 1, 0, 0 | 0 | 341 | 2.79 |
| `electricity-tariff-control-1` | 1 completed | completed | true | false | 0, 0, 0, 0 | 0 | 555 | 0.58 |
| `bicycle-rental-control-1` | 1 completed | completed | true | false | 0, 0, 0, 0 | 0 | 321 | 1.79 |
| `exam-grade-treatment-1` | 1 completed | completed | false | false | 0, 3, 0, 1 | 0 | 344 | 3.01 |
| `bicycle-rental-control-3` | 1 completed | completed | true | false | 0, 0, 0, 0 | 0 | 372 | 3.11 |
| `boxoffice-treatment-1` | 1 completed | completed | true | false | 0, 1, 0, 0 | 0 | 304 | 2.68 |
| `electricity-tariff-treatment-2` | 1 completed | completed | true | false | 0, 0, 0, 0 | 0 | 632 | 5.77 |
| `boxoffice-control-1` | 1 completed | completed | true | false | 0, 0, 0, 0 | 0 | 292 | 1.44 |
| `exam-grade-control-1` | 1 completed | completed | true | false | 0, 0, 0, 0 | 0 | 305 | 2.58 |
| `library-late-fees-control-2` | 1 completed | completed | true | false | 0, 0, 0, 0 | 0 | 371 | 3.10 |
| `exam-grade-control-2` | 1 completed | completed | true | false | 1, 0, 0, 0 | 0 | 304 | 2.59 |
| `boxoffice-control-2` | 1 completed | completed | true | false | 0, 0, 0, 0 | 0 | 405 | 3.29 |
| `train-fare-control-3` | 1 completed | completed | true | false | 0, 0, 0, 0 | 0 | 304 | 2.59 |
| `boxoffice-treatment-2` | 1 completed | completed | true | false | 0, 0, 0, 0 | 0 | 271 | 2.27 |
| `exam-grade-treatment-2` | 1 completed | completed | false | false | 1, 2, 0, 1 | 0 | 470 | 4.30 |
| `boxoffice-control-3` | 1 completed | completed | true | false | 0, 0, 0, 0 | 0 | 293 | 2.50 |
| `bicycle-rental-treatment-1` | 1 completed | completed | true | false | 0, 1, 0, 0 | 0 | 308 | 2.59 |
| `train-fare-treatment-2` | 1 completed | completed | true | false | 0, 0, 0, 0 | 0 | 355 | 2.94 |

## Deviations

Entries per kind: provenance 0, quota-stop 0, harness-error 0, quota-blocked 0, harness-blocked 0, token-exposed 0, protocol 0. Every entry is in `DEVIATIONS.md`.

## Limits

- the tasks are small fixtures built for this study, each with a test that leaves an
  implemented criterion partly unchecked by construction, so block rates are not
  those of natural code;
- `boxoffice` was built for the demonstration of the loop and is in-sample;
- the truth is model-written (`gpt-5.6-terra`) from the spec alone; no human labels
  any item;
- one coding agent and one model; the gate's executors run on the coder's model, in
  its session;
- the control arm holds the same tools and instructions, so it measures the gate's
  requirement, not the presence of the modules;
- the escape test treats a task's runs as exchangeable; the sign test over tasks
  does not, and has less power;
- intervals and secondary tests are unadjusted.

## Exploratory discussion (exploratory)

<!-- exploratory discussion: at most 300 words, written by hand -->

The oracle found no defective final change in either arm, 0/18 each, so no gate had an escape to prevent. On these six small tasks the coding agent implemented the task criterion correctly in every run, and the study's primary test had nothing to work on. A study with the same design needs tasks on which the agent sometimes ships a defect, so that the contrast between the arms can show.

The four treatment runs that the acceptance gate rejected, 4/18, were all oracle-clean. In each one the blocking findings are `stale_proof`: the session ended without publishing a verified proof set (`.musthold/verified/manifest.json` absent). Their records show pending blocks on proofs awaiting their host and give-ups of the end-of-turn hook. The rejections come from proofs the session did not finish, not from a proof that judged a correct change wrong.

The control arm ran the proof modules on its own in 18/18 runs, so it saw the same counterexamples as the treatment arm without being held by them. That narrows the contrast to the gate's requirement, as the limits say.

H2 rests on two treatment runs with a finding block; one of them, 1/2, was corrected and accepted.
