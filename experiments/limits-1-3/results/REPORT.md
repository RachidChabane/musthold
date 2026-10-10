# Pilot on limits 1-3: report

## 1. Provenance

<!-- quoted:begin -->
- run id: run-02
- pre-registration tags: prereg-limits-1-3 (superseded), prereg-limits-1-3-a1
- tag commits: e95f55147cd66fe778af4134cda6dfcaaad5c5a9, 3ba0b8d1f08d479b5e88823a416dd5cf43a9b54c
- timestamp genTimes: {'prereg': '2026-10-06T21:49:09+00:00', 'a1': '2026-10-07T00:56:28+00:00'}
- amendment.json digest: fa88c389a77d95cd1781a28eb65a2b8ac0b8b2da4d5b524fea87a4af292b1529
- imports (files per role): {'contrast-generation': 70, 'terra-generation': 40, 'relabel': 15}
- run-01 counts per kind (calls): {'contrast-generation': 10, 'terra-generation': 5, 'relabel': 2, 'retired attempts': 1}
- pre-registration commit: 3ba0b8d1f08d479b5e88823a416dd5cf43a9b54c
- harness digest: dbc9ce226df9524cf68a5fa7a6f17c15244bc241cdbe16ca2dac168088621570
- fixture digest: 0d2e1be50cbc6ff3abdae4b4b61e11caaa8204622c850177a8884cd232f16c9b
- CLI versions: {'claude': '2.1.296', 'codex': '0.155.1'}
- models: [{'cli': 'claude', 'model': 'claude-opus-5-5', 'provenance': 'reported'}, {'cli': 'claude', 'model': 'claude-sonnet-5-5', 'provenance': 'reported'}, {'cli': 'codex', 'model': 'gpt-5.6-terra', 'provenance': 'requested_only'}, {'cli': 'codex', 'model': 'gpt-6-luna', 'provenance': 'requested_only'}]
- bootstrap resamples: 10000
- permutations: 10000
- seed: 20261006
- scenarios: 134 (including 10 full sets)
<!-- quoted:end -->

## 2. Conclusions

<!-- quoted:begin -->
- D1 met its pre-registered band at pilot scale (estimate [0.000, 0.561], n = 3). The fixture is author-built (10 criteria, 38 tests, one Python domain); this does not establish performance on other repositories.
- D2 fell in its pre-registered NO-GO band (estimate [0.095, 0.905]); mutant validity must be redesigned before a powered study.
- D3 met its pre-registered band at pilot scale (estimate [0.000, 0.032], n = 118). The fixture is author-built (10 criteria, 38 tests, one Python domain); this does not establish performance on other repositories.
- The pilot cannot decide D4 (estimate [0.000, 1.000]); a decision needs about 16 units.
- D5 met its pre-registered band at pilot scale (estimate [0.000, 0.179], n = 134). The fixture is author-built (10 criteria, 38 tests, one Python domain); this does not establish performance on other repositories.
- The pilot cannot decide D6 (estimate [0.779, 0.962]); a decision needs about unreachable units.
- D7 met its pre-registered band at pilot scale (estimate [0.839, 1.000], n = 30). The fixture is author-built (10 criteria, 38 tests, one Python domain); this does not establish performance on other repositories.
- D8 met its pre-registered band at pilot scale (estimate [0.701, 1.000], n = 9). The fixture is author-built (10 criteria, 38 tests, one Python domain); this does not establish performance on other repositories.
- D9 met its pre-registered band at pilot scale (estimate [0.641, 0.933], n = 38). The fixture is author-built (10 criteria, 38 tests, one Python domain); this does not establish performance on other repositories.
- The pilot cannot decide D10 (estimate [0.588, 0.923]); a decision needs about unreachable units.
- The pilot cannot decide D11 (estimate [0.074, 0.525]); a decision needs about 165 units.
<!-- quoted:end -->

## 3. Decision table

| ID | limit | measure | n | estimate [lo, hi] | GO | NO-GO | outcome | capped by |
|---|---|---|---|---|---|---|---|---|
| D4 | 1 | scenario false-block rate over ADEQUATE scenarios (policy G3, primary (oracle-adjudicated) truth) | 16 | 0.500 [0.000, 1.000] | point <= 0.10 and upper <= 0.25 | lower > 0.25 | INCONCLUSIVE |  |
| D2 | 1 | full-set false block over ADEQUATE full sets (policy G3, primary (oracle-adjudicated) truth) | 2 | 0.500 [0.095, 0.905] | <= floor(0.15 n_A) | >= ceil(0.5 n_A) | NO-GO |  |
| D11 | 3 | Cohen's kappa, J-H binary verdict vs matrix binary verdict (determinate items) | 36 | 0.289 [0.074, 0.525] | point >= 0.60, lower >= 0.30 | upper < 0.40 | INCONCLUSIVE |  |
| D10 | 3 | matrix binary verdict, balanced accuracy over the tests (UNDETERMINED counts as predicted bad) | 38 | 0.765 [0.588, 0.923] | point >= 0.80, lower >= 0.65, lower - N6 mean > 0, mean DC above the N3 95th percentile | upper < 0.70, or mean DC not above the N3 mean | INCONCLUSIVE |  |
| D1 | 1 | full-set PASS on an uncovered criterion {6, 7, 10} (policy G3, primary (oracle-adjudicated) truth) | 3 | 0.000 [0.000, 0.561] | 0/3 | >= 1/3 | GO |  |
| D8 | 2 | criteria of C_D8 with >= 2 VALID_x arm-T mutants (C_D8 = ['PARK-1~1', 'PARK-2~1', 'PARK-4~1', 'PARK-5~1', 'PARK-6~1', 'PARK-7~1', 'PARK-8~1', 'PARK-9~1', 'PARK-10~1']) | 9 | 1.000 [0.701, 1.000] | 9/9 | <= 6/9 | GO |  |
| D9 | 3 | J-H NAME-MASKED bad-test detection (predicted bad = MISALIGNED or UNDETERMINED; gold bad n = 24) | 38 | 0.833 [0.641, 0.933] | recall and precision point >= 0.80, lower >= 0.60, precision - N2 >= 0.10 | either upper < 0.70 | GO |  |
| D6 | 2 | VALID_x precision against oracle-valid-for-target, arm T (primary-matrix VALID_x mutants) | 42 | 0.905 [0.779, 0.962] | point >= 0.80, lower >= 0.70, precision - N7 >= 0.10 (waived if N7 >= 0.90) | upper < 0.70 | INCONCLUSIVE |  |
| D7 | 2 | planted controls: r-items VALID_w for their criterion, n1-items VALID_w for any | 30 | 1.000 [0.839, 1.000] | >= 16/20 r-items and <= 1/10 n1-items | <= 10/20 r-items, or >= 3/10 n1-items | GO |  |
| D3 | 1 | scenario false-pass rate over INADEQUATE scenarios (policy G3, primary (oracle-adjudicated) truth) | 118 | 0.000 [0.000, 0.032] | point <= 0.05 and upper <= 0.20 | lower > 0.20 | GO |  |
| D5 | 1 | escalation rate over all scenarios (policy G3, primary (oracle-adjudicated) truth) | 134 | 0.060 [0.000, 0.179] | point <= 0.30 and upper <= 0.45 | lower > 0.50 | GO |  |

