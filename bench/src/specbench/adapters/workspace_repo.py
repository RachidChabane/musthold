"""Writes an agent workspace as a fresh single-commit repository, and reads a tree back as files."""

from __future__ import annotations

import os
import subprocess
from pathlib import Path
from typing import Final

from specbench.domain.workspace import Workspace

BRANCH: Final = "main"
COMMIT_MESSAGE: Final = "workspace"
COMMIT_DATE: Final = "2026-01-01T00:00:00+00:00"
IDENTITY: Final = {
    "GIT_AUTHOR_NAME": "bench",
    "GIT_AUTHOR_EMAIL": "bench@example.invalid",
    "GIT_COMMITTER_NAME": "bench",
    "GIT_COMMITTER_EMAIL": "bench@example.invalid",
    "GIT_AUTHOR_DATE": COMMIT_DATE,
    "GIT_COMMITTER_DATE": COMMIT_DATE,
    "GIT_CONFIG_GLOBAL": os.devnull,
    "GIT_CONFIG_SYSTEM": os.devnull,
}
GIT_DIRECTORY: Final = ".git"


class DiskTreeReader:
    """`TreeReader` over a directory: every regular file by relative POSIX path, skipping repository metadata."""

    def read(self, root: Path) -> dict[str, bytes]:
        return {
            path.relative_to(root).as_posix(): path.read_bytes()
            for path in sorted(root.rglob("*"))
            if path.is_file() and GIT_DIRECTORY not in path.relative_to(root).parts
        }


def _git(directory: Path, *arguments: str) -> str:
    done = subprocess.run(
        ["git", "-C", str(directory), *arguments],
        capture_output=True,
        text=True,
        check=True,
        env={**os.environ, **IDENTITY},
    )
    return done.stdout.strip()


def write_workspace_repo(workspace: Workspace, destination: Path) -> str:
    """Create `destination` as a repository whose only commit holds the workspace files; returns the commit id."""
    destination.mkdir(parents=True, exist_ok=False)
    for relative, content in workspace.files.items():
        target = destination / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_bytes(content)
    _git(destination, "init", "-q", "-b", BRANCH)
    _git(destination, "add", "-A")
    _git(destination, "commit", "-q", "--allow-empty", "-m", COMMIT_MESSAGE)
    return _git(destination, "rev-parse", "HEAD")
