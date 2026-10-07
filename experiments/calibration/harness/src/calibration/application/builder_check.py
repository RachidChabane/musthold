"""The builder check of §16 step 3 and PC3 (e): the alignment builder rebuilds, byte for byte, every closed set that
the frozen alignment binary exports at its first `awaiting_host` wave on the product's own test project."""

from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path
from typing import Final

from calibration.application.build_ports import AlignmentDriver, Builders
from calibration.domain.canonical import JsonValue
from calibration.domain.jsonread import integer, sequence, text

FIXTURE_DIR: Final = Path("s1relay") / "testdata" / "alignment"
PROJECT_DIR: Final = FIXTURE_DIR / "project"
RECORDED_SHALLNOT: Final = FIXTURE_DIR / "shallnot-recorded.sh"
RECORDED_REPORT: Final = FIXTURE_DIR / "shallnot-report.json"
RECORDED_REPORT_ENV: Final = "SHALLNOT_RECORDED_REPORT"


@dataclass(frozen=True)
class BuilderCheck:
    exported: int
    rebuilt: int
    matched: int
    unmatched_exported: tuple[str, ...]
    unmatched_rebuilt: tuple[str, ...]

    @property
    def passed(self) -> bool:
        """Every closed set the product exported is rebuilt byte for byte. Tests the product decides without System
        One are never exported, so rebuilt sets beyond the exported ones are expected."""
        return self.exported > 0 and self.matched == self.exported and not self.unmatched_exported

    def to_record(self, harness_commit: str) -> dict[str, JsonValue]:
        return {
            "harness_commit": harness_commit,
            "exported": self.exported,
            "rebuilt": self.rebuilt,
            "matched": self.matched,
            "unmatched_exported": list(self.unmatched_exported),
            "unmatched_rebuilt": list(self.unmatched_rebuilt),
            "passed": self.passed,
        }


def recorded_report_path(export_dir: Path) -> Path:
    return export_dir / RECORDED_REPORT


def run_builder_check(driver: AlignmentDriver, builders: Builders, export_dir: Path) -> BuilderCheck:
    """Drives the alignment binary of the export to its first wave and compares its export with the builder's."""
    exported_text = driver.first_wave_export(export_dir, export_dir / RECORDED_SHALLNOT, export_dir / RECORDED_REPORT)
    report = builders.compare_export(export_dir / PROJECT_DIR, export_dir / RECORDED_SHALLNOT, exported_text)
    return BuilderCheck(
        exported=integer(report.get("exported"), "compare.exported"),
        rebuilt=integer(report.get("rebuilt"), "compare.rebuilt"),
        matched=integer(report.get("matched"), "compare.matched"),
        unmatched_exported=tuple(
            text(ticket, "ticket")
            for ticket in sequence(report.get("unmatched_exported"), "compare.unmatched_exported")
        ),
        unmatched_rebuilt=tuple(
            text(label, "label") for label in sequence(report.get("unmatched_rebuilt"), "compare.unmatched_rebuilt")
        ),
    )
