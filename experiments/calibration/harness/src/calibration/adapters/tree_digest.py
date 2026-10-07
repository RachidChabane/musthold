"""The SHA-256 of every file of a directory tree."""

from __future__ import annotations

import hashlib
import shutil
from pathlib import Path
from typing import Final

CHUNK_BYTES: Final = 1 << 20
POSIX_SEPARATOR: Final = "/"


def file_digest(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(CHUNK_BYTES), b""):
            digest.update(chunk)
    return digest.hexdigest()


class FileTreeDigester:
    """Implements `TreeDigester`: files only, keyed by their POSIX path relative to the root, sorted."""

    def digest(self, root: Path) -> dict[str, str]:
        files = sorted(path for path in root.rglob("*") if path.is_file() and not path.is_symlink())
        return {path.relative_to(root).as_posix(): file_digest(path) for path in files}

    def remove(self, root: Path) -> None:
        shutil.rmtree(root, ignore_errors=True)
