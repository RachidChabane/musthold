"""Directory layout of an experiment root including the ground truth (pre-registration tools, analysis)."""

from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path

from pilot.cli.pipeline_layout import PipelineLayout, default_root
from pilot.domain.ground_truth_paths import (
    ARM_P_LABELS_FILENAME,
    LABELS_FILENAME,
    ORACLE_DIRNAME,
    PLANTED_DIRNAME,
    REFACTOR_DIRNAME,
    VIOLATIONS_DIRNAME,
)
from pilot.domain.pipeline_constants import ARM_P_DIRNAME

__all__ = ["ExperimentLayout", "default_root"]


@dataclass(frozen=True)
class ExperimentLayout(PipelineLayout):
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
    def arm_p_labels(self) -> Path:
        return self.prereg / ARM_P_DIRNAME / ARM_P_LABELS_FILENAME

    @property
    def oracle(self) -> Path:
        return self.prereg / ORACLE_DIRNAME
