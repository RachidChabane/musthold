"""The freeze of the pre-registration (§16 step 1) and the preconditions PC1 and PC2 (§14.1)."""

from __future__ import annotations

from datetime import datetime

from calibration.domain.canonical import sha256_hex
from calibration.domain.ports import Timestamp


class PreconditionFailed(Exception):
    """A precondition of §14.1 does not hold: nothing runs."""

    def __init__(self, code: str, message: str) -> None:
        super().__init__(f"{code}: {message}")
        self.code = code
        self.message = message


def freeze_problems(tag_commit: str | None, recorded_commit_text: str | None, stamp: Timestamp | None) -> list[str]:
    """Why the freeze of the tag cannot be relied on: the recorded commit is the commit the tag peels to, and its
    timestamp imprint is the SHA-256 of the recorded file."""
    if tag_commit is None:
        return ["the pre-registration tag does not exist"]
    problems: list[str] = []
    if recorded_commit_text is None:
        return ["freeze/prereg-commit.txt is missing"]
    if recorded_commit_text.strip() != tag_commit:
        problems.append(f"freeze/prereg-commit.txt names {recorded_commit_text.strip()}, the tag peels to {tag_commit}")
    if stamp is None:
        problems.append("freeze/prereg-commit.tsr is missing or unreadable")
    elif stamp.imprint != sha256_hex(recorded_commit_text):
        problems.append(f"freeze/prereg-commit.tsr stamps {stamp.imprint}, not the digest of freeze/prereg-commit.txt")
    return problems


def stamped_before(stamp: Timestamp, moment: datetime) -> bool:
    """Whether the timestamp's genTime strictly precedes `moment` (both aware datetimes)."""
    return stamp.generated_at < moment