Composite verdict:

REDESIGN(mutant validity)

## 4. Feasibility

| ID | check | value | status |
|---|---|---|---|
| F1 | valid outputs | 99.9% of 1047 calls | green |
| F2 | isolation violations | 4 | red |
| F3 | judge missing rate (worst judge) | 0.5% | green |
| F4 | arm-T stillborn share | 1.6% | green |
| F5 | criteria with >= 2 VALID_w arm-T mutants (all ten criteria) | 10 | green |
| F6 | runner cross-check exact | True | green |
| F7 | Claude cost per criterion, production path (mean; max in the cost table) | 0.47 USD mean, 0.55 USD max | green |
| F8 | wall clock | 1.18 h | green |

Protocol verdict: STOP_AND_REDESIGN_PROTOCOL

## 5. Preconditions

| ID | precondition | status | detail | affects |
|---|---|---|---|---|
| P1 | label verification passes and the runner cross-check is exact | pass | label verification passed and the runner cross-check is exact | D1, D2, D3, D4, D5, D6, D7, D8, D9, D10, D11 |
| P2 | Fleiss kappa on binary alignment truth (point >= 0.60, lower >= 0.40) | pass | kappa 0.854, lower 0.726 | D9, D11 |
| P3 | leakage probe not significant for J-H (one-sided exact binomial against 0.5, alpha 0.05) | pass | 14/28 separated, p = 0.5747 | D9, D11 |
| P4 | canaries pass and every pin matches | pass | all pins have canaries | D1, D2, D3, D4, D5, D6, D7, D8, D9, D10, D11 |
| P5 | judge missing rate <= 10% (V-H1) | pass | 0/85 missing (0.0%) | D1, D2, D3, D4, D5, D6, D7, D8, D10, D11 |
| P5 | judge missing rate <= 10% (V-H2) | pass | 0/85 missing (0.0%) | D1, D2, D3, D4, D5, D6, D7, D8, D10, D11 |
| P5 | judge missing rate <= 10% (J-H NAME-MASKED) | pass | 0/190 missing (0.0%) | D1, D2, D3, D4, D5, D9, D11 |
| P6 | no logged protocol deviation touches a decisional endpoint's inputs | pass | 0 protocol entries; 2 provenance entries logged; see DEVIATIONS.md in the run directory |  |
| P7 | no stage blocked by quota exhaustion | pass | 0 quota-blocked entries, 6 quota-pause entries logged; see DEVIATIONS.md in the run directory |  |

## 6. Blind spot (descriptive, reported first)

**shared blind spot SB (descriptive): contrasts, bootstrap interval and fixed reading**

| measure | value |
|---|---|
| Delta_S | -0.021 |
| Delta_O | -0.022 |
| SB_S | -0.069 |
| SB_O | 0.060 |
| SB | -0.004 |
| SB interval lo | -0.024 |
| SB interval hi | 0.011 |
| reading | no material shared blind spot |
| indicated with the upper bound below materiality | False |
| criteria in C* | 9 |

**e(W, M) over C*: writers (rows) by mutant sources, with pooled counts**

| writer | source | escaped | caught | excluded | e(W,M) criterion-weighted |
|---|---|---|---|---|---|
| S | S | 6 | 45 | 0 | 0.106 |
| S | O | 2 | 41 | 0 | 0.044 |
| S | op | 3 | 32 | 0 | 0.127 |
| O | S | 9 | 42 | 0 | 0.159 |
| O | O | 4 | 39 | 0 | 0.089 |
| O | op | 2 | 33 | 0 | 0.111 |

**criteria outside C* with their reasons**

| criterion | reasons |
|---|---|
| PARK-1~1 | U(op) is empty |

**escaping model mutants: coverage hole (an escaping operator mutant edits the same definition) vs model-specific**

| writer | source | escaping model mutants | coverage hole | model-specific | coverage-hole share |
|---|---|---|---|---|---|
| S | S | 6 | 0 | 6 | 0.000 |
| S | O | 2 | 0 | 2 | 0.000 |
| O | S | 9 | 0 | 9 | 0.000 |
| O | O | 4 | 0 | 4 | 0.000 |

**exploratory cells (labelled cross-family where luna or terra is involved)**

| writer | source | family | criteria | escaped | caught | excluded | e(W,M) | luna calls not complete | status |
|---|---|---|---|---|---|---|---|---|---|
| S | terra | cross-family | 3 | 0 | 7 | 0 | 0.000 | 0 |  |
| S | luna | cross-family | 10 | 3 | 23 | 0 | 0.100 | 0 |  |
| O | terra | cross-family | 3 | 0 | 7 | 0 | 0.000 | 0 |  |
| O | luna | cross-family | 10 | 2 | 24 | 0 | 0.067 | 0 |  |
| L | S | cross-family | 10 | 23 | 31 | 0 | 0.382 | 0 |  |
| L | O | cross-family | 10 | 14 | 32 | 0 | 0.310 | 0 |  |
| L | op | cross-family | 9 | 2 | 33 | 0 | 0.111 | 0 |  |
| L | terra | cross-family | 3 | 2 | 5 | 0 | 0.167 | 0 |  |
| L | luna | cross-family | 10 | 6 | 20 | 0 | 0.233 | 0 |  |
| F | S | same-family | 7 | 13 | 23 | 0 | 0.331 | 0 |  |
| F | O | same-family | 7 | 10 | 22 | 0 | 0.257 | 0 |  |
| F | op | same-family | 6 | 3 | 10 | 0 | 0.333 | 0 |  |
| F | terra | cross-family | 3 | 0 | 7 | 0 | 0.000 | 0 |  |
| F | luna | cross-family | 7 | 5 | 13 | 0 | 0.238 | 0 |  |

**per-source oracle-valid yield: Sonnet (S), Opus (O), operators (op), terra, luna**

| source | candidates | non-stillborn | oracle-valid | share | wilson lo | wilson hi |
|---|---|---|---|---|---|---|
| S | 62 | 61 | 54 | 0.885 | 0.782 | 0.943 |
| O | 52 | 52 | 46 | 0.885 | 0.770 | 0.946 |
| op | 99 | 99 | 35 | 0.354 | 0.266 | 0.452 |
| terra | 10 | 10 | 7 | 0.700 | 0.397 | 0.892 |
| luna | 30 | 30 | 26 | 0.867 | 0.703 | 0.947 |

## 7. Confusion matrices and descriptive measures

**6-class label x matrix signature, arm T mutants**

