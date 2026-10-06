"""Pre-registration drift check: the guarded paths must equal the pre-registration commit."""

from __future__ import annotations

from collections.abc import Sequence
from pathlib import Path

from pilot.adapters.process import ProcessRunner, SubprocessRunner

GIT = "git"
PEEL_TO_COMMIT = "^{commit}"


class PreregistrationTagMissing(Exception):
    """The pre-registration tag does not exist, so the pre-registration is not frozen."""


def preregistration_commit(root: Path, tag: str, process: ProcessRunner | None = None) -> str:
    """The commit the tag points to; annotated tags are peeled to their commit."""
    runner = process or SubprocessRunner()
    result = runner.run([GIT, "rev-parse", "--verify", "--quiet", f"refs/tags/{tag}{PEEL_TO_COMMIT}"], root)
    commit = result.stdout.strip()
    if result.returncode != 0 or not commit:
        raise PreregistrationTagMissing(f"tag {tag} does not exist; the pre-registration is not frozen")
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
