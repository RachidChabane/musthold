"""`ProductExporter`: the product tree of a revision without `experiments/`, unpacked as it is and digested."""

from __future__ import annotations

import hashlib
from pathlib import Path
from typing import Final

from calibration.domain.constants import PRODUCT_EXCLUDE
from calibration.domain.ports import VersionControl

ARCHIVE_ROOT: Final = "."
POSIX_SEPARATOR: Final = "/"


def file_digests(root: Path) -> dict[str, str]:
    """The SHA-256 of every file below `root`, keyed by its POSIX relative path, sorted."""
    files = sorted(path for path in root.rglob("*") if path.is_file())
    return {
        POSIX_SEPARATOR.join(path.relative_to(root).parts): hashlib.sha256(path.read_bytes()).hexdigest()
        for path in files
    }


class GitProductExporter:
    """Implements `ProductExporter` through `VersionControl.archive`; an exported file is never edited."""

    def __init__(self, vcs: VersionControl, root: Path) -> None:
        self._vcs = vcs
        self._root = root

    def export(self, revision: str, destination: Path) -> dict[str, str]:
        self._vcs.archive(revision, (ARCHIVE_ROOT,), destination, exclude=(PRODUCT_EXCLUDE,))
        return dict(sorted(file_digests(destination).items()))