| label | predicted | UNDEFINED_OWN | INERT | OFF_TARGET | PARTIAL | SPECIFIC | OVERBROAD |
|---|---|---|---|---|---|---|---|
| GOOD | SPECIFIC | 0 | 0 | 0 | 5 | 7 | 0 |
| GOOD_BROAD | OVERBROAD | 0 | 0 | 0 | 1 | 0 | 1 |
| WEAK | PARTIAL | 0 | 0 | 0 | 3 | 1 | 0 |
| VACUOUS | INERT | 0 | 6 | 0 | 0 | 0 | 0 |
| MIS_OTHER | OFF_TARGET | 0 | 0 | 6 | 0 | 0 | 0 |
| OFFSPEC | INERT or OFF_TARGET | 0 | 3 | 1 | 4 | 0 | 0 |

**6-class label x matrix signature, the 30 violations as the mutant pool**

| label | predicted | UNDEFINED_OWN | INERT | OFF_TARGET | PARTIAL | SPECIFIC | OVERBROAD |
|---|---|---|---|---|---|---|---|
| GOOD | SPECIFIC | 0 | 0 | 0 | 0 | 12 | 0 |
| GOOD_BROAD | OVERBROAD | 0 | 0 | 0 | 0 | 0 | 2 |
| WEAK | PARTIAL | 0 | 0 | 0 | 3 | 1 | 0 |
| VACUOUS | INERT | 0 | 6 | 0 | 0 | 0 | 0 |
| MIS_OTHER | OFF_TARGET | 0 | 0 | 6 | 0 | 0 | 0 |
| OFFSPEC | INERT or OFF_TARGET | 0 | 8 | 0 | 0 | 0 | 0 |

**6-class label x judge verdict, per judge and arm**

| judge | arm | label | ALIGNED | MISALIGNED | UNDETERMINED |
|---|---|---|---|---|---|
| J-H | NAME-MASKED | GOOD | 11 | 1 | 0 |
| J-H | NAME-MASKED | GOOD_BROAD | 1 | 0 | 1 |
| J-H | NAME-MASKED | WEAK | 0 | 4 | 0 |
| J-H | NAME-MASKED | VACUOUS | 0 | 6 | 0 |
| J-H | NAME-MASKED | MIS_OTHER | 0 | 6 | 0 |
| J-H | NAME-MASKED | OFFSPEC | 4 | 3 | 1 |
| J-H | NAME-SHOWN | GOOD | 11 | 1 | 0 |
| J-H | NAME-SHOWN | GOOD_BROAD | 2 | 0 | 0 |
| J-H | NAME-SHOWN | WEAK | 0 | 4 | 0 |
| J-H | NAME-SHOWN | VACUOUS | 0 | 6 | 0 |
| J-H | NAME-SHOWN | MIS_OTHER | 0 | 6 | 0 |
| J-H | NAME-SHOWN | OFFSPEC | 4 | 3 | 1 |
| J-X | NAME-MASKED | GOOD | 11 | 1 | 0 |
| J-X | NAME-MASKED | GOOD_BROAD | 1 | 1 | 0 |
| J-X | NAME-MASKED | WEAK | 0 | 4 | 0 |
| J-X | NAME-MASKED | VACUOUS | 0 | 6 | 0 |
| J-X | NAME-MASKED | MIS_OTHER | 0 | 6 | 0 |
| J-X | NAME-MASKED | OFFSPEC | 4 | 4 | 0 |

**6-class label x judge answer type x coverage (modal answer)**

| judge | arm | label | declared|FULL | declared|PARTIAL | declared|WEAK | declared|NONE | true B|FULL | true B|PARTIAL | true B|WEAK | true B|NONE | other|FULL | other|PARTIAL | other|WEAK | other|NONE | NONE|FULL | NONE|PARTIAL | NONE|WEAK | NONE|NONE |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| J-H | NAME-MASKED | GOOD | 10 | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 |
| J-H | NAME-MASKED | GOOD_BROAD | 2 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 |
| J-H | NAME-MASKED | WEAK | 0 | 0 | 4 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 |
| J-H | NAME-MASKED | VACUOUS | 0 | 0 | 6 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 |
| J-H | NAME-MASKED | MIS_OTHER | 0 | 0 | 0 | 0 | 3 | 3 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 |
| J-H | NAME-MASKED | OFFSPEC | 2 | 2 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 3 |
| J-H | NAME-SHOWN | GOOD | 11 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 |
| J-H | NAME-SHOWN | GOOD_BROAD | 2 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 |
| J-H | NAME-SHOWN | WEAK | 0 | 0 | 4 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 |
| J-H | NAME-SHOWN | VACUOUS | 0 | 0 | 6 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 |
| J-H | NAME-SHOWN | MIS_OTHER | 0 | 0 | 0 | 0 | 4 | 2 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 |
| J-H | NAME-SHOWN | OFFSPEC | 1 | 3 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 2 |
| J-X | NAME-MASKED | GOOD | 8 | 3 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 |
| J-X | NAME-MASKED | GOOD_BROAD | 1 | 0 | 0 | 0 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 |
| J-X | NAME-MASKED | WEAK | 0 | 0 | 4 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 |
| J-X | NAME-MASKED | VACUOUS | 0 | 0 | 6 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 |
| J-X | NAME-MASKED | MIS_OTHER | 0 | 0 | 0 | 0 | 5 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 |
| J-X | NAME-MASKED | OFFSPEC | 3 | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 1 | 0 | 0 | 0 | 0 | 2 |

**6-class label x single-test gate outcome, G0 to G3**

| policy | label | PASS | ESCALATE | BLOCK |
|---|---|---|---|---|
| G0 | GOOD | 12 | 0 | 0 |
| G0 | GOOD_BROAD | 2 | 0 | 0 |
| G0 | WEAK | 4 | 0 | 0 |
| G0 | VACUOUS | 6 | 0 | 0 |
| G0 | MIS_OTHER | 6 | 0 | 0 |
| G0 | OFFSPEC | 8 | 0 | 0 |
| G1 | GOOD | 1 | 0 | 11 |
| G1 | GOOD_BROAD | 0 | 0 | 2 |
| G1 | WEAK | 0 | 0 | 4 |
| G1 | VACUOUS | 0 | 0 | 6 |
| G1 | MIS_OTHER | 0 | 0 | 6 |
| G1 | OFFSPEC | 0 | 0 | 8 |
| G2 | GOOD | 11 | 1 | 0 |
| G2 | GOOD_BROAD | 1 | 1 | 0 |
| G2 | WEAK | 0 | 4 | 0 |
| G2 | VACUOUS | 0 | 6 | 0 |
| G2 | MIS_OTHER | 0 | 6 | 0 |
| G2 | OFFSPEC | 4 | 4 | 0 |
| G3 | GOOD | 0 | 1 | 11 |
| G3 | GOOD_BROAD | 0 | 0 | 2 |
| G3 | WEAK | 0 | 0 | 4 |
| G3 | VACUOUS | 0 | 0 | 6 |
| G3 | MIS_OTHER | 0 | 0 | 6 |
| G3 | OFFSPEC | 0 | 0 | 8 |

**per-criterion full-set gate outcome under every policy**

