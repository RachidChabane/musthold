"""A run directory on disk: atomic writes, so a record appears complete or not at all."""

from __future__ import annotations

import os
import tempfile
from pathlib import Path
from typing import Final

from calibration.domain.constants import ENCODING

TEMPORARY_PREFIX: Final = ".tmp-"


class FileRunStore:
    """Implements `RunStore` over a directory; relative paths never leave it."""

    def __init__(self, root: Path) -> None:
        self._root = root

    @property
    def root(self) -> Path:
        return self._root

    def _resolve(self, relative: str) -> Path:
        path = (self._root / relative).resolve()
        if not path.is_relative_to(self._root.resolve()):
            raise ValueError(f"{relative!r} leaves the run directory")
        return path

    def exists(self, relative: str) -> bool:
        return self._resolve(relative).exists()

    def read_bytes(self, relative: str) -> bytes:
        return self._resolve(relative).read_bytes()

    def read_text(self, relative: str) -> str:
        return self._resolve(relative).read_text(encoding=ENCODING)

    def write_bytes(self, relative: str, data: bytes) -> None:
        path = self._resolve(relative)
        path.parent.mkdir(parents=True, exist_ok=True)
        descriptor, temporary = tempfile.mkstemp(prefix=TEMPORARY_PREFIX, dir=path.parent)
        try:
            with os.fdopen(descriptor, "wb") as handle:
                handle.write(data)
                handle.flush()
                os.fsync(handle.fileno())
            os.replace(temporary, path)
        except BaseException:
            Path(temporary).unlink(missing_ok=True)
            raise

    def write_text(self, relative: str, text: str) -> None:
        self.write_bytes(relative, text.encode(ENCODING))

    def append_text(self, relative: str, text: str) -> None:
        path = self._resolve(relative)
        path.parent.mkdir(parents=True, exist_ok=True)
        with path.open("a", encoding=ENCODING) as handle:
            handle.write(text)
            handle.flush()
            os.fsync(handle.fileno())

    def list_dir(self, relative: str) -> list[str]:
        path = self._resolve(relative)
        return sorted(entry.name for entry in path.iterdir()) if path.is_dir() else []
