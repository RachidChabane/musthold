"""Directory layout of an experiment root (PREREG 1)."""

from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path

from pilot.domain.constants import (
    CANARY_DIRNAME,
    CONFTEST_FILENAME,
    FIXTURE_DIRNAME,
    LABELS_FILENAME,
    MODELS_LOCK_FILENAME,
    ORACLE_DIRNAME,
    PACKAGE_NAME,
    PLANTED_DIRNAME,
    PREREG_DIRNAME,
    PROMPTS_DIRNAME,
    RAW_DIRNAME,
    REFACTOR_DIRNAME,
    RUNS_DIRNAME,
    SYSTEM_PROMPT_FILENAME,
    TESTS_DIRNAME,
    VIOLATIONS_DIRNAME,
)

_PACKAGE_DEPTH_TO_ROOT = 4


def default_root() -> Path:
    return Path(__file__).resolve().parents[_PACKAGE_DEPTH_TO_ROOT]


@dataclass(frozen=True)
class ExperimentLayout:
    root: Path

    @property
    def project(self) -> Path:
        return self.root / FIXTURE_DIRNAME / PACKAGE_NAME

    @property
    def tests_dir(self) -> Path:
        return self.project / TESTS_DIRNAME

    @property
    def conftest(self) -> Path:
        return self.project / CONFTEST_FILENAME

    @property
    def prereg(self) -> Path:
        return self.root / PREREG_DIRNAME

    @property
    def labels(self) -> Path:
        return self.prereg / LABELS_FILENAME

    @property
    def violations(self) -> Path:
        return self.prereg / VIOLATIONS_DIRNAME

    @property
    def planted(self) -> Path:
        return self.prereg / PLANTED_DIRNAME

    @property
    def refactor(self) -> Path:
        return self.prereg / REFACTOR_DIRNAME

    @property
    def oracle(self) -> Path:
        return self.prereg / ORACLE_DIRNAME

    @property
    def models_lock(self) -> Path:
        return self.prereg / MODELS_LOCK_FILENAME

    @property
    def system_prompt(self) -> Path:
        return self.prereg / PROMPTS_DIRNAME / SYSTEM_PROMPT_FILENAME

    def run_dir(self, run_id: str) -> Path:
        return self.root / RUNS_DIRNAME / run_id

    def raw_dir(self, run_id: str) -> Path:
        return self.run_dir(run_id) / RAW_DIRNAME

    def canary_dir(self, run_id: str) -> Path:
        return self.run_dir(run_id) / CANARY_DIRNAME