| criterion | G0 | G1 | G2 | G3 | G3-prereg | G3-noiso | G3-strict | G3-lenient |
|---|---|---|---|---|---|---|---|---|
| PARK-1~1 | PASS | BLOCK | ESCALATE | BLOCK | BLOCK | BLOCK | BLOCK | BLOCK |
| PARK-2~1 | PASS | PASS | ESCALATE | ESCALATE | ESCALATE | ESCALATE | BLOCK | PASS |
| PARK-3~1 | PASS | BLOCK | ESCALATE | BLOCK | BLOCK | BLOCK | BLOCK | BLOCK |
| PARK-4~1 | PASS | BLOCK | ESCALATE | BLOCK | BLOCK | BLOCK | BLOCK | BLOCK |
| PARK-5~1 | PASS | BLOCK | ESCALATE | BLOCK | BLOCK | BLOCK | BLOCK | BLOCK |
| PARK-6~1 | PASS | BLOCK | ESCALATE | BLOCK | BLOCK | BLOCK | BLOCK | BLOCK |
| PARK-7~1 | PASS | BLOCK | ESCALATE | BLOCK | BLOCK | BLOCK | BLOCK | BLOCK |
| PARK-8~1 | PASS | BLOCK | ESCALATE | BLOCK | BLOCK | BLOCK | BLOCK | BLOCK |
| PARK-9~1 | PASS | BLOCK | ESCALATE | BLOCK | BLOCK | BLOCK | BLOCK | BLOCK |
| PARK-10~1 | PASS | BLOCK | ESCALATE | BLOCK | BLOCK | BLOCK | BLOCK | BLOCK |

**oracle class x pipeline class, arm T**

| oracle class | COLLATERAL | UNDETERMINED | VALID |
|---|---|---|---|
| oracle-collateral | 0 | 0 | 1 |
| oracle-valid | 2 | 0 | 52 |
| silent-equivalent-probable | 0 | 2 | 4 |

**oracle class x pipeline class, arm P**

| oracle class | EQUIVALENT | UNATTRIBUTED | UNDETERMINED |
|---|---|---|---|
| oracle-valid | 0 | 0 | 14 |
| silent-different | 0 | 1 | 0 |
| silent-equivalent-probable | 8 | 1 | 0 |

**oracle class x pipeline class, arm O**

| oracle class | not judged |
|---|---|
| oracle-collateral | 60 |
| oracle-valid | 31 |
| silent-untested | 3 |

**J-H NAME-MASKED (rows) x matrix (columns), with cross-check counts**

| judge verdict / measure | ALIGNED | MISALIGNED | UNDETERMINED |
|---|---|---|---|
| ALIGNED | 6 | 10 | 0 |
| MISALIGNED | 2 | 18 | 0 |
| UNDETERMINED | 1 | 1 | 0 |
| Cohen kappa (determinate) | 0.289 | 36 |  |
| raw agreement (determinate) | 24 | 36 |  |
| P(matrix correct \| judge wrong) | 7 | 7 |  |
| P(judge correct \| matrix wrong) | 7 | 7 |  |
| both wrong | 0 | 38 |  |
| matrix silent (UNDETERMINED) | 0 | 38 |  |

**class shares per arm (pipeline classes for T and P; oracle classes for every arm)**

| arm | class | count | share |
|---|---|---|---|
| T | COLLATERAL | 2 | 0.033 |
| T | UNDETERMINED | 2 | 0.033 |
| T | VALID | 57 | 0.934 |
| P | EQUIVALENT | 8 | 0.333 |
| P | UNATTRIBUTED | 2 | 0.083 |
| P | UNDETERMINED | 14 | 0.583 |
| T (oracle) | oracle-collateral | 1 | 0.016 |
| T (oracle) | oracle-valid | 54 | 0.885 |
| T (oracle) | silent-equivalent-probable | 6 | 0.098 |
| P (oracle) | oracle-valid | 14 | 0.583 |
| P (oracle) | silent-different | 1 | 0.042 |
| P (oracle) | silent-equivalent-probable | 9 | 0.375 |
| O (oracle) | oracle-collateral | 60 | 0.638 |
| O (oracle) | oracle-valid | 31 | 0.330 |
| O (oracle) | silent-untested | 3 | 0.032 |
| X (oracle) | oracle-valid | 25 | 0.926 |
| X (oracle) | silent-untested | 2 | 0.074 |
| R (oracle) | oracle-collateral | 1 | 0.200 |
| R (oracle) | oracle-valid | 3 | 0.600 |
| R (oracle) | silent-untested | 1 | 0.200 |
| L (oracle) | oracle-valid | 13 | 0.867 |
| L (oracle) | silent-untested | 2 | 0.133 |

**VALID share backed by an executed witness; false-discard rate**

| measure | count | total | share |
|---|---|---|---|
| VALID mutants with an executed witness for the target | 57 | 57 | 1.000 |
| oracle-valid mutants not VALID (false discard) | 2 | 53 | 0.038 |

**oracle-valid share of non-stillborn mutants, arm T vs arm O**

| arm | oracle-valid | non-stillborn | share | wilson lo | wilson hi |
|---|---|---|---|---|---|
| T | 54 | 61 | 0.885 | 0.782 | 0.943 |
| O | 31 | 94 | 0.330 | 0.243 | 0.430 |

**arm-T yield per slot**

| slot | candidates | stillborn | VALID (primary) | oracle-valid for target |
|---|---|---|---|---|
| S1 | 19 | 0 | 18 | 18 |
| S2 | 18 | 1 | 15 | 11 |
| S3 | 25 | 0 | 24 | 24 |

**arm-T yield per generator**

| generator | candidates | stillborn | VALID (primary) | oracle-valid for target |
|---|---|---|---|---|
| claude-sonnet-5-5/claude | 62 | 1 | 57 | 53 |

**audit agreement on 'VALID for the declared target' (A-OPUS, pipeline, oracle)**

| pair | n | raw agreement | kappa | ci lo | ci hi |
|---|---|---|---|---|---|
| A-OPUS vs pipeline | 66 | 0.985 | 0.951 | 0.880 | 1.000 |
| A-OPUS vs oracle | 66 | 0.924 | 0.791 | 0.618 | 0.952 |
| pipeline vs oracle | 66 | 0.909 | 0.744 | 0.611 | 0.891 |
| Fleiss, three raters | 66 | 0.909 | 0.822 | 0.713 | 0.926 |

**modal accuracy against PRIMARY and CONSENSUS labels (UNDETERMINED counts as wrong)**

| judge | arm | PRIMARY correct | PRIMARY n | CONSENSUS correct | CONSENSUS n |
|---|---|---|---|---|---|
| J-H | NAME-MASKED | 31 | 38 | 29 | 33 |
| J-H | NAME-SHOWN | 32 | 38 | 30 | 33 |
| J-X | NAME-MASKED | 32 | 38 | 30 | 33 |

**share of tests with modal_share >= 0.8; Fleiss kappa over repetitions**

| judge | arm | stable | tests | share | fleiss kappa | ci lo | ci hi | complete tests |
|---|---|---|---|---|---|---|---|---|
| J-H | NAME-MASKED | 36 | 38 | 0.947 | 0.962 | 0.904 | 1.000 | 38 |
| J-H | NAME-SHOWN | 37 | 38 | 0.974 | 0.958 | 0.905 | 1.000 | 37 |
| J-X | NAME-MASKED | 38 | 38 | 1.000 | 0.988 | 0.960 | 1.000 | 38 |

