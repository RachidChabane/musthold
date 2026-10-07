"""Typed constants of the analysis (PREREG 6.6 to 13); no other module spells these literals."""

from __future__ import annotations

from typing import Final

from pilot.domain.arms import ARM_O, ARM_P, ARM_T, ARMS
from pilot.domain.constants import LOCAL_SUBPROCESS_LIMIT
from pilot.domain.model_slate import Judge
from pilot.domain.mutant_classes import CriterionClass
from pilot.domain.endpoint_ids import ENDPOINT_IDS as DECISIONAL_ENDPOINT_IDS

ANALYSIS_SEED: Final = 20261006
BOOTSTRAP_RESAMPLES: Final = 10_000
PERMUTATION_COUNT: Final = 10_000
CONFIDENCE_LEVEL: Final = 0.95
WILSON_ALPHA: Final = 1.0 - CONFIDENCE_LEVEL
NULL_PERCENTILE: Final = 95.0
BINOMIAL_ALPHA: Final = 0.05

THETA_OWN: Final = 0.50
THETA_OTHER: Final = 0.20

CRITERION_FORMAT: Final = "PARK-{number}~1"
KILLED_RESULT: Final = "killed"
NONE_ANSWER: Final = "NONE"

KIND_REGRESSION: Final = "r"
KIND_NEUTRAL: Final = "n1"

MUTANT_VALID_W: Final = CriterionClass.VALID_W.value
MUTANT_NOT_VIOLATING: Final = CriterionClass.NOT_VIOLATING.value
MUTANT_UNDETERMINED: Final = CriterionClass.UNDETERMINED.value
STATUS_OK: Final = "OK"
STATUS_TRIVIALLY_EQUIVALENT: Final = "TRIVIALLY_EQUIVALENT"
STATUS_DUPLICATE: Final = "DUPLICATE"
PROCESSING_STATUSES: Final = (STATUS_OK, STATUS_TRIVIALLY_EQUIVALENT, STATUS_DUPLICATE)
ERROR_RESULT: Final = "error"
KILL_RESULTS: Final = (KILLED_RESULT, "survived", ERROR_RESULT, "flaky")
TEST_OUTCOMES: Final = ("passed", "failed", "error", "skipped")
CLASS_VERDICTS: Final = (MUTANT_VALID_W, MUTANT_NOT_VIOLATING, MUTANT_UNDETERMINED)
CALL_OK: Final = "ok"
CALL_MISSING: Final = "missing"
CALL_STATUSES: Final = (CALL_OK, CALL_MISSING)
DIFF_DIFFERS: Final = "DIFFERS"
DIFF_NO_DIFF: Final = "NO_DIFF_FOUND"
DIFF_SKIPPED: Final = "SKIPPED"
DIFF_STATUSES: Final = (DIFF_DIFFERS, DIFF_NO_DIFF, DIFF_SKIPPED)

ALIGNMENT_ARM_MASKED: Final = "NAME-MASKED"
ALIGNMENT_ARM_SHOWN: Final = "NAME-SHOWN"
JUDGE_H: Final = Judge.J_H.value
JUDGE_X: Final = Judge.J_X.value
VALIDITY_JUDGE_H1: Final = Judge.V_H1.value
VALIDITY_JUDGE_H2: Final = Judge.V_H2.value
AUDIT_JUDGE: Final = Judge.A_OPUS.value
RATER_OPENAI: Final = Judge.R_OAI.value
RATER_AUTHOR: Final = Judge.R_AUTHOR.value
PROBE_TARGETED: Final = "TARGETED"
PROBE_OTHER: Final = "OTHER"
TRUTH_ALIGNED: Final = "ALIGNED"
TRUTH_NOT_ALIGNED: Final = "NOT_ALIGNED"

MODAL_SHARE_THRESHOLD: Final = 0.8
EXPECTED_ALIGNMENT_REPS: Final = 5
EXPECTED_PROBE_REPS: Final = 3

AUDIT_STRATA_SAMPLE_SIZE: Final = 20
AUDIT_STRATUM_FLOOR: Final = 3
AUDIT_STRATUM_SEPARATOR: Final = "|"
AUDIT_STRATUM_ALWAYS_SURVIVED: Final = "survived-all-valid"
AUDIT_STRATUM_PLANTED: Final = "planted"
AUDIT_PIPELINE_VALID: Final = "pipeline-valid"
AUDIT_PIPELINE_OTHER: Final = "pipeline-not-valid"
AUDIT_ORACLE_VALID: Final = "oracle-valid"
AUDIT_ORACLE_OTHER: Final = "oracle-not-valid"

