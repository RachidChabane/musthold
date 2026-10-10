# Benchmark sample report

Sample: 6 tasks from 6 repositories (12 trunks); every figure is descriptive, with 95 % percentile cluster-bootstrap intervals by repository.

## Harness claude-code

Sample: 6 tasks, 6 repositories, 6 trunks (6 reached S_T), harness model claude-sonnet-5-5; descriptive.

On 6 real tasks from 6 repositories (6 trunks, 2 languages, harness claude-code, model claude-sonnet-5-5), changes accepted by all proofs had X = 0.600 [-0.500, 1.000] fewer defects detected by the projects' own tests than changes accepted by tests only, for 46.894 [28.393, 134.933] extra minutes and 39.333 [18.800, 139.000] extra model calls per accepted change; the gate rejected 1.000 [1.000, 1.000] of changes those tests found correct.

### Arms

| Arm | Accepted | X in the loop | X lenient | X strict | X_sel | Yield | False rejection | Escaped defects |
|---|---|---|---|---|---|---|---|---|
| AL | 5 | 0.280 [0.000, 0.760] | 0.280 [0.000, 0.760] | 0.280 [0.000, 0.760] | 0.100 [0.000, 0.500] | 0.833 [0.500, 1.000] | 0.000 [0.000, 0.000] | 0.500 [0.167, 0.833] |
| ALL | 3 | 0.600 [-0.500, 1.000] | 0.600 [-0.500, 1.000] | 0.600 [-0.500, 1.000] | -0.200 [-1.000, 0.000] | 0.500 [0.167, 0.833] | 1.000 [1.000, 1.000] | 0.167 [0.000, 0.500] |
| ALL-native | 5 | 0.280 [-0.500, 0.833] | 0.280 [-0.500, 0.833] | 0.280 [-0.500, 0.833] | -0.200 [-1.000, 0.000] | 0.833 [0.500, 1.000] | 1.000 [1.000, 1.000] | 0.500 [0.167, 0.833] |
| CO | 6 | 0.400 [0.000, 0.833] | 0.400 [0.000, 0.833] | 0.400 [0.000, 0.833] | 0.100 [0.000, 0.500] | 1.000 [1.000, 1.000] | 0.000 [0.000, 0.000] | 0.500 [0.167, 0.833] |
| DP | 4 | 0.100 [0.000, 0.500] | 0.100 [0.000, 0.500] | 0.100 [0.000, 0.500] | 0.100 [0.000, 0.500] | 0.667 [0.333, 1.000] | 0.000 [0.000, 0.000] | 0.500 [0.167, 0.833] |
| PL | 5 | 0.520 [0.000, 1.000] | 0.520 [0.000, 1.000] | 0.520 [0.000, 1.000] | 0.200 [0.000, 1.000] | 0.833 [0.500, 1.000] | 0.000 [0.000, 0.000] | 0.333 [0.000, 0.667] |
| T | 6 | 0.000 [0.000, 0.000] | 0.000 [0.000, 0.000] | 0.000 [0.000, 0.000] | 0.000 [0.000, 0.000] | 1.000 [1.000, 1.000] | 0.000 [0.000, 0.000] | 0.833 [0.500, 1.000] |
| TR | 6 | 0.000 [0.000, 0.000] | 0.000 [0.000, 0.000] | 0.000 [0.000, 0.000] | 0.000 [0.000, 0.000] | 1.000 [1.000, 1.000] | 0.000 [0.000, 0.000] | 0.833 [0.500, 1.000] |

### H-inclusive truth

| Arm | X on U, H and R | X on U, H and R without exposed criteria |
|---|---|---|
| AL | 0.280 [0.000, 0.760] | 0.280 [0.000, 0.760] |
| ALL | 0.600 [-0.500, 1.000] | 0.600 [-0.500, 1.000] |
| ALL-native | 0.280 [-0.500, 0.833] | 0.280 [-0.500, 0.833] |
| CO | 0.200 [-0.667, 0.667] | 0.400 [0.000, 0.833] |
| DP | 0.100 [0.000, 0.500] | 0.100 [0.000, 0.500] |
| PL | 0.280 [0.000, 0.760] | 0.520 [0.000, 1.000] |
| T | 0.000 [0.000, 0.000] | 0.000 [0.000, 0.000] |
| TR | 0.000 [0.000, 0.000] | 0.000 [0.000, 0.000] |

### Per-module ladder and placebo

