"""Directory layout of an experiment root as the pipeline sees it (PREREG 1): no ground truth."""

from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path

from pilot.domain.constants import (
    CANARY_DIRNAME,
    CONFTEST_FILENAME,
    FIXTURE_DIRNAME,
    MODELS_LOCK_FILENAME,
    PACKAGE_NAME,
    PREREG_DIRNAME,
    PROMPTS_DIRNAME,
    RAW_DIRNAME,
    RUNS_DIRNAME,
    SOURCE_DIRNAME,
    SPEC_DIRNAME,
    SYSTEM_PROMPT_FILENAME,
    TESTS_DIRNAME,
)
from pilot.domain.pipeline_constants import (
    ARM_P_DIRNAME,
    ARM_P_ITEMS_DIRNAME,
    CODEBOOK_FILENAME,
    HARNESS_DIRNAME,
    LEAK_AUDIT_DIRNAME,
    SCHEMAS_DIRNAME,
)

_PACKAGE_DEPTH_TO_ROOT = 4


def default_root() -> Path:
    return Path(__file__).resolve().parents[_PACKAGE_DEPTH_TO_ROOT]


@dataclass(frozen=True)
class PipelineLayout:
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
    def spec_dir(self) -> Path:
        return self.project / SPEC_DIRNAME

    @property
    def source_dir(self) -> Path:
        return self.project / SOURCE_DIRNAME

    @property
    def package_dir(self) -> Path:
        return self.source_dir / PACKAGE_NAME

    @property
    def harness_source(self) -> Path:
        return self.root / HARNESS_DIRNAME / SOURCE_DIRNAME

    @property
    def prereg(self) -> Path:
        return self.root / PREREG_DIRNAME

    @property
    def models_lock(self) -> Path:
        return self.prereg / MODELS_LOCK_FILENAME

    @property
    def prompts_dir(self) -> Path:
        return self.prereg / PROMPTS_DIRNAME

    @property
    def schemas_dir(self) -> Path:
        return self.prereg / SCHEMAS_DIRNAME

    @property
    def system_prompt(self) -> Path:
        return self.prompts_dir / SYSTEM_PROMPT_FILENAME

    @property
    def codebook(self) -> Path:
        return self.prereg / CODEBOOK_FILENAME

    @property
    def arm_p_items(self) -> Path:
        return self.prereg / ARM_P_DIRNAME / ARM_P_ITEMS_DIRNAME

    @property
    def leak_audit_dir(self) -> Path:
        return self.prereg / LEAK_AUDIT_DIRNAME

    def run_dir(self, run_id: str) -> Path:
        return self.root / RUNS_DIRNAME / run_id

    def raw_dir(self, run_id: str) -> Path:
        return self.run_dir(run_id) / RAW_DIRNAME

    def canary_dir(self, run_id: str) -> Path:
        return self.run_dir(run_id) / CANARY_DIRNAME
