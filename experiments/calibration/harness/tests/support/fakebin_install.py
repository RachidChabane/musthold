"""Installs the fake executables of `support/fakebin` into a directory, with the test interpreter as their shebang."""

from __future__ import annotations

import stat
import sys
from pathlib import Path

TESTS_DIR = Path(__file__).resolve().parents[1]
FAKEBIN_DIR = Path(__file__).resolve().parent / "fakebin"
FAKE_EXECUTABLES = ("ollama", "uv")
TESTS_DIR_VARIABLE = "FAKEBIN_TESTS_DIR"


def install_fakebin(destination: Path) -> Path:
    """Copies `ollama` and `uv` into `destination` (created) and returns it; the scripts read FAKEBIN_TESTS_DIR."""
    destination.mkdir(parents=True, exist_ok=True)
    for name in FAKE_EXECUTABLES:
        lines = (FAKEBIN_DIR / name).read_text(encoding="utf-8").splitlines(keepends=True)
        target = destination / name
        target.write_text(f"#!{sys.executable}\n" + "".join(lines[1:]), encoding="utf-8")
        target.chmod(target.stat().st_mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)
    return destination
