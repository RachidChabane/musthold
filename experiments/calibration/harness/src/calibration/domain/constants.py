"""Every literal with meaning of the pre-registration, declared once."""

from __future__ import annotations

from typing import Final

# Identity of the experiment.
PREREG_TAG: Final = "prereg-calibration"
PILOT_FREEZE_TAG: Final = "prereg-limits-1-3-a1"
PILOT_RUN_ID: Final = "run-02"
SEED_TEXT: Final = "20261007"
SEED: Final = 20261007
PASS2_LABEL: Final = "pass2"
PASS2_MODULUS: Final = 4
PASS2_HEX_DIGITS: Final = 8
HASH_SEPARATOR: Final = "|"

# Layout.
EXPERIMENT_DIR: Final = "experiments/calibration"
PILOT_DIR: Final = "experiments/limits-1-3"
PILOT_FIXTURE_DIR: Final = "experiments/limits-1-3/fixture"
PILOT_PREREG_DIR: Final = "experiments/limits-1-3/prereg"
PILOT_SOURCE_DIR: Final = "experiments/limits-1-3/harness/src/pilot"
SNAPSHOT_PATHS: Final = (PILOT_FIXTURE_DIR, PILOT_PREREG_DIR, PILOT_SOURCE_DIR)
PILOT_SOURCE_ROOT: Final = "harness/src"
PILOT_DECISIONS_FILE: Final = "experiments/limits-1-3/results/decisions.json"
FIXTURE_PROJECT: Final = "fixture/parkfee"
PRODUCT_EXCLUDE: Final = "experiments"
PRODUCT_MODULES: Final = ("format", "alignment", "discpower", "s1relay")
CACHE_DIR_NAME: Final = ".cache"
RUNS_DIR_NAME: Final = "runs"
RESULTS_DIR_NAME: Final = "results"
DEFAULT_RUN_ID: Final = "run-01"
ENCODING: Final = "utf-8"

# Run directory files.
MANIFEST_FILE: Final = "manifest.json"
PILOT_INPUTS_FILE: Final = "pilot_inputs.json"
SNAPSHOT_FILE: Final = "snapshot.json"
MODELS_LOCK_FILE: Final = "models.lock.json"
ITEMS_DIR: Final = "items"
EXCLUDED_FILE: Final = "excluded.json"
SCHEDULE_FILE: Final = "schedule.json"
SERVER_DIR: Final = "server"
CANARY_DIR: Final = "canary"
RAW_DIR: Final = "raw"
SAFETY_FILE: Final = "safety.jsonl"
DEVIATIONS_FILE: Final = "DEVIATIONS.md"
TIMING_FILE: Final = "timing.json"
SESSION_LOCK_FILE: Final = ".session.lock"
REQUEST_FILE: Final = "request.bin"
RESPONSE_FILE: Final = "response.bin"
META_FILE: Final = "meta.json"
SOURCE_SUMS_FILE: Final = "SHA256SUMS"

# Results files.
DECISIONS_FILE: Final = "decisions.json"
POLICY_INPUTS_FILE: Final = "policy-inputs.json"
REPORT_FILE: Final = "REPORT.md"
CHECKS_FILE: Final = "checks.csv"
RELIABILITY_DIR: Final = "reliability"
RISK_COVERAGE_DIR: Final = "risk_coverage"
COMPARISONS_DIR: Final = "comparisons"

# Pilot inputs the item construction reads (not truth).
PILOT_BINDINGS_FILE: Final = "bindings.json"
PILOT_PROCESSING_FILE: Final = "processing.jsonl"
PILOT_DIFFERENTIAL_FILE: Final = "differential.jsonl"
PILOT_MUTANTS_DIR: Final = "mutants"
PILOT_BUILD_INPUT_FILES: Final = (PILOT_BINDINGS_FILE, PILOT_PROCESSING_FILE, PILOT_DIFFERENTIAL_FILE)

