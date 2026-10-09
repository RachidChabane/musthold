"""Writing a built site to its directory, replacing a previous build and nothing else."""

from __future__ import annotations

import shutil
from collections.abc import Mapping
from pathlib import Path
from typing import Final

ENCODING: Final = "utf-8"
BUILD_MARKER: Final = ".specbench-site"
"""Marks a directory this writer made, so a rebuild never deletes a directory it did not write."""


class NotASiteDirectory(ValueError):
    """The output directory holds files this writer did not put there."""


def write_site(files: Mapping[str, str], out: Path) -> None:
    if out.exists() and any(out.iterdir()):
        if not (out / BUILD_MARKER).is_file():
            raise NotASiteDirectory(f"{out} is not empty and was not written by the site build")
        shutil.rmtree(out)
    out.mkdir(parents=True, exist_ok=True)
    (out / BUILD_MARKER).write_text("", encoding=ENCODING)
    for relative, content in sorted(files.items()):
        target = out / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_bytes(content.encode(ENCODING))
