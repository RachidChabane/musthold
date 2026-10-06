"""Typed constants for every literal with meaning in the protocol (PREREG)."""

from __future__ import annotations

from typing import Final

SEED: Final = 20261006

PYTEST_TIMEOUT_SECONDS: Final = 10
PYTEST_RUN_DEADLINE_SECONDS: Final = 1800
MAX_CHANGED_LINES: Final = 10
LOCAL_SUBPROCESS_LIMIT: Final = 2
MAX_CONCURRENT_PYTEST: Final = LOCAL_SUBPROCESS_LIMIT
MAX_CONCURRENT_MODEL_CALLS: Final = 2
REFERENCE_RUN_COUNT: Final = 3
MODEL_RETRY_COUNT: Final = 1
MODEL_CALL_DEADLINE_SECONDS: Final = 1800
IMPORT_SMOKE_TIMEOUT_SECONDS: Final = 30

DIFFERENTIAL_MAX_EXAMPLES: Final = 2000
DIFFERENTIAL_MAX_DURATION_HOURS: Final = 96
DIFFERENTIAL_MIN_YEAR: Final = 2020
DIFFERENTIAL_MAX_YEAR: Final = 2030
DIFFERENTIAL_MAX_AMOUNT: Final = 1000
DIFFERENTIAL_EVAL_TIMEOUT_SECONDS: Final = 600

TOKEN_CHARS_PER_TOKEN: Final = 2.5
TOKEN_BOUND_SLACK: Final = 500

PACKAGE_NAME: Final = "parkfee"
PUBLIC_FUNCTIONS: Final = ("quote", "breakdown", "format_receipt")
SOURCE_DIRNAME: Final = "src"
TESTS_DIRNAME: Final = "tests"
SPEC_DIRNAME: Final = "spec"
PYTEST_TIMEOUT_MESSAGE_PREFIX: Final = "Failed: Timeout"
PYTEST_EXIT_INTERRUPTED: Final = 2

EXPECTED_TEST_COUNT: Final = 38
EXPECTED_VIOLATION_COUNT: Final = 30
EXPECTED_PLANTED_COUNT: Final = 10
EXPECTED_CRITERION_COUNT: Final = 10
EXPECTED_TEST_FILES: Final = (
    "test_intervals.py",
    "test_tariff.py",
    "test_night.py",
    "test_tickets.py",
    "test_receipt.py",
)

REFACTOR_VARIANT_ID: Final = "R"
REFERENCE_VARIANT_ID: Final = "REFERENCE"
CRITERION_REF_PATTERN: Final = r"^PARK-(\d+)~(\d+)$"
CRITERION_ID_PATTERN: Final = r"PARK-(\d+)"
VERIFIES_MARKER_NAME: Final = "verifies"
FIXTURE_DECORATOR_NAMES: Final = ("fixture",)
PARAMETRIZE_DECORATOR_NAMES: Final = ("parametrize",)
TEST_FUNCTION_PREFIX: Final = "test_"


CELL_PASS: Final = "pass"
CELL_FAIL: Final = "fail"

JUNIT_FILENAME: Final = "junit.xml"
TRACEABILITY_FILENAME: Final = "traceability.json"
LABEL_MATRIX_FILENAME: Final = "label_matrix.csv"
VERIFY_LOG_FILENAME: Final = "verify_log.txt"
MODELS_LOCK_FILENAME: Final = "models.lock.json"

FORBIDDEN_TRACEABILITY_FINDINGS: Final = (
    "tag_not_in_results",
    "orphan_tag",
    "malformed_tag",
    "revision_mismatch",
)
TRACEABILITY_PASS_VERDICT: Final = "pass"

CLAUDE_NESTING_ENV_VARS: Final = ("CLAUDECODE", "CLAUDE_CODE_ENTRYPOINT")
CODEX_ALLOWED_ITEM_TYPES: Final = ("agent_message", "reasoning")

PREREG_DIRNAME: Final = "prereg"
FIXTURE_DIRNAME: Final = "fixture"
RUNS_DIRNAME: Final = "runs"
PROMPTS_DIRNAME: Final = "prompts"
SYSTEM_PROMPT_FILENAME: Final = "system.txt"
RAW_DIRNAME: Final = "raw"
RESULT_FILENAME: Final = "result.json"
CANARY_DIRNAME: Final = "canary"

CANARY_ITEM: Final = 'Reply with the JSON object {"ok": true} and nothing else.'
CANARY_SCHEMA_TEXT: Final = (
    '{"type":"object","properties":{"ok":{"type":"boolean"}},'
    '"required":["ok"],"additionalProperties":false}'
)

SHALLNOT_CONFTEST_HOOK: Final = """def pytest_collection_modifyitems(items):
    for item in items:
        for marker in item.iter_markers(name="verifies"):
            item.user_properties.append(("verifies", ", ".join(marker.args)))"""
CONFTEST_FILENAME: Final = "conftest.py"
USEFIXTURES_DECORATOR_NAMES: Final = ("usefixtures",)
MODULE_MARKS_NAME: Final = "pytestmark"