# Wire.
OLLAMA_PORT: Final = 11435
KEV_PORT: Final = 8008
LOOPBACK: Final = "127.0.0.1"
OLLAMA_INSTALLED_PORT: Final = 11434
OLLAMA_VERSION: Final = "0.35.1"
OLLAMA_DEFAULT_ADDRESS: Final = "http://localhost:11434/v1/systemone"
KEV_DEFAULT_ADDRESS: Final = "http://127.0.0.1:8008/v1/systemone"
SYSTEM_ONE_ROUTE: Final = "/v1/systemone"
KEV_REPO_URL: Final = "https://github.com/jaredpalmer/kev"
KEV_COMMIT: Final = "5e42a7a03f28134853dd3ff77461457e921e5ec1"
KEV_HUB_OWNER: Final = "jaredpalmer"
REQUEST_TIMEOUT_SECONDS: Final = 300
RETRY_DELAY_SECONDS: Final = 5
MAX_ATTEMPTS: Final = 2
PROBABILITY_SUM_TOLERANCE: Final = 0.02
CONFIDENCE_TOLERANCE: Final = 0.001
HTTP_OK: Final = 200
HTTP_BAD_REQUEST: Final = 400
HTTP_PAYLOAD_TOO_LARGE: Final = 413
HTTP_UNPROCESSABLE: Final = 422
CHARS_PER_TOKEN_PROBE: Final = 4
JSON_CONTENT_TYPE: Final = "application/json"

# Machine safety and pacing.
MIN_FREE_PERCENT: Final = 40
MEM_GUARD_ENV: Final = "MEM_GUARD_MIN_FREE_PCT"
HEAVY_CHECK_EVERY: Final = 25
HEAVY_WAIT_SECONDS: Final = 300
MEMORY_STATIC_FRACTION: Final = 0.5
MEMORY_RESERVE_FRACTION: Final = 0.15
MEMORY_DEFERRAL_LIMIT: Final = 5
MEMORY_STOP_LIMIT: Final = 2
BYTES_PER_GIB: Final = 1024**3
FOOTPRINT_OVERHEAD_GIB: Final = 1
DEFAULT_RUN_WINDOW: Final = "21:00-07:30"
RUN_WINDOW_ENV: Final = "CALIBRATION_RUN_WINDOW"
ALWAYS_OPEN: Final = "always"
EXIT_OK: Final = 0
EXIT_FAILURE: Final = 1
EXIT_PRECONDITION: Final = 2
EXIT_PAUSED: Final = 3
EXIT_GUARD_REFUSED: Final = 4
EXIT_GUARD_INTERRUPTED: Final = 5
SERVER_STOP_GRACE_SECONDS: Final = 15
SERVER_START_TIMEOUT_SECONDS: Final = 600
HEAVY_PROCESS_PATTERNS: Final = ("ollama runner", "kev.serve", "llama-server", "mlx_lm")
OLLAMA_ENVIRONMENT: Final = (
    ("OLLAMA_HOST", f"{LOOPBACK}:{OLLAMA_PORT}"),
    ("OLLAMA_MAX_LOADED_MODELS", "1"),
    ("OLLAMA_NUM_PARALLEL", "1"),
    ("OLLAMA_KEEP_ALIVE", "2m"),
)
KEV_UNSET_ENVIRONMENT: Final = ("KEV_TEMPERATURE", "KEV_DTYPE", "KEV_TRUNCATE_STATES", "KEV_API_KEY")
KEV_OFFLINE_ENVIRONMENT: Final = (("HF_HUB_OFFLINE", "1"),)

# Statistics.
BOOTSTRAP_RESAMPLES: Final = 10_000
CONFIDENCE_LEVEL: Final = 0.95
WILSON_Z: Final = 1.959963984540054
ECE_BINS: Final = 10
EQUAL_MASS_BINS_K4: Final = 10
EQUAL_MASS_BINS_K13: Final = 5
Q09_LOW: Final = 0.85
Q09_HIGH: Final = 0.95
C05_RISK: Final = 0.05
C05U_UPPER: Final = 0.10
SCREEN_GRID: Final = (0.05, 0.10, 0.15, 0.20, 0.25, 0.30, 0.40, 0.50)
SCREEN_TAU_DEFAULT: Final = 0.20
MISS_UPPER_LIMIT: Final = 0.10
FLAG_POINT_LIMIT: Final = 0.50
GATE_THRESHOLD_S1: Final = 0.90
GATE_THRESHOLD_JH5: Final = 0.80
GATE_THRESHOLD_JH3: Final = 0.90
VH2_JUDGE_THRESHOLD: Final = 0.70
FALL_THROUGH_LIMIT: Final = 0.10
UNSTABLE_FLIP_RATE: Final = 0.01
FLOAT_DECIMALS: Final = 9
