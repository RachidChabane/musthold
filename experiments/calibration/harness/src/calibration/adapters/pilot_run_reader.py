"""`PilotRunReader`: the only files of the pilot run that item construction may open, each digested as it is read."""

from __future__ import annotations

import hashlib
import re
from pathlib import Path
from typing import Final

from calibration.domain.constants import (
    ENCODING,
    PILOT_BUILD_INPUT_FILES,
    PILOT_MUTANTS_DIR,
    SOURCE_SUMS_FILE,
)

MUTANT_ID_PATTERN: Final = r"[0-9a-f]{16}"
MUTANT_FILE_SUFFIX: Final = ".json"
READABLE_FILES: Final = (*PILOT_BUILD_INPUT_FILES, SOURCE_SUMS_FILE)
ALLOWED_PATH: Final = re.compile(
    "|".join(
        (
            *(re.escape(name) for name in READABLE_FILES),
            re.escape(f"{PILOT_MUTANTS_DIR}/") + MUTANT_ID_PATTERN + re.escape(MUTANT_FILE_SUFFIX),
        )
    )
)
SEAL_LINE: Final = re.compile(r"^(?P<digest>[0-9a-f]{64}) [ *](?P<path>.+)$")
CURRENT_DIRECTORY_PREFIX: Final = "./"


class ForbiddenPilotFile(Exception):
    """A path outside the allowlist of the pilot run: it is refused before anything is opened."""


def is_allowed(relative: str) -> bool:
    return ALLOWED_PATH.fullmatch(relative) is not None


def seal_digests(seal_text: str) -> dict[str, str]:
    """The `<digest>  <path>` lines of a `SHA256SUMS` file as {path: digest}."""
    sealed: dict[str, str] = {}
    for line in seal_text.splitlines():
        match = SEAL_LINE.match(line)
        if match:
            sealed[match.group("path").removeprefix(CURRENT_DIRECTORY_PREFIX)] = match.group("digest")
    return sealed


class AllowlistedPilotRunReader:
    """Implements `PilotRunReader` over one run directory."""

    def __init__(self, run_dir: Path) -> None:
        self._run_dir = run_dir
        self._digests: dict[str, str] = {}

    def exists(self, relative: str) -> bool:
        return self._path(relative).is_file()

    def read_text(self, relative: str) -> str:
        data = self._path(relative).read_bytes()
        self._digests[relative] = hashlib.sha256(data).hexdigest()
        return data.decode(ENCODING)

    def digests(self) -> dict[str, str]:
        return dict(sorted(self._digests.items()))

    def verify_against_seal(self) -> list[str]:
        """The files read whose digest differs from the seal or is absent from it; empty when no seal exists."""
        if not self.exists(SOURCE_SUMS_FILE):
            return []
        sealed = seal_digests(self.read_text(SOURCE_SUMS_FILE))
        return [
            self._mismatch(relative, digest, sealed.get(relative))
            for relative, digest in self.digests().items()
            if relative != SOURCE_SUMS_FILE and sealed.get(relative) != digest
        ]

    @staticmethod
    def _mismatch(relative: str, read: str, sealed: str | None) -> str:
        if sealed is None:
            return f"{relative}: read {read}, absent from the seal"
        return f"{relative}: read {read}, sealed {sealed}"

    def _path(self, relative: str) -> Path:
        if not is_allowed(relative):
            raise ForbiddenPilotFile(f"{relative!r} is not a file of the pilot run that item construction may read")
        path = self._run_dir / relative
        if path.is_symlink():
            raise ForbiddenPilotFile(f"{relative!r} is a symbolic link")
        return path
