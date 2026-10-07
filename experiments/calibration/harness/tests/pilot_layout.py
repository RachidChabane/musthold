"""Unpacks the pilot code and fixture of the working tree into a temporary snapshot layout."""

from __future__ import annotations

import shutil
from pathlib import Path
from typing import Final

from repository_paths import PILOT_FIXTURE, PILOT_SOURCE

SNAPSHOT_SOURCE_DIR: Final = Path("experiments/limits-1-3/harness/src/pilot")
SNAPSHOT_FIXTURE_DIR: Final = Path("experiments/limits-1-3/fixture/parkfee")
IGNORED: Final = shutil.ignore_patterns("__pycache__", ".pytest_cache", ".venv", "*.pyc")
MARKER_MODULE: Final = Path("pilot/domain/normalization.py")


def unpack_snapshot(root: Path, marker: str | None = None) -> Path:
    """Copies the pilot package and the fixture below `root`; `marker` is appended to the normalization module."""
    shutil.copytree(PILOT_SOURCE, root / SNAPSHOT_SOURCE_DIR, ignore=IGNORED)
    shutil.copytree(PILOT_FIXTURE, root / SNAPSHOT_FIXTURE_DIR, ignore=IGNORED)
    if marker is not None:
        module = root / SNAPSHOT_SOURCE_DIR / "domain" / "normalization.py"
        module.write_text(module.read_text() + f'\nSNAPSHOT_MARKER = "{marker}"\n')
    return root
