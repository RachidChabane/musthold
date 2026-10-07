from __future__ import annotations

import io
import tarfile
from collections.abc import Generator, Sequence
from contextlib import contextmanager
from datetime import UTC, datetime
from pathlib import Path
from typing import IO

import pytest

from calibration.adapters.git_vcs import GitFailure, GitVersionControl
from real_process_runner import RealProcessRunner
from temp_repository import TempRepository

pytestmark = pytest.mark.integration

FIRST_DATE = "2026-10-07T10:27:05+00:00"
SECOND_DATE = "2026-10-08T23:30:00+02:00"
MISSING_REVISION = "0" * 40


@pytest.fixture
def repository(tmp_path: Path) -> TempRepository:
    return TempRepository(tmp_path / "repository")


@pytest.fixture
def vcs(repository: TempRepository) -> GitVersionControl:
    return GitVersionControl(repository.root, RealProcessRunner())


def test_peel_tag_resolves_lightweight_and_annotated_tags_to_their_commit(
    repository: TempRepository, vcs: GitVersionControl
) -> None:
    first = repository.commit({"a.txt": "a"})
    repository.tag("light")
    second = repository.commit({"a.txt": "b"})
    repository.tag("annotated", annotated=True)
    assert vcs.peel_tag("light") == first
    assert vcs.peel_tag("annotated") == second


def test_peel_tag_is_none_for_an_unknown_tag_and_ignores_branches(
    repository: TempRepository, vcs: GitVersionControl
) -> None:
    commit = repository.commit({"a.txt": "a"})
    repository.branch_at("only-a-branch", commit)
    assert vcs.peel_tag("missing") is None
    assert vcs.peel_tag("only-a-branch") is None


def test_head_commit(repository: TempRepository, vcs: GitVersionControl) -> None:
    repository.commit({"a.txt": "a"})
    head = repository.commit({"a.txt": "b"})
    assert vcs.head_commit() == head


def test_is_clean_sees_modified_and_untracked_files_only_under_the_given_paths(
    repository: TempRepository, vcs: GitVersionControl
) -> None:
    repository.commit({"watched/a.txt": "a", "other/b.txt": "b"})
    assert vcs.is_clean(["watched", "other"])
    (repository.root / "other" / "b.txt").write_text("changed")
    assert vcs.is_clean(["watched"])
    assert not vcs.is_clean(["other"])
    (repository.root / "watched" / "new.txt").write_text("new")
    assert not vcs.is_clean(["watched"])


def test_archive_unpacks_the_revision_not_the_working_tree_and_excludes_paths(
    repository: TempRepository, vcs: GitVersionControl, tmp_path: Path
) -> None:
    repository.commit(
        {
            "script/run": "#!/bin/sh\n",
            "src/module.py": "value = 1\n",
            "experiments/pilot/data.txt": "excluded",
            "pkg/experiments/keep.txt": "nested directories named like the excluded one stay",
            "root.txt": "root",
        },
        executable=frozenset({"script/run"}),
    )
    revision = repository.head()
    (repository.root / "src" / "module.py").write_text("value = 2\n")
    destination = tmp_path / "out" / "export"
    vcs.archive(revision, (".",), destination, exclude=("experiments",))
    exported = sorted(str(path.relative_to(destination)) for path in destination.rglob("*") if path.is_file())
    assert exported == ["pkg/experiments/keep.txt", "root.txt", "script/run", "src/module.py"]
    assert (destination / "src" / "module.py").read_text() == "value = 1\n"
    assert (destination / "script" / "run").stat().st_mode & 0o100


def test_archive_of_pathspecs_unpacks_only_those_paths(
    repository: TempRepository, vcs: GitVersionControl, tmp_path: Path
) -> None:
    repository.commit({"a/one.txt": "1", "b/two.txt": "2"})
    destination = tmp_path / "subset"
    vcs.archive("HEAD", ("a",), destination)
    assert [path.name for path in destination.rglob("*") if path.is_file()] == ["one.txt"]


def test_archive_of_an_unknown_revision_fails(
    vcs: GitVersionControl, repository: TempRepository, tmp_path: Path
) -> None:
    repository.commit({"a.txt": "a"})
    with pytest.raises(GitFailure):
        vcs.archive(MISSING_REVISION, (".",), tmp_path / "never")