**false ALIGNED on gold MIS_OTHER and OFFSPEC, NAME-SHOWN vs NAME-MASKED (exact McNemar)**

| judge | tests | false ALIGNED shown | false ALIGNED masked | shown only | masked only | p |
|---|---|---|---|---|---|---|
| J-H | 14 | 4 | 4 | 0 | 0 | 1.000 |
| J-X | 14 | 0 | 4 | 0 | 4 | 0.125 |

**J-H vs J-X error overlap (NAME-MASKED, against PRIMARY labels)**

| pattern | tests |
|---|---|
| both wrong | 6 |
| J-H only | 1 |
| J-X only | 0 |
| neither | 31 |

**leakage probe: tests separated (gold good vs MIS_OTHER and OFFSPEC)**

| judge | separated | tests |
|---|---|---|
| J-H | 14 | 28 |
| J-X | 0 | 0 |

**VALID_w vs VALID_x per criterion (primary-matrix arm-T mutants); predicted exposure {3, 5, 8}**

| criterion | VALID_w | VALID_x | vetoed by another binding | in C_D8 | exposure |
|---|---|---|---|---|---|
| PARK-1~1 | 4 | 4 | 0 | yes |  |
| PARK-2~1 | 4 | 4 | 0 | yes |  |
| PARK-3~1 | 7 | 3 | 4 | no | predicted |
| PARK-4~1 | 6 | 6 | 0 | yes |  |
| PARK-5~1 | 7 | 3 | 4 | yes | predicted |
| PARK-6~1 | 6 | 6 | 0 | yes |  |
| PARK-7~1 | 4 | 4 | 0 | yes |  |
| PARK-8~1 | 5 | 2 | 3 | yes | predicted |
| PARK-9~1 | 8 | 5 | 3 | yes |  |
| PARK-10~1 | 6 | 5 | 1 | yes |  |

**errors of the two host votes against the oracle**

| measure | count | of | rate |
|---|---|---|---|
| mutants with both votes | 61 |  |  |
| V-H1 wrong | 6 | 61 | 0.098 |
| V-H2 wrong | 6 | 61 | 0.098 |
| both wrong | 6 | 61 | 0.098 |
| P(V-H2 wrong \| V-H1 wrong) | 6 | 6 | 1.000 |

**J-H NAME-MASKED modal_share against labels (reliability, ECE, risk-coverage) and the UNDETERMINED rate**

| measure | modal_share | n or coverage | accuracy, ECE or risk |
|---|---|---|---|
| reliability | 0.200 | 0 |  |
| reliability | 0.400 | 0 |  |
| reliability | 0.600 | 0 |  |
| reliability | 0.800 | 0 |  |
| reliability | 1.000 | 36 | 0.861 |
| ECE |  | 36 | 0.139 |
| risk-coverage | 1.000 | 1.000 | 0.139 |
| UNDETERMINED rate |  | 2 | 0.053 |

**G3 on poisoned vs clean criteria; GOOD_BROAD vs GOOD signatures; G3 vs G3-prereg vs G3-noiso**

| contrast | count | total | rate | wilson lo | wilson hi |
|---|---|---|---|---|---|
| G3 false-block rate, poisoned {3, 5, 8} | 0 | 0 |  | 0.000 | 1.000 |
| G3 false-block rate, clean {1, 2, 4, 9} | 8 | 16 | 0.500 | 0.280 | 0.720 |
| G3 false-pass rate | 0 | 118 | 0.000 | 0.000 | 0.032 |
| G3-prereg false-pass rate | 0 | 118 | 0.000 | 0.000 | 0.032 |
| G3-noiso false-pass rate | 0 | 118 | 0.000 | 0.000 | 0.032 |
| GOOD signatures | PARTIAL=5, SPECIFIC=7 |  |  |  |  |
| GOOD_BROAD signatures | OVERBROAD=1, PARTIAL=1 |  |  |  |  |

**inter-rater agreement, 6-class and binary, bootstrap 95% intervals**

| scheme | pair | n | raw agreement | kappa | ci lo | ci hi |
|---|---|---|---|---|---|---|
| 6-class | Cohen AUTHOR vs R-AUTHOR | 38 | 0.974 | 0.967 | 0.897 | 1.000 |
| 6-class | Cohen AUTHOR vs R-OAI | 38 | 0.895 | 0.867 | 0.752 | 0.967 |
| 6-class | Cohen R-AUTHOR vs R-OAI | 38 | 0.868 | 0.834 | 0.720 | 0.934 |
| 6-class | Fleiss AUTHOR, R-AUTHOR, R-OAI | 38 | 0.868 | 0.889 | 0.813 | 0.956 |
| binary | Cohen AUTHOR vs R-AUTHOR | 38 | 0.974 | 0.944 | 0.834 | 1.000 |
| binary | Cohen AUTHOR vs R-OAI | 38 | 0.921 | 0.838 | 0.665 | 1.000 |
| binary | Cohen R-AUTHOR vs R-OAI | 38 | 0.895 | 0.785 | 0.608 | 0.946 |
| binary | Fleiss AUTHOR, R-AUTHOR, R-OAI | 38 | 0.895 | 0.854 | 0.726 | 0.964 |

## 8. Error multiplication and attribution

| link | false pass | false block | definition |
|---|---|---|---|
| traceability | 1.000 | 0.000 | passes every bound test |
| alignment (J-H NAME-MASKED vs labels) | 0.167 | 0.071 | false pass = gold bad judged ALIGNED; false block = gold good judged MISALIGNED |
| matrix verdict vs labels | 0.042 | 0.429 | false pass = gold bad ALIGNED; false block = gold good MISALIGNED |

| measure | predicted under independence | observed | ratio | ratio 95% interval |
|---|---|---|---|---|
| false pass (gold bad test passed by G3, single-test scenarios) | 0.007 | 0.000 | 0.000 | [0.000, 0.000] |
| false block (gold good test blocked by G3, single-test scenarios) | 0.469 | 0.929 | 1.978 | [1.224, 4.667] |

Mutant validity against the oracle: false VALID rate 0.105, false-discard rate 0.038.

Attribution by substitution:

- D1: 0 error(s); removed by {'alignment': 0, 'validity': 0, 'isolation': 0, 'all': 0}; component gate
- D2: 1 error(s); removed by {'alignment': 0, 'validity': 1, 'isolation': 0, 'all': 1}; component mutant validity
- D3: 0 error(s); removed by {'alignment': 0, 'validity': 0, 'isolation': 0, 'all': 0}; component gate
- D4: 8 error(s); removed by {'alignment': 0, 'validity': 8, 'isolation': 0, 'all': 8}; component mutant validity
- D5: 8 error(s); removed by {'alignment': 2, 'validity': 0, 'isolation': 0, 'all': 2}; component alignment judge

## 9. Failure modes

