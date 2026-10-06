"""Pre-registration drift check: the guarded paths must equal the pre-registration commit."""

from __future__ import annotations

from collections.abc import Sequence
from pathlib import Path

from pilot.adapters.process import ProcessRunner, SubprocessRunner

GIT = "git"


class PreregistrationUncommitted(Exception):
    """PREREG.md has no commit yet."""


def preregistration_commit(root: Path, document: str, process: ProcessRunner | None = None) -> str:
    runner = process or SubprocessRunner()
    result = runner.run([GIT, "log", "-1", "--format=%H", "--", document], root)
    commit = result.stdout.strip()
    if result.returncode != 0 or not commit:
        raise PreregistrationUncommitted(f"{document} has no commit; the pre-registration is not committed")
    return commit


def drifted_paths(
    root: Path, commit: str, guarded: Sequence[str], process: ProcessRunner | None = None
) -> list[str]:
    """Tracked paths changed since the commit plus any untracked or modified path under the guarded set."""
    runner = process or SubprocessRunner()
    changed = runner.run([GIT, "diff", "--name-only", commit, "--", *guarded], root)
    status = runner.run([GIT, "status", "--porcelain", "--untracked-files=all", "--", *guarded], root)
    lines = changed.stdout.splitlines() + [line[3:] for line in status.stdout.splitlines()]
    return sorted({line.strip() for line in lines if line.strip()})