def _hostile_stream(member_name: str) -> Generator[IO[bytes]]:
    buffer = io.BytesIO()
    with tarfile.open(fileobj=buffer, mode="w") as archive:
        payload = b"escaped"
        member = tarfile.TarInfo(member_name)
        member.size = len(payload)
        archive.addfile(member, io.BytesIO(payload))
    buffer.seek(0)
    yield buffer


@pytest.mark.parametrize("member_name", ["../escaped.txt", "a/../../escaped.txt"])
def test_archive_never_extracts_outside_the_destination(
    member_name: str, repository: TempRepository, tmp_path: Path
) -> None:
    @contextmanager
    def hostile(argv: Sequence[str]) -> Generator[IO[bytes]]:
        yield from _hostile_stream(member_name)

    vcs = GitVersionControl(repository.root, RealProcessRunner(), stream=hostile)
    destination = tmp_path / "work" / "destination"
    with pytest.raises(tarfile.TarError):
        vcs.archive("HEAD", (".",), destination)
    assert not (tmp_path / "work" / "escaped.txt").exists()
    assert not (tmp_path / "escaped.txt").exists()


def test_archive_confines_an_absolute_member_name_to_the_destination(
    repository: TempRepository, tmp_path: Path
) -> None:
    absolute = tmp_path / "outside" / "absolute.txt"

    @contextmanager
    def hostile(argv: Sequence[str]) -> Generator[IO[bytes]]:
        yield from _hostile_stream(str(absolute))

    vcs = GitVersionControl(repository.root, RealProcessRunner(), stream=hostile)
    destination = tmp_path / "destination"
    vcs.archive("HEAD", (".",), destination)
    assert not absolute.exists()
    assert [path.name for path in destination.rglob("*") if path.is_file()] == ["absolute.txt"]


def test_is_ancestor(repository: TempRepository, vcs: GitVersionControl) -> None:
    first = repository.commit({"a.txt": "a"})
    second = repository.commit({"a.txt": "b"})
    repository.branch_at("side", first)
    repository.checkout("side")
    side = repository.commit({"c.txt": "c"})
    assert vcs.is_ancestor(first, second)
    assert vcs.is_ancestor(first, first)
    assert not vcs.is_ancestor(second, first)
    assert not vcs.is_ancestor(second, side)


def test_is_ancestor_raises_for_an_unknown_commit(repository: TempRepository, vcs: GitVersionControl) -> None:
    head = repository.commit({"a.txt": "a"})
    with pytest.raises(GitFailure):
        vcs.is_ancestor(MISSING_REVISION, head)


def test_commit_time_is_the_committer_date_in_utc(repository: TempRepository, vcs: GitVersionControl) -> None:
    first = repository.commit({"a.txt": "a"}, date=FIRST_DATE)
    second = repository.commit({"a.txt": "b"}, date=SECOND_DATE)
    assert vcs.commit_time(first) == datetime(2026, 10, 7, 10, 27, 5, tzinfo=UTC)
    assert vcs.commit_time(second) == datetime(2026, 10, 8, 21, 30, 0, tzinfo=UTC)
    assert vcs.commit_time(second).tzinfo is UTC


def test_commits_touching_lists_the_commits_that_changed_the_path_oldest_first(
    repository: TempRepository, vcs: GitVersionControl
) -> None:
    created = repository.commit({"tracked.txt": "1", "other.txt": "1"})
    repository.commit({"other.txt": "2"})
    changed = repository.commit({"tracked.txt": "2"})
    repository.commit({"other.txt": "3"})
    removed = repository.commit({}, remove=("tracked.txt",))
    assert vcs.commits_touching("HEAD", "tracked.txt") == [created, changed, removed]
    assert vcs.commits_touching("HEAD", "never.txt") == []


def test_blob_returns_the_content_at_a_revision_and_none_when_absent(
    repository: TempRepository, vcs: GitVersionControl
) -> None:
    first = repository.commit({"dir/a.bin": b"\x00\x01binary"})
    second = repository.commit({"dir/a.bin": b"later"})
    assert vcs.blob(first, "dir/a.bin") == b"\x00\x01binary"
    assert vcs.blob(second, "dir/a.bin") == b"later"
    assert vcs.blob(first, "dir/missing.bin") is None
    assert vcs.blob(MISSING_REVISION, "dir/a.bin") is None
