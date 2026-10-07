from __future__ import annotations

import json
from collections.abc import Sequence
from pathlib import Path

import pytest

from calibration.application.snapshot import SnapshotUseCase, snapshot_directory
from calibration.domain.constants import PILOT_FREEZE_TAG, SNAPSHOT_FILE, SNAPSHOT_PATHS
from calibration.domain.freeze import PreconditionFailed
from support.build_fakes import FakeTreeDigester, FakeVersionControl, InMemoryRunStore

PILOT_COMMIT = "b" * 40
CACHE = Path("/cache")
DIGESTS = {"experiments/limits-1-3/fixture/a.py": "digest-a", "experiments/limits-1-3/prereg/b.md": "digest-b"}


class _OrderedVersionControl(FakeVersionControl):
    def __init__(self, events: list[str]) -> None:
        super().__init__({PILOT_FREEZE_TAG: PILOT_COMMIT}, "head", {}, clean=True)
        self._events = events

    def archive(
        self,
        revision: str,
        pathspecs: Sequence[str],
        destination: Path,
        *,
        exclude: Sequence[str] = (),
    ) -> None:
        self._events.append(f"archive:{destination}")
        super().archive(revision, pathspecs, destination, exclude=exclude)


def _use_case() -> tuple[SnapshotUseCase, _OrderedVersionControl, InMemoryRunStore, list[str]]:
    events: list[str] = []
    vcs = _OrderedVersionControl(events)
    store = InMemoryRunStore()
    return SnapshotUseCase(vcs, store, FakeTreeDigester(DIGESTS, events), CACHE), vcs, store, events


def test_the_snapshot_directory_is_named_by_the_pilot_commit() -> None:
    assert snapshot_directory(CACHE, PILOT_COMMIT) == CACHE / f"pilot-{PILOT_COMMIT}"


def test_the_pilot_paths_are_archived_at_the_a1_tag_commit() -> None:
    use_case, vcs, _, _ = _use_case()

    result = use_case.run()

    assert vcs.archived == [(PILOT_COMMIT, SNAPSHOT_PATHS, CACHE / f"pilot-{PILOT_COMMIT}")]
    assert result.pilot_commit == PILOT_COMMIT
    assert result.directory == CACHE / f"pilot-{PILOT_COMMIT}"


def test_a_previous_unpacking_is_removed_before_the_archive_is_unpacked() -> None:
    use_case, _, _, events = _use_case()

    use_case.run()

    directory = CACHE / f"pilot-{PILOT_COMMIT}"
    assert events == [f"remove:{directory}", f"archive:{directory}"]


def test_the_digest_of_every_extracted_file_is_recorded() -> None:
    use_case, _, store, _ = _use_case()

    result = use_case.run()

    assert result.files == DIGESTS
    assert json.loads(store.read_text(SNAPSHOT_FILE)) == {"pilot_commit": PILOT_COMMIT, "files": DIGESTS}


def test_a_missing_pilot_tag_is_a_pc2_failure_and_writes_nothing() -> None:
    use_case, vcs, store, _ = _use_case()
    vcs.tags.clear()

    with pytest.raises(PreconditionFailed) as failure:
        use_case.run()

    assert failure.value.code == "PC2"
    assert store.paths() == []
    assert vcs.archived == []