ORACLE_CLASSES_FILENAME: Final = "oracle_classes.json"
ORACLE_PARALLELISM: Final = LOCAL_SUBPROCESS_LIMIT
ORACLE_TESTS_DIRNAME: Final = "tests"

# Scenario universe and truth.
EXPECTED_SCENARIO_COUNT: Final = 134
UNCOVERED_CRITERIA: Final = (6, 7, 10)
POISONED_CRITERIA: Final = (3, 5, 8)
CLEAN_CRITERIA: Final = (1, 2, 4, 9)

# Endpoint identifiers, in report order.
ENDPOINT_IDS: Final = DECISIONAL_ENDPOINT_IDS
OUTCOME_GO: Final = "GO"
OUTCOME_NO_GO: Final = "NO-GO"
OUTCOME_INCONCLUSIVE: Final = "INCONCLUSIVE"

# Bands (PREREG 10), as exact rationals where the protocol states a fraction of n.
D2_GO_FRACTION_PERCENT: Final = 15
D2_NO_GO_FRACTION_PERCENT: Final = 50
D3_GO_POINT: Final = 0.05
D3_GO_UPPER: Final = 0.20
D3_NO_GO_LOWER: Final = 0.20
D4_GO_POINT: Final = 0.10
D4_GO_UPPER: Final = 0.25
D4_NO_GO_LOWER: Final = 0.25
D5_GO_POINT: Final = 0.30
D5_GO_UPPER: Final = 0.45
D5_NO_GO_LOWER: Final = 0.50
D6_GO_POINT: Final = 0.80
D6_GO_LOWER: Final = 0.70
D6_GO_MARGIN_OVER_N7: Final = 0.10
D6_N7_WAIVER: Final = 0.90
D6_NO_GO_UPPER: Final = 0.70
D7_GO_REGRESSION_MIN: Final = 16
D7_GO_NEUTRAL_MAX: Final = 1
D7_NO_GO_REGRESSION_MAX: Final = 10
D7_NO_GO_NEUTRAL_MIN: Final = 3
D8_NO_GO_SHORT: Final = 3
D8_ALL_GO_CRITERIA: Final = 10
D8_ALL_NO_GO_CRITERIA_MAX: Final = 7
EXPOSED_CRITERIA: Final = (5, 8)
PREDICTED_ISOLATION_EXPOSURE: Final = (3, 5, 8)
D8_MIN_VALID_PER_CRITERION: Final = 2
D9_GO_POINT: Final = 0.80
D9_GO_LOWER: Final = 0.60
D9_GO_MARGIN_OVER_N2: Final = 0.10
D9_NO_GO_UPPER: Final = 0.70
D10_GO_POINT: Final = 0.80
D10_GO_LOWER: Final = 0.65
D10_NO_GO_UPPER: Final = 0.70
D11_GO_POINT: Final = 0.60
D11_GO_LOWER: Final = 0.30
D11_NO_GO_UPPER: Final = 0.40

# Preconditions.
P2_MIN_POINT: Final = 0.60
P2_MIN_LOWER: Final = 0.40
P5_MAX_MISSING_RATE: Final = 0.10
PRECONDITION_IDS: Final = tuple(f"P{n}" for n in range(1, 8))
GATE_ENDPOINTS: Final = ("D1", "D2", "D3", "D4", "D5")
VALIDITY_ENDPOINTS: Final = ("D6", "D7", "D8")
JUDGE_ENDPOINTS: Final = ("D9",)
MATRIX_ENDPOINTS: Final = ("D10",)
CROSS_CHECK_ENDPOINTS: Final = ("D11",)

# Composite components (fallback map of PREREG 10).
COMPONENT_GATE: Final = "gate"
COMPONENT_VALIDITY: Final = "mutant validity"
COMPONENT_ISOLATION: Final = "isolation"
COMPONENT_JUDGE: Final = "alignment judge"
COMPONENT_MATRIX: Final = "matrix"
COMPONENT_CROSS_CHECK: Final = "cross-check"
FALLBACK_COMPONENT: Final = {
    **{endpoint: COMPONENT_GATE for endpoint in GATE_ENDPOINTS},
    **{endpoint: COMPONENT_VALIDITY for endpoint in VALIDITY_ENDPOINTS},
    "D9": COMPONENT_JUDGE,
    "D10": COMPONENT_MATRIX,
    "D11": COMPONENT_CROSS_CHECK,
}
COMPOSITE_PROCEED: Final = "PROCEED_TO_POWERED_STUDY"
COMPOSITE_REDESIGN: Final = "REDESIGN"
COMPOSITE_INCONCLUSIVE: Final = "INCONCLUSIVE"
N_NEEDED_UNREACHABLE: Final = "unreachable"
N_NEEDED_BLOCKED: Final = "blocked"
N_NEEDED_SEARCH_LIMIT: Final = 100_000

