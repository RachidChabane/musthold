"""Directory layout of an experiment root as the analysis sees it (PREREG 1)."""

from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path

from pilot.domain.analysis_constants import (
    CROSSCHECK_CACHE_FILENAME,
    DECISIONS_FILENAME,
    ENDPOINTS_FILENAME,
    EXPLORATORY_FILENAME,
    PRECISION_TABLE_FILENAME,
    REPORT_FILENAME,
    REPORT_TEMPLATE_FILENAME,
    RESULTS_DIRNAME,
)
from pilot.domain.ground_truth_paths import ORACLE_DIRNAME, ORACLE_FILE_GLOB
from pilot.domain.constants import (
    FIXTURE_DIRNAME,
    MODELS_LOCK_FILENAME,
    PACKAGE_NAME,
    PREREG_DIRNAME,
    RUNS_DIRNAME,
)

_PACKAGE_DEPTH_TO_ROOT = 4


def default_root() -> Path:
    return Path(__file__).resolve().parents[_PACKAGE_DEPTH_TO_ROOT]


@dataclass(frozen=True)
class AnalysisLayout:
    root: Path

    @property
    def project(self) -> Path:
        return self.root / FIXTURE_DIRNAME / PACKAGE_NAME

    @property
    def prereg(self) -> Path:
        return self.root / PREREG_DIRNAME

    @property
    def oracle_files(self) -> list[Path]:
        return sorted((self.prereg / ORACLE_DIRNAME).glob(ORACLE_FILE_GLOB))

    @property
    def models_lock(self) -> Path:
        return self.prereg / MODELS_LOCK_FILENAME

    @property
    def report_template(self) -> Path:
        return self.prereg / REPORT_TEMPLATE_FILENAME

    @property
    def precision_table(self) -> Path:
        return self.prereg / PRECISION_TABLE_FILENAME

    @property
    def results(self) -> Path:
        return self.root / RESULTS_DIRNAME

    @property
    def decisions(self) -> Path:
        return self.results / DECISIONS_FILENAME

    @property
    def endpoints(self) -> Path:
        return self.results / ENDPOINTS_FILENAME

    @property
    def report(self) -> Path:
        return self.results / REPORT_FILENAME

    @property
    def exploratory(self) -> Path:
        return self.results / EXPLORATORY_FILENAME

    @property
    def crosscheck_cache(self) -> Path:
        return self.results / CROSSCHECK_CACHE_FILENAME

    def run_dir(self, run_id: str) -> Path:
        return self.root / RUNS_DIRNAME / run_id
