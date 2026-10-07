"""Where the experiment's directories are (§1)."""

from __future__ import annotations

import os
from dataclasses import dataclass
from pathlib import Path
from typing import Final

from calibration.domain.constants import (
    CACHE_DIR_NAME,
    EXPERIMENT_DIR,
    KEV_COMMIT,
    PILOT_DIR,
    PILOT_RUN_ID,
    RESULTS_DIR_NAME,
    RUNS_DIR_NAME,
)

REPO_ROOT_ENV: Final = "CALIBRATION_REPO_ROOT"
SHALLNOT_SOURCE_ENV: Final = "SHALLNOT_SRC"
SHALLNOT_SOURCE_DEFAULT: Final = "shallnot-proof-gate"
PARENTS_TO_REPOSITORY: Final = 6
HARNESS_PATHS: Final = (
    f"{EXPERIMENT_DIR}/harness/src",
    f"{EXPERIMENT_DIR}/harness/pyproject.toml",
    f"{EXPERIMENT_DIR}/harness/uv.lock",
    f"{EXPERIMENT_DIR}/builder",
    f"{EXPERIMENT_DIR}/script",
)


@dataclass(frozen=True)
class Layout:
    """The repository root and the directories below it that the verbs use."""

    root: Path

    @classmethod
    def discover(cls) -> Layout:
        override = os.environ.get(REPO_ROOT_ENV)
        if override:
            return cls(Path(override).resolve())
        return cls(Path(__file__).resolve().parents[PARENTS_TO_REPOSITORY])

    @property
    def calibration_dir(self) -> Path:
        return self.root / EXPERIMENT_DIR

    @property
    def harness_dir(self) -> Path:
        return self.calibration_dir / "harness"

    @property
    def builder_dir(self) -> Path:
        return self.calibration_dir / "builder"

    @property
    def cache_dir(self) -> Path:
        return self.calibration_dir / CACHE_DIR_NAME

    @property
    def freeze_receipt(self) -> Path:
        return self.calibration_dir / "freeze" / "prereg-commit.tsr"

    @property
    def canary_prompt(self) -> Path:
        return self.calibration_dir / "prompts" / "canary.json"

    @property
    def mem_guard(self) -> Path:
        return self.root / "script" / "mem-guard"

    @property
    def kev_directory(self) -> Path:
        return self.cache_dir / f"kev-{KEV_COMMIT}"

    @property
    def pilot_run_dir(self) -> Path:
        return self.root / PILOT_DIR / RUNS_DIR_NAME / PILOT_RUN_ID

    def run_dir(self, run_id: str) -> Path:
        return self.calibration_dir / RUNS_DIR_NAME / run_id

    def results_dir(self) -> Path:
        return self.calibration_dir / RESULTS_DIR_NAME

    def shallnot_source(self) -> Path:
        configured = os.environ.get(SHALLNOT_SOURCE_ENV)
        return Path(configured).resolve() if configured else (self.root.parent / SHALLNOT_SOURCE_DEFAULT).resolve()