# Blind-spot estimand (PREREG 11).
BLIND_SPOT_MATERIALITY: Final = 0.10
BLIND_SPOT_MIN_CRITERIA: Final = 3
BLIND_SPOT_INDICATED: Final = "shared blind spot indicated"
BLIND_SPOT_NONE: Final = "no material shared blind spot"
BLIND_SPOT_UNDETERMINED: Final = "undetermined"

# Feasibility (PREREG 12).
FEASIBILITY_GREEN: Final = "green"
FEASIBILITY_AMBER: Final = "amber"
FEASIBILITY_RED: Final = "red"
F1_GREEN_MIN: Final = 0.95
F1_AMBER_MIN: Final = 0.85
F3_GREEN_MAX: Final = 0.05
F3_AMBER_MAX: Final = 0.15
F4_GREEN_MAX: Final = 0.20
F4_AMBER_MAX: Final = 0.40
F5_GREEN: Final = 10
F5_AMBER_MIN: Final = 8
F7_GREEN_MAX_USD: Final = 5.0
F7_AMBER_MAX_USD: Final = 15.0
F8_GREEN_MAX_HOURS: Final = 12.0
F8_AMBER_MAX_HOURS: Final = 36.0
SECONDS_PER_HOUR: Final = 3600.0
VERDICT_PROTOCOL_READY: Final = "PROTOCOL_READY"
VERDICT_AMEND: Final = "AMEND"
VERDICT_STOP: Final = "STOP_AND_REDESIGN_PROTOCOL"

# Files.
RESULTS_DIRNAME: Final = "results"
DECISIONS_FILENAME: Final = "decisions.json"
ENDPOINTS_FILENAME: Final = "endpoints.json"
REPORT_FILENAME: Final = "REPORT.md"
REPORT_TEMPLATE_FILENAME: Final = "REPORT.tmpl.md"
PRECISION_TABLE_FILENAME: Final = "precision-table.csv"
CROSSCHECK_CACHE_FILENAME: Final = "runner_crosscheck.json"
ARM_P_LABELS_YAML: Final = "labels.yaml"

# Report lint (PREREG 13).
FORBIDDEN_WORDS: Final = (
    "works",
    "proves",
    "proven",
    "validated",
    "confirm",
    "human audit",
    "generalizes",
)
QUOTED_BEGIN_MARKER: Final = "<!-- quoted:begin -->"
QUOTED_END_MARKER: Final = "<!-- quoted:end -->"
CONSENSUS_WORD: Final = "CONSENSUS"
PRIMARY_WORD: Final = "PRIMARY"

# Precision table (PREREG 13 and the precision-table verb).
PRECISION_DENOMINATORS: Final = (3, 7, 10, 14, 20, 24, 30, 38, 52, 82)
PRECISION_PROPORTIONS: Final = (0.0, 0.5, 0.8, 1.0)
PRECISION_FULL_COUNT_DENOMINATORS: Final = (3, 7, 14, 24, 38)

# Preregistration drift check.
DRIFT_GUARDED_PATHS: Final = (
    "prereg",
    "fixture",
    "harness/src/pilot/analysis",
    "harness/src/pilot/domain/gate.py",
    "harness/src/pilot/domain/matrix.py",
    "harness/src/pilot/domain/analysis_constants.py",
    "harness/src/pilot/domain/deviations.py",
    "harness/src/pilot/domain/endpoint_ids.py",
    "harness/src/pilot/domain/mutant_classes.py",
    "harness/src/pilot/domain/model_slate.py",
    "harness/src/pilot/domain/execution_validation.py",
    "harness/src/pilot/application/blind_spot_stage.py",
    "runs/run-01",
    "freeze/prereg-commit.*",
    "freeze/README.md",
)
EXPLORATORY_WORD_LIMIT: Final = 300
EXPLORATORY_FILENAME: Final = "EXPLORATORY.md"
REPORT_CONCLUSION_BEGIN: Final = "<!-- conclusion-templates:begin -->"
REPORT_CONCLUSION_END: Final = "<!-- conclusion-templates:end -->"