| Arm | X in the loop |
|---|---|
| TR | 0.000 [0.000, 0.000] |
| AL | 0.280 [0.000, 0.760] |
| DP | 0.100 [0.000, 0.500] |
| CO | 0.400 [0.000, 0.833] |
| ALL minus PL | 0.080 [-1.500, 1.000] |

### Composition check

| Module | Criteria | Mismatches |
|---|---|---|
| alignment | 12 | 1/12 [0.015, 0.354] |
| discriminating-power | 12 | 1/12 [0.015, 0.354] |

### Absent-module probes

| Arm and module | Probes | Counted as passed |
|---|---|---|
| AL|discriminating-power | 6 | 0 |
| TR|alignment | 6 | 0 |

### Blind spots on agent-written tests

| Mutant source | Tasks | SB_coder |
|---|---|---|
| generator | 6 | 0.156 [-0.028, 0.375] |
| opus | 6 | 0.013 [-0.176, 0.192] |

### Same-model judge replay (6 snapshots)

| Metric | Same model | Replay | Delta |
|---|---|---|---|
| sensitivity | 0.800 [0.400, 1.000] | 1.000 [1.000, 1.000] | 0.200 [0.000, 0.600] |
| specificity | 0.000 [0.000, 0.000] | 0.000 [0.000, 0.000] | 0.000 [0.000, 0.000] |
| false_rejection | 1.000 [1.000, 1.000] | 1.000 [1.000, 1.000] | 0.000 [0.000, 0.000] |
| x_sel | -0.200 [-1.000, 0.000] | n/a | n/a |

### System One calibration per confidence source

| Source | Decisions | ECE | Brier | Coverage at 5 % error |
|---|---|---|---|---|
| endpoint | 22 | 0.275 [0.187, 0.473] | 0.244 [0.142, 0.345] | 0.091 [0.000, 0.471] |
| host-votes | 42 | 0.204 [0.125, 0.307] | 0.215 [0.158, 0.284] | 0.000 [0.000, 0.278] |
| undetermined rate | 72 | 0.111 [0.056, 0.153] |  |  |

## Harness codex

Sample: 6 tasks, 6 repositories, 6 trunks (6 reached S_T), harness model gpt-6-luna; descriptive.

On 6 real tasks from 6 repositories (6 trunks, 2 languages, harness codex, model gpt-6-luna), changes accepted by all proofs had X = 1.000 [1.000, 1.000] fewer defects detected by the projects' own tests than changes accepted by tests only, for 29.557 [21.697, 46.983] extra minutes and 24.800 [18.167, 37.333] extra model calls per accepted change; the gate rejected 0.333 [0.000, 1.000] of changes those tests found correct.

### Arms

| Arm | Accepted | X in the loop | X lenient | X strict | X_sel | Yield | False rejection | Escaped defects |
|---|---|---|---|---|---|---|---|---|
| AL | 6 | 1.000 [1.000, 1.000] | 1.000 [1.000, 1.000] | 1.000 [1.000, 1.000] | 1.000 [1.000, 1.000] | 1.000 [1.000, 1.000] | 0.000 [0.000, 0.000] | 0.000 [0.000, 0.000] |
| ALL | 5 | 1.000 [1.000, 1.000] | 1.000 [1.000, 1.000] | 1.000 [1.000, 1.000] | 1.000 [1.000, 1.000] | 0.833 [0.500, 1.000] | 0.333 [0.000, 1.000] | 0.000 [0.000, 0.000] |
| ALL-native | 3 | 0.333 [-2.000, 1.000] | 0.200 [-1.000, 1.000] | 0.333 [-2.000, 1.000] | 0.000 [-2.000, 1.000] | 0.500 [0.167, 0.833] | 0.667 [0.000, 1.000] | 0.167 [0.000, 0.500] |
| CO | 5 | 0.200 [-1.000, 1.000] | 0.200 [-1.000, 1.000] | 0.200 [-1.000, 1.000] | 0.000 [-1.000, 1.000] | 0.833 [0.500, 1.000] | 0.333 [0.000, 1.000] | 0.333 [0.000, 0.667] |
| DP | 6 | 0.333 [0.000, 1.000] | 0.333 [0.000, 1.000] | 0.333 [0.000, 1.000] | 0.200 [0.000, 1.000] | 1.000 [1.000, 1.000] | 0.000 [0.000, 0.000] | 0.333 [0.000, 0.667] |
| PL | 6 | 0.333 [0.000, 1.000] | 0.333 [0.000, 1.000] | 0.333 [0.000, 1.000] | 0.200 [0.000, 1.000] | 1.000 [1.000, 1.000] | 0.000 [0.000, 0.000] | 0.333 [0.000, 0.667] |
| T | 6 | 0.000 [0.000, 0.000] | 0.000 [0.000, 0.000] | 0.000 [0.000, 0.000] | 0.000 [0.000, 0.000] | 1.000 [1.000, 1.000] | 0.000 [0.000, 0.000] | 0.500 [0.167, 0.833] |
| TR | 3 | -0.333 [-2.000, 1.000] | -0.333 [-2.000, 1.000] | -0.333 [-2.000, 1.000] | -0.333 [-2.000, 1.000] | 0.500 [0.167, 0.833] | 0.667 [0.000, 1.000] | 0.333 [0.000, 0.667] |