Catalogue size by kind: gate error: 8; judge error: 7; matrix error: 7; pipeline/oracle discordance: 8.

| kind | id | summary | causal chain |
|---|---|---|---|
| matrix error | tests/test_intervals.py::test_breakdown_rejects_exit_before_entry | matrix PARTIAL on a GOOD test | label GOOD, declared PARK-1~1; signature PARTIAL (own 0.25, other 0.00, none 0.00); kills VALID mutants of criteria [1]; verdict MISALIGNED |
| judge error | tests/test_intervals.py::test_exit_before_entry_reports_reversed_times | J-H NAME-MASKED ALIGNED on a OFFSPEC test | label OFFSPEC, declared PARK-1~1; modal answer PARK-1~1 with coverage FULL and share 1.00; verdict ALIGNED, truth MISALIGNED |
| judge error | tests/test_intervals.py::test_quarter_hour_over_midnight_is_free | J-H NAME-MASKED MISALIGNED on a GOOD test | label GOOD, declared PARK-2~1; modal answer PARK-2~1 with coverage WEAK and share 1.00; verdict MISALIGNED, truth ALIGNED |
| matrix error | tests/test_night.py::test_night_stay_over_two_hours_costs_six_euros | matrix SPECIFIC on a WEAK test | label WEAK, declared PARK-6~1; signature SPECIFIC (own 0.6666666666666666, other 0.00, none 0.00); kills VALID mutants of criteria [6]; verdict ALIGNED |
| judge error | tests/test_receipt.py::test_receipt_for_two_hour_stay_prints_euros | J-H NAME-MASKED UNDETERMINED on a GOOD_BROAD test | label GOOD_BROAD, declared PARK-9~1; modal answer PARK-9~1 with coverage FULL and share 0.60; verdict UNDETERMINED, truth ALIGNED |
| matrix error | tests/test_tariff.py::test_cap_is_reached_after_seven_hours | matrix PARTIAL on a GOOD_BROAD test | label GOOD_BROAD, declared PARK-4~1; signature PARTIAL (own 0.16666666666666666, other 0.29, none 0.00); kills VALID mutants of criteria [3, 4]; verdict MISALIGNED |
| matrix error | tests/test_tariff.py::test_charge_is_capped_at_eighteen_euros | matrix PARTIAL on a GOOD test | label GOOD, declared PARK-4~1; signature PARTIAL (own 0.16666666666666666, other 0.00, none 0.00); kills VALID mutants of criteria [4]; verdict MISALIGNED |
| judge error | tests/test_tariff.py::test_hourly_charge_is_quoted_in_cents | J-H NAME-MASKED UNDETERMINED on a OFFSPEC test | label OFFSPEC, declared PARK-3~1; modal answer NONE with coverage NONE and share 0.60; verdict UNDETERMINED, truth MISALIGNED |
| matrix error | tests/test_tariff.py::test_hours_are_counted_from_entry_time | matrix PARTIAL on a GOOD test | label GOOD, declared PARK-3~1; signature PARTIAL (own 0.2857142857142857, other 0.00, none 0.00); kills VALID mutants of criteria [3]; verdict MISALIGNED |
| judge error | tests/test_tariff.py::test_long_stay_breakdown_uses_day_rate | J-H NAME-MASKED ALIGNED on a OFFSPEC test | label OFFSPEC, declared PARK-5~1; modal answer PARK-5~1 with coverage FULL and share 1.00; verdict ALIGNED, truth MISALIGNED |
| matrix error | tests/test_tariff.py::test_long_stay_charges_each_started_day | matrix PARTIAL on a GOOD test | label GOOD, declared PARK-5~1; signature PARTIAL (own 0.42857142857142855, other 0.00, none 0.00); kills VALID mutants of criteria [5]; verdict MISALIGNED |
| matrix error | tests/test_tariff.py::test_two_hour_stay_costs_two_started_hours | matrix PARTIAL on a GOOD test | label GOOD, declared PARK-3~1; signature PARTIAL (own 0.42857142857142855, other 0.00, none 0.00); kills VALID mutants of criteria [3]; verdict MISALIGNED |
| judge error | tests/test_tickets.py::test_lost_ticket_breakdown_shows_flat_fee | J-H NAME-MASKED ALIGNED on a OFFSPEC test | label OFFSPEC, declared PARK-8~1; modal answer PARK-8~1 with coverage PARTIAL and share 1.00; verdict ALIGNED, truth MISALIGNED |
| judge error | tests/test_tickets.py::test_validated_stay_breakdown_is_free | J-H NAME-MASKED ALIGNED on a OFFSPEC test | label OFFSPEC, declared PARK-7~1; modal answer PARK-7~1 with coverage PARTIAL and share 1.00; verdict ALIGNED, truth MISALIGNED |
| gate error | PARK-1:tests/test_intervals.py::test_breakdown_rejects_exit_before_entry+tests/test_intervals.py::test_exit_before_entry_raises_even_when_ticket_is_lost | false block on 2 test(s) of PARK-1~1 | ADEQUATE scenario, gate BLOCK; rules fired ['SURVIVING_VALID', 'SURVIVING_UNDETERMINED']; error removed by substituting ['validity', 'all'] |
| gate error | PARK-1:tests/test_intervals.py::test_breakdown_rejects_exit_before_entry+tests/test_intervals.py::test_exit_before_entry_raises_even_when_ticket_is_lost+tests/test_intervals.py::test_exit_before_entry_reports_reversed_times | false block on 3 test(s) of PARK-1~1 | ADEQUATE scenario, gate BLOCK; rules fired ['SURVIVING_VALID', 'SURVIVING_UNDETERMINED']; error removed by substituting ['validity', 'all'] |
| gate error | PARK-1:tests/test_intervals.py::test_breakdown_rejects_exit_before_entry+tests/test_intervals.py::test_exit_before_entry_raises_even_when_ticket_is_lost+tests/test_intervals.py::test_exit_before_entry_reports_reversed_times+tests/test_intervals.py::test_exit_earlier_than_entry_is_rejected | false block on 4 test(s) of PARK-1~1 | ADEQUATE scenario, gate BLOCK; rules fired ['SURVIVING_VALID', 'SURVIVING_UNDETERMINED', 'ALIGNMENT']; error removed by substituting ['validity', 'all'] |
| gate error | PARK-1:tests/test_intervals.py::test_breakdown_rejects_exit_before_entry+tests/test_intervals.py::test_exit_before_entry_raises_even_when_ticket_is_lost+tests/test_intervals.py::test_exit_earlier_than_entry_is_rejected | false block on 3 test(s) of PARK-1~1 | ADEQUATE scenario, gate BLOCK; rules fired ['SURVIVING_VALID', 'SURVIVING_UNDETERMINED', 'ALIGNMENT']; error removed by substituting ['validity', 'all'] |
| gate error | PARK-1:tests/test_intervals.py::test_exit_before_entry_raises_even_when_ticket_is_lost | false block on 1 test(s) of PARK-1~1 | ADEQUATE scenario, gate BLOCK; rules fired ['SURVIVING_VALID', 'SURVIVING_UNDETERMINED']; error removed by substituting ['validity', 'all'] |
| gate error | PARK-1:tests/test_intervals.py::test_exit_before_entry_raises_even_when_ticket_is_lost+tests/test_intervals.py::test_exit_before_entry_reports_reversed_times | false block on 2 test(s) of PARK-1~1 | ADEQUATE scenario, gate BLOCK; rules fired ['SURVIVING_VALID', 'SURVIVING_UNDETERMINED']; error removed by substituting ['validity', 'all'] |
| gate error | PARK-1:tests/test_intervals.py::test_exit_before_entry_raises_even_when_ticket_is_lost+tests/test_intervals.py::test_exit_before_entry_reports_reversed_times+tests/test_intervals.py::test_exit_earlier_than_entry_is_rejected | false block on 3 test(s) of PARK-1~1 | ADEQUATE scenario, gate BLOCK; rules fired ['SURVIVING_VALID', 'SURVIVING_UNDETERMINED', 'ALIGNMENT']; error removed by substituting ['validity', 'all'] |
| gate error | PARK-1:tests/test_intervals.py::test_exit_before_entry_raises_even_when_ticket_is_lost+tests/test_intervals.py::test_exit_earlier_than_entry_is_rejected | false block on 2 test(s) of PARK-1~1 | ADEQUATE scenario, gate BLOCK; rules fired ['SURVIVING_VALID', 'SURVIVING_UNDETERMINED', 'ALIGNMENT']; error removed by substituting ['validity', 'all'] |
| pipeline/oracle discordance | 23d20e0bf87ed079 | VALID but not oracle-valid | arm T, target PARK-6~1; pipeline class VALID; oracle silent-equivalent-probable []; differential NO_DIFF_FOUND; killed by 0 test(s) |
| pipeline/oracle discordance | 25b90165953c1666 | VALID but not oracle-valid | arm T, target PARK-4~1; pipeline class VALID; oracle oracle-valid [7]; differential DIFFERS; killed by 0 test(s) |
| pipeline/oracle discordance | 2c2ed14c225addfa | VALID but not oracle-valid | arm T, target PARK-9~1; pipeline class VALID; oracle silent-equivalent-probable []; differential NO_DIFF_FOUND; killed by 0 test(s) |
| pipeline/oracle discordance | 2fec2bdb9fe5c101 | oracle-valid but not VALID | arm T, target PARK-7~1; pipeline class COLLATERAL; oracle oracle-valid [7]; differential DIFFERS; killed by 0 test(s) |
| pipeline/oracle discordance | 3d838c2b31e553a0 | oracle-valid but not VALID | arm T, target PARK-7~1; pipeline class COLLATERAL; oracle oracle-valid [7]; differential DIFFERS; killed by 0 test(s) |
| pipeline/oracle discordance | 98fb506e096a919d | VALID but not oracle-valid | arm T, target PARK-3~1; pipeline class VALID; oracle oracle-collateral [3, 4]; differential DIFFERS; killed by 3 test(s) |
| pipeline/oracle discordance | efa320a48ffb6865 | VALID but not oracle-valid | arm T, target PARK-1~1; pipeline class VALID; oracle silent-equivalent-probable []; differential NO_DIFF_FOUND; killed by 0 test(s) |
| pipeline/oracle discordance | fe1923621a1f2582 | VALID but not oracle-valid | arm T, target PARK-9~1; pipeline class VALID; oracle silent-equivalent-probable []; differential NO_DIFF_FOUND; killed by 2 test(s) |

