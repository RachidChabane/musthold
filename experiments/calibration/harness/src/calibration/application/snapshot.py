"""`snapshot` (§2.4): the pilot's fixture, pre-registration and `pilot` package at the a1 tag, unpacked and digested."""

from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path

from calibration.application.build_ports import TreeDigester
from calibration.application.ports import RunStore
from calibration.domain.canonical import JsonValue, document_json
from calibration.domain.constants import PILOT_FREEZE_TAG, SNAPSHOT_FILE, SNAPSHOT_PATHS
from calibration.domain.freeze import PreconditionFailed
from calibration.domain.ports import VersionControl

SNAPSHOT_DIR_PREFIX = "pilot-"


@dataclass(frozen=True)
class SnapshotResult:
    pilot_commit: str
    directory: Path
    files: dict[str, str]


def snapshot_directory(cache_dir: Path, pilot_commit: str) -> Path:
    return cache_dir / f"{SNAPSHOT_DIR_PREFIX}{pilot_commit}"


class SnapshotUseCase:
    """`git archive` of the three pilot paths at the a1 tag into `.cache/pilot-<commit>/`; the working tree of
    `experiments/limits-1-3/` is never read."""

    def __init__(self, vcs: VersionControl, store: RunStore, digester: TreeDigester, cache_dir: Path) -> None:
        self._vcs = vcs
        self._store = store
        self._digester = digester
        self._cache_dir = cache_dir

    def run(self) -> SnapshotResult:
        commit = self._vcs.peel_tag(PILOT_FREEZE_TAG)
        if commit is None:
            raise PreconditionFailed("PC2", f"the tag {PILOT_FREEZE_TAG} does not exist")
        directory = snapshot_directory(self._cache_dir, commit)
        self._digester.remove(directory)
        self._vcs.archive(commit, SNAPSHOT_PATHS, directory)
        files = self._digester.digest(directory)
        record: dict[str, JsonValue] = {"pilot_commit": commit, "files": files}
        self._store.write_text(SNAPSHOT_FILE, document_json(record))
        return SnapshotResult(commit, directory, files)
