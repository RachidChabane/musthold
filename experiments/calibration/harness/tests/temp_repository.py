"""A throwaway repository driven by plain `git`, for the adapters that read a history."""

from __future__ import annotations

import os
import subprocess
from collections.abc import Mapping
from pathlib import Path
from typing import Final

AUTHOR_NAME: Final = "Calibration Test"
AUTHOR_EMAIL: Final = "calibration@example.invalid"
INITIAL_BRANCH: Final = "main"
DEFAULT_DATE: Final = "2026-10-07T12:00:00+00:00"
EXECUTABLE_MODE: Final = 0o755
REGULAR_MODE: Final = 0o644


class TempRepository:
    def __init__(self, root: Path) -> None:
        self.root = root
        root.mkdir(parents=True, exist_ok=True)
        self._run("init", "--quiet", f"--initial-branch={INITIAL_BRANCH}")

    def commit(
        self,
        files: Mapping[str, bytes | str],
        message: str = "commit",
        *,
        date: str = DEFAULT_DATE,
        executable: frozenset[str] = frozenset(),
        remove: tuple[str, ...] = (),
    ) -> str:
        for relative in remove:
            (self.root / relative).unlink()
        for relative, content in files.items():
            path = self.root / relative
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(content if isinstance(content, bytes) else content.encode())
            path.chmod(EXECUTABLE_MODE if relative in executable else REGULAR_MODE)
        self._run("add", "--all")
        self._run("commit", "--quiet", "--allow-empty", "-m", message, date=date)
        return self.head()

    def head(self) -> str:
        return self._run("rev-parse", "HEAD").strip()

    def tag(self, name: str, *, annotated: bool = False) -> None:
        if annotated:
            self._run("tag", "-a", name, "-m", name)
        else:
            self._run("tag", name)

    def branch_at(self, name: str, commit: str) -> None:
        self._run("branch", name, commit)

    def checkout(self, name: str) -> None:
        self._run("checkout", "--quiet", name)

    def merge(self, name: str) -> str:
        self._run("merge", "--quiet", "--no-ff", "-m", "merge", name)
        return self.head()

    def _run(self, *arguments: str, date: str = DEFAULT_DATE) -> str:
        environment = {
            **os.environ,
            "GIT_AUTHOR_NAME": AUTHOR_NAME,
            "GIT_AUTHOR_EMAIL": AUTHOR_EMAIL,
            "GIT_COMMITTER_NAME": AUTHOR_NAME,
            "GIT_COMMITTER_EMAIL": AUTHOR_EMAIL,
            "GIT_AUTHOR_DATE": date,
            "GIT_COMMITTER_DATE": date,
            "GIT_CONFIG_GLOBAL": os.devnull,
            "GIT_CONFIG_SYSTEM": os.devnull,
        }
        completed = subprocess.run(
            ["git", "-C", str(self.root), *arguments], env=environment, capture_output=True, check=True
        )
        return completed.stdout.decode()