## 10. Cost

| group | criterion | calls | Claude USD | Claude tokens | codex tokens |
|---|---|---|---|---|---|
| overhead |  | 700 | 15.472 | 2580417 | 337244 |
| production | PARK-1 | 35 | 0.485 | 125159 | 53044 |
| production | PARK-2 | 30 | 0.389 | 98858 | 35153 |
| production | PARK-3 | 39 | 0.549 | 142799 | 60967 |
| production | PARK-4 | 35 | 0.507 | 127618 | 53464 |
| production | PARK-5 | 38 | 0.504 | 135518 | 60113 |
| production | PARK-6 | 36 | 0.513 | 130454 | 53175 |
| production | PARK-7 | 30 | 0.401 | 108585 | 53092 |
| production | PARK-8 | 33 | 0.470 | 117628 | 43937 |
| production | PARK-9 | 40 | 0.497 | 137446 | 62631 |
| production | PARK-10 | 31 | 0.422 | 110471 | 47083 |

| stage | seconds | cpu seconds |
|---|---|---|
| alignment | 529.756 |  |
| audit | 366.698 |  |
| blindspot-arm-o | 0.056 |  |
| blindspot-generate | 219.494 |  |
| blindspot-kills | 269.706 |  |
| blindspot-process | 83.877 |  |
| blindspot-writers | 132.191 |  |
| canary | 82.005 |  |
| generate | 339.152 |  |
| import-run-01 | 0.059 |  |
| luna-calls | 149.521 |  |
| luna-canary | 29.795 |  |
| luna-kills | 58.376 |  |
| luna-process | 4.479 |  |
| probe | 302.051 |  |
| process | 30.321 |  |
| proofs | 0.073 |  |
| reference | 0.494 |  |
| relabel | 0.003 |  |
| secondary-alignment | 1034.416 |  |
| traceability | 0.003 |  |
| validity | 621.654 |  |
| total wall clock | 4254.180 |  |

## 11. Sensitivity analyses

Variants: CONSENSUS, S1, label-based truth, G3-prereg, G3-noiso, G3-strict (not an admissible product policy), G3-lenient, J-X. Each line gives the variant figure and the PRIMARY figure it is compared with.

