"""Run directories as digests: the source of an import, the target it fills and the seal of a run."""

from __future__ import annotations

import hashlib
import json
from collections.abc import Collection
from pathlib import Path
from typing import Any

from pilot.domain.import_manifest import SHA256SUMS_FILENAME, ImportRefused, parse_sums

ENCODING = "utf-8"


def file_digest(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def file_digests(root: Path, exclude: Collection[str] = ()) -> dict[str, str]:
    """SHA-256 of every file under `root`, keyed by its relative POSIX path."""
    if not root.is_dir():
        return {}
    return {
        path.relative_to(root).as_posix(): file_digest(path)
        for path in sorted(root.rglob("*"))
        if path.is_file() and path.relative_to(root).as_posix() not in exclude
    }


class DirectoryImportSource:
    """Implements the `ImportSource` port over a sealed run directory."""

    def __init__(self, root: Path) -> None:
        self._root = root

    def listing(self) -> dict[str, str]:
        if not self._root.is_dir():
            raise ImportRefused(f"no run directory: {self._root}")
        return file_digests(self._root, exclude=(SHA256SUMS_FILENAME,))

    def sums_text(self) -> str:
        path = self._root / SHA256SUMS_FILENAME
        if not path.is_file():
            raise ImportRefused(f"{path} is missing: the source run is not sealed")
        return path.read_text(encoding=ENCODING)

    def read_bytes(self, relative: str) -> bytes:
        return (self._root / relative).read_bytes()

    def read_json(self, relative: str) -> Any:
        return json.loads((self._root / relative).read_text(encoding=ENCODING))


def sealed_sums(root: Path) -> dict[str, str] | None:
    """The parsed SHA256SUMS of a run directory, or None when the run is not sealed."""
    path = root / SHA256SUMS_FILENAME
    return parse_sums(path.read_text(encoding=ENCODING)) if path.is_file() else None
