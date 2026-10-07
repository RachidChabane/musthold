from __future__ import annotations

import subprocess
import sys
from pathlib import Path

import pytest

PROBE = """
import sys
from calibration.cli.main import build_parser
build_parser()
loaded = sorted(name for name in sys.modules if name.startswith("calibration.analysis"))
print(",".join(loaded))
"""
RUN_SIDE_PROBE = """
import sys
from calibration.cli import run_verbs
loaded = sorted(name for name in sys.modules if name.startswith("calibration.analysis"))
print(",".join(loaded))
"""


def _loaded(probe: str) -> list[str]:
    source = Path(__file__).resolve().parents[2] / "src"
    result = subprocess.run(
        [sys.executable, "-I", "-c", probe],
        capture_output=True,
        text=True,
        check=True,
        env={"PYTHONPATH": str(source)},
    )
    return [name for name in result.stdout.strip().split(",") if name]


@pytest.mark.integration
def test_building_the_parser_loads_no_analysis_module() -> None:
    assert _loaded(PROBE) == []


@pytest.mark.integration
def test_the_run_side_verbs_load_no_analysis_module() -> None:
    assert _loaded(RUN_SIDE_PROBE) == []