- S1, D1: S1 0.000 [0.000, 0.561] n = 3 (GO); PRIMARY 0.000 [0.000, 0.561] n = 3 (GO)
- S1, D2: S1 0.500 [0.095, 0.905] n = 2 (NO-GO); PRIMARY 0.500 [0.095, 0.905] n = 2 (NO-GO)
- S1, D3: S1 0.000 [0.000, 0.032] n = 118 (GO); PRIMARY 0.000 [0.000, 0.032] n = 118 (GO)
- S1, D4: S1 0.500 [0.000, 1.000] n = 16 (INCONCLUSIVE); PRIMARY 0.500 [0.000, 1.000] n = 16 (INCONCLUSIVE)
- S1, D5: S1 0.060 [0.000, 0.179] n = 134 (GO); PRIMARY 0.060 [0.000, 0.179] n = 134 (GO)
- G3-prereg, D1: G3-prereg 0.000 [0.000, 0.561] n = 3 (GO); PRIMARY 0.000 [0.000, 0.561] n = 3 (GO)
- G3-prereg, D2: G3-prereg 0.500 [0.095, 0.905] n = 2 (NO-GO); PRIMARY 0.500 [0.095, 0.905] n = 2 (NO-GO)
- G3-prereg, D3: G3-prereg 0.000 [0.000, 0.032] n = 118 (GO); PRIMARY 0.000 [0.000, 0.032] n = 118 (GO)
- G3-prereg, D4: G3-prereg 0.500 [0.000, 1.000] n = 16 (INCONCLUSIVE); PRIMARY 0.500 [0.000, 1.000] n = 16 (INCONCLUSIVE)
- G3-prereg, D5: G3-prereg 0.060 [0.000, 0.179] n = 134 (GO); PRIMARY 0.060 [0.000, 0.179] n = 134 (GO)
- G3-noiso, D1: G3-noiso 0.000 [0.000, 0.561] n = 3 (GO); PRIMARY 0.000 [0.000, 0.561] n = 3 (GO)
- G3-noiso, D2: G3-noiso 0.500 [0.095, 0.905] n = 2 (NO-GO); PRIMARY 0.500 [0.095, 0.905] n = 2 (NO-GO)
- G3-noiso, D3: G3-noiso 0.000 [0.000, 0.032] n = 118 (GO); PRIMARY 0.000 [0.000, 0.032] n = 118 (GO)
- G3-noiso, D4: G3-noiso 0.500 [0.000, 1.000] n = 16 (INCONCLUSIVE); PRIMARY 0.500 [0.000, 1.000] n = 16 (INCONCLUSIVE)
- G3-noiso, D5: G3-noiso 0.060 [0.000, 0.179] n = 134 (GO); PRIMARY 0.060 [0.000, 0.179] n = 134 (GO)
- label-based truth, D1: label-based truth 0.000 [0.000, 0.561] n = 3 (GO); PRIMARY 0.000 [0.000, 0.561] n = 3 (GO)
- label-based truth, D2: label-based truth 0.857 [0.487, 0.974] n = 7 (NO-GO); PRIMARY 0.500 [0.095, 0.905] n = 2 (NO-GO)
- label-based truth, D3: label-based truth 0.000 [0.000, 0.069] n = 52 (GO); PRIMARY 0.000 [0.000, 0.032] n = 118 (GO)
- label-based truth, D4: label-based truth 0.902 [0.692, 1.000] n = 82 (NO-GO); PRIMARY 0.500 [0.000, 1.000] n = 16 (INCONCLUSIVE)
- label-based truth, D5: label-based truth 0.060 [0.000, 0.179] n = 134 (GO); PRIMARY 0.060 [0.000, 0.179] n = 134 (GO)
- G3-strict (not an admissible product policy), D1: G3-strict (not an admissible product policy) 0.000 [0.000, 0.561] n = 3 (GO); PRIMARY 0.000 [0.000, 0.561] n = 3 (GO)
- G3-strict (not an admissible product policy), D2: G3-strict (not an admissible product policy) 1.000 [0.342, 1.000] n = 2 (NO-GO); PRIMARY 0.500 [0.095, 0.905] n = 2 (NO-GO)
- G3-strict (not an admissible product policy), D3: G3-strict (not an admissible product policy) 0.000 [0.000, 0.032] n = 118 (GO); PRIMARY 0.000 [0.000, 0.032] n = 118 (GO)
- G3-strict (not an admissible product policy), D4: G3-strict (not an admissible product policy) 1.000 [0.806, 1.000] n = 16 (NO-GO); PRIMARY 0.500 [0.000, 1.000] n = 16 (INCONCLUSIVE)
- G3-strict (not an admissible product policy), D5: G3-strict (not an admissible product policy) 0.000 [0.000, 0.028] n = 134 (GO); PRIMARY 0.060 [0.000, 0.179] n = 134 (GO)
- G3-lenient, D1: G3-lenient 0.000 [0.000, 0.561] n = 3 (GO); PRIMARY 0.000 [0.000, 0.561] n = 3 (GO)
- G3-lenient, D2: G3-lenient 0.500 [0.095, 0.905] n = 2 (NO-GO); PRIMARY 0.500 [0.095, 0.905] n = 2 (NO-GO)
- G3-lenient, D3: G3-lenient 0.000 [0.000, 0.032] n = 118 (GO); PRIMARY 0.000 [0.000, 0.032] n = 118 (GO)
- G3-lenient, D4: G3-lenient 0.500 [0.000, 1.000] n = 16 (INCONCLUSIVE); PRIMARY 0.500 [0.000, 1.000] n = 16 (INCONCLUSIVE)
- G3-lenient, D5: G3-lenient 0.000 [0.000, 0.028] n = 134 (GO); PRIMARY 0.060 [0.000, 0.179] n = 134 (GO)
- J-X, D1: J-X 0.000 [0.000, 0.561] n = 3 (GO); PRIMARY 0.000 [0.000, 0.561] n = 3 (GO)
- J-X, D2: J-X 0.500 [0.095, 0.905] n = 2 (NO-GO); PRIMARY 0.500 [0.095, 0.905] n = 2 (NO-GO)
- J-X, D3: J-X 0.000 [0.000, 0.032] n = 118 (GO); PRIMARY 0.000 [0.000, 0.032] n = 118 (GO)
- J-X, D4: J-X 0.500 [0.000, 1.000] n = 16 (INCONCLUSIVE); PRIMARY 0.500 [0.000, 1.000] n = 16 (INCONCLUSIVE)
- J-X, D5: J-X 0.060 [0.000, 0.179] n = 134 (GO); PRIMARY 0.060 [0.000, 0.179] n = 134 (GO)
- CONSENSUS, D9: CONSENSUS 0.905 [0.711, 0.973] n = 33 (GO); PRIMARY 0.833 [0.641, 0.933] n = 38 (GO)
- CONSENSUS, D10: CONSENSUS 0.783 [0.560, 0.907] n = 33 (INCONCLUSIVE); PRIMARY 0.765 [0.588, 0.923] n = 38 (INCONCLUSIVE)
- CONSENSUS, D11: CONSENSUS 0.421 [0.154, 0.659] n = 31 (INCONCLUSIVE); PRIMARY 0.289 [0.074, 0.525] n = 36 (INCONCLUSIVE)
- S1, D9: S1 0.880 [0.700, 0.958] n = 38 (GO); PRIMARY 0.833 [0.641, 0.933] n = 38 (GO)
- S1, D11: S1 0.388 [0.140, 0.681] n = 33 (INCONCLUSIVE); PRIMARY 0.289 [0.074, 0.525] n = 36 (INCONCLUSIVE)
- J-X, D9: J-X 0.833 [0.641, 0.933] n = 38 (GO); PRIMARY 0.833 [0.641, 0.933] n = 38 (GO)
- J-X, D11: J-X 0.254 [0.085, 0.423] n = 38 (INCONCLUSIVE); PRIMARY 0.289 [0.074, 0.525] n = 36 (INCONCLUSIVE)

## 12. Limits

Limits: one language; one domain; 38 tests and 10 criteria; author-assigned labels; ground truth relative to the violation set and the oracle; relabels and audits done by models; one model executes every product role; the contrast model shares its family; the fixture, its tests and labels were authored in a session of the contrast model; the oracle's author model is not recorded; unadjusted intervals.

## 13. Discussion (exploratory)

No exploratory discussion has been written for this run.