### H-inclusive truth

| Arm | X on U, H and R | X on U, H and R without exposed criteria |
|---|---|---|
| AL | 0.667 [0.000, 1.000] | 1.000 [1.000, 1.000] |
| ALL | 1.000 [1.000, 1.000] | 1.000 [1.000, 1.000] |
| ALL-native | -0.333 [-5.000, 1.000] | 0.333 [-2.000, 1.000] |
| CO | -0.200 [-3.000, 0.700] | 0.200 [-1.000, 1.000] |
| DP | 0.000 [-2.000, 0.750] | 0.333 [0.000, 1.000] |
| PL | 0.000 [-2.000, 0.750] | 0.333 [0.000, 1.000] |
| T | -0.333 [-2.000, 0.000] | 0.000 [0.000, 0.000] |
| TR | -0.333 [-2.000, 1.000] | -0.333 [-2.000, 1.000] |

### Per-module ladder and placebo

| Arm | X in the loop |
|---|---|
| TR | -0.333 [-2.000, 1.000] |
| AL | 1.000 [1.000, 1.000] |
| DP | 0.333 [0.000, 1.000] |
| CO | 0.200 [-1.000, 1.000] |
| ALL minus PL | 0.667 [0.000, 1.000] |

### Composition check

| Module | Criteria | Mismatches |
|---|---|---|
| alignment | 12 | 0/12 [0.000, 0.242] |
| discriminating-power | 12 | 0/12 [0.000, 0.242] |

### Absent-module probes

| Arm and module | Probes | Counted as passed |
|---|---|---|
| AL|discriminating-power | 6 | 0 |
| TR|alignment | 6 | 0 |

### Blind spots on agent-written tests

| Mutant source | Tasks | SB_coder |
|---|---|---|
| generator | 6 | 0.015 [-0.217, 0.308] |
| opus | 6 | -0.238 [-0.540, 0.215] |

### Same-model judge replay (6 snapshots)

| Metric | Same model | Replay | Delta |
|---|---|---|---|
| sensitivity | 0.333 [0.000, 1.000] | 0.667 [0.000, 1.000] | 0.333 [0.000, 1.000] |
| specificity | 1.000 [1.000, 1.000] | 0.333 [0.000, 1.000] | -0.667 [-1.000, 0.000] |
| false_rejection | 0.000 [0.000, 0.000] | 0.667 [0.000, 1.000] | 0.667 [0.000, 1.000] |
| x_sel | 0.200 [0.000, 1.000] | 0.000 [-2.000, 1.000] | -0.200 [-2.000, 1.000] |

### System One calibration per confidence source

| Source | Decisions | ECE | Brier | Coverage at 5 % error |
|---|---|---|---|---|
| endpoint | 35 | 0.237 [0.121, 0.347] | 0.218 [0.142, 0.283] | 0.000 [0.000, 0.138] |
| host-votes | 32 | 0.109 [0.083, 0.270] | 0.181 [0.110, 0.264] | 0.000 [0.000, 0.636] |
| undetermined rate | 72 | 0.069 [0.014, 0.125] |  |  |

## Harness by arm interaction

Sample: the tasks both harnesses ran, claude-code minus codex; descriptive.

| Arm | Difference in X in the loop |
|---|---|
| AL | -0.720 [-1.000, -0.240] |
| ALL | -0.400 [-1.500, 0.000] |
| ALL-native | -0.053 [-1.125, 1.800] |
| CO | 0.200 [-0.833, 1.167] |
| DP | -0.233 [-1.000, 0.400] |
| PL | 0.187 [-0.500, 0.800] |
| T | 0.000 [0.000, 0.000] |
| TR | 0.333 [-1.000, 2.000] |
