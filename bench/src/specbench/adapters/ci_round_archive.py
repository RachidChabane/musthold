"""The archive of the CI round trips of a gate: `<record dir>/ci/<k>/<module>/` holds the flat files of the host ref the
gate-host session pushed after answering the requests CI issued in round `k` (ADR 0011, decision 12).

A round is archived whole or not at all: it is written beside the others under a temporary name and renamed into place,
so a gate killed while archiving leaves no round that a replay could mistake for finished. Every push of a session
holds all the answers its store holds, so the bundle of the latest round that has a module's directory carries every
earlier answer; `bundle_for` checks that against the answers the settle step recorded itself.
"""

from __future__ import annotations

import shutil
from collections.abc import Mapping
from pathlib import Path
from typing import Final

from specbench.domain.host_bundle import MANIFEST_FILE, BundleManifest, parse_bundle_manifest

CI_DIRECTORY: Final = "ci"
PARTIAL_SUFFIX: Final = ".partial"


class CiRoundArchive:
    """Reads and writes the archived rounds under one record directory."""

    def __init__(self, record_dir: Path) -> None:
        self._root = record_dir / CI_DIRECTORY

    def rounds(self) -> int:
        """The number of consecutive archived rounds, counting from the first."""
        number = 0
        while self._round(number + 1).is_dir():
            number += 1
        return number

    def archive(self, number: int, exports: Mapping[str, Path]) -> None:
        """Archives round `number`, whose modules' exported host refs are the directories of `exports`."""
        partial = self._root / f"{number}{PARTIAL_SUFFIX}"
        shutil.rmtree(partial, ignore_errors=True)
        for module_ref, export in exports.items():
            shutil.copytree(export, partial / module_ref)
        shutil.rmtree(self._round(number), ignore_errors=True)
        partial.rename(self._round(number))

    def latest_bundle(self, module_ref: str, rounds: int) -> Path | None:
        """The directory of the latest of the first `rounds` rounds that archived `module_ref`, if any."""
        for number in range(rounds, 0, -1):
            candidate = self._round(number) / module_ref
            if candidate.is_dir():
                return candidate
        return None

    def _round(self, number: int) -> Path:
        return self._root / str(number)


def read_manifest(bundle: Path) -> BundleManifest:
    return parse_bundle_manifest((bundle / MANIFEST_FILE).read_text(encoding="utf-8"))
