"""Typed constants of the pipeline verbs (PREREG 4 to 7); ground-truth names live elsewhere."""

from __future__ import annotations

from typing import Final

from pilot.domain.constants import LOCAL_SUBPROCESS_LIMIT

TAU: Final = 0.70
WITNESS_TIMEOUT_SECONDS: Final = 5
AUDIT_SAMPLE_FILENAME: Final = "audit_sample.json"

GENERATION_CALLS_CLAUDE: Final = 3
GENERATION_CALLS_CODEX: Final = 2
GENERATION_SLOTS: Final = ("S1", "S2", "S3")

ALIGNMENT_REPS: Final = 5
PROBE_REPS: Final = 3
ALIGNMENT_MODAL_SHARE_THRESHOLD: Final = 0.8
ALIGNMENT_CONFIDENCE_MAX: Final = 100
TOKEN_PREFIX: Final = "K-"
TOKEN_HEX_DIGITS: Final = 4
TOKEN_REDRAW_LIMIT: Final = 1000
HASH_FIELD_SEPARATOR: Final = "‖"
MASKED_TEST_NAME: Final = "test_case"
MASKED_HELPER_PREFIX: Final = "helper_"
NAME_RUN_FORBIDDEN_LENGTH: Final = 3
LEAK_FORBIDDEN_STRINGS: Final = ("verifies", "PARK-")

NONE_ANSWER: Final = "NONE"
RANDOM_NONCE_BYTES: Final = 16

ARM_P_DIRNAME: Final = "arm_p"
ARM_P_ITEMS_DIRNAME: Final = "items"
LEAK_AUDIT_DIRNAME: Final = "leak_audit"
LEAK_AUDIT_HINTS_FILENAME: Final = "hints.json"
CODEBOOK_FILENAME: Final = "codebook.md"
SCHEMAS_DIRNAME: Final = "schemas"
HARNESS_DIRNAME: Final = "harness"
MUTANT_ID_LENGTH: Final = 16

RUN_MANIFEST_FILENAME: Final = "manifest.json"
DEVIATIONS_FILENAME: Final = "DEVIATIONS.md"
PLAN_FILENAME: Final = "plan.json"
BINDINGS_FILENAME: Final = "bindings.json"
REFERENCE_FILENAME: Final = "reference.json"
MUTANTS_DIRNAME: Final = "mutants"
PROCESSING_FILENAME: Final = "processing.jsonl"
KILLS_FILENAME: Final = "kills.jsonl"
DIFFERENTIAL_FILENAME: Final = "differential.jsonl"
VALIDITY_FILENAME: Final = "validity.jsonl"
WITNESS_FILENAME: Final = "witness.jsonl"
MUTANT_CLASSES_FILENAME: Final = "mutant_classes.json"
ALIGNMENT_FILENAME: Final = "alignment.jsonl"
ALIGNMENT_AGGREGATE_FILENAME: Final = "alignment_aggregate.json"
PROBE_FILENAME: Final = "probe.jsonl"
RELABEL_FILENAME: Final = "relabel.jsonl"
TIMING_FILENAME: Final = "timing.json"
TOKEN_MAPS_DIRNAME: Final = "token_maps"
PROOFS_DIRNAME: Final = "proofs"
ALIGNMENT_PROOFS_DIRNAME: Final = "alignment"
DISCRIMINATING_POWER_PROOFS_DIRNAME: Final = "discriminating-power"
PROOF_SUFFIX: Final = ".intoto.json"

EXIT_QUOTA_EXHAUSTED: Final = 3

STATEMENT_TYPE: Final = "https://in-toto.io/Statement/v1"
PREDICATE_ALIGNMENT: Final = "https://rachid-chabane.com/attestations/alignment/v1"
PREDICATE_DISCRIMINATING_POWER: Final = "https://rachid-chabane.com/attestations/discriminating-power/v1"
STATEMENT_CANONICALIZATION: Final = "c1"
MODULE_NAME: Final = "pilot"
MODULE_VERSION: Final = "0.1.0"
PRODUCED_BY_LOCAL: Final = "local"
CLASS_EXECUTED: Final = "executed"
CLASS_JUDGED: Final = "judged"

PROGRESS_INTERVAL: Final = 5
MAX_WITNESS_WORKERS: Final = LOCAL_SUBPROCESS_LIMIT
