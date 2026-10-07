"""Test doubles of the version-control and timestamp ports that `analyze` reads."""

from __future__ import annotations

from collections.abc import Sequence
from datetime import UTC, datetime
from pathlib import Path

from calibration.domain.ports import Timestamp

COMMIT_TIME = datetime(2026, 9, 20, 12, 0, tzinfo=UTC)
STAMP_TIME = datetime(2026, 9, 1, 8, 30, tzinfo=UTC)
STAMP_IMPRINT = "f" * 64


class FakeVersionControl:
    """A `VersionControl` that knows one commit time and the text of one blob."""

    def __init__(self, commit_time: datetime = COMMIT_TIME, blob_text: bytes | None = None) -> None:
        self._commit_time = commit_time
        self._blob_text = blob_text

    def peel_tag(self, tag: str) -> str | None:
        return None

    def head_commit(self) -> str:
        return "1" * 40

    def is_clean(self, paths: Sequence[str]) -> bool:
        return True

    def archive(
        self, revision: str, pathspecs: Sequence[str], destination: Path, *, exclude: Sequence[str] = ()
    ) -> None:
        destination.mkdir(parents=True, exist_ok=True)

    def is_ancestor(self, ancestor: str, descendant: str) -> bool:
        return True

    def commit_time(self, commit: str) -> datetime:
        return self._commit_time

    def commits_touching(self, revision: str, path: str) -> list[str]:
        return []

    def blob(self, revision: str, path: str) -> bytes | None:
        return self._blob_text


class FakeTimestampReader:
    """A `TimestampReader` that returns one stamp, or None when it cannot be read."""

    def __init__(self, generated_at: datetime | None = STAMP_TIME) -> None:
        self._stamp = None if generated_at is None else Timestamp(STAMP_IMPRINT, generated_at)

    def read(self, reply: Path) -> Timestamp | None:
        return self._stamp
