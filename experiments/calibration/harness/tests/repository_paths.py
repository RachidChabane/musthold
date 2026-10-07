"""Locations of the committed experiment files the tests read."""

from __future__ import annotations

from pathlib import Path
from typing import Final

REPOSITORY_ROOT: Final = Path(__file__).resolve().parents[4]
PILOT_DIRECTORY: Final = REPOSITORY_ROOT / "experiments" / "limits-1-3"
PILOT_SOURCE: Final = PILOT_DIRECTORY / "harness" / "src" / "pilot"
PILOT_FIXTURE: Final = PILOT_DIRECTORY / "fixture" / "parkfee"
PILOT_REPLY: Final = PILOT_DIRECTORY / "freeze" / "prereg-commit.tsr"
CALIBRATION_REPLY: Final = REPOSITORY_ROOT / "experiments" / "calibration" / "freeze" / "prereg-commit.tsr"
