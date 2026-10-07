"""Local bare mirrors of upstream repositories and the read-only questions asked of their history."""

from __future__ import annotations

from collections.abc import Sequence
from pathlib import Path
from typing import Final

from specbench.application.ports import ProcessRunner

GIT: Final = "git"
GITHUB_URL: Final = "https://github.com/{repository}.git"
MIRROR_SUFFIX: Final = ".git"
SHA_LENGTH: Final = 40


class GitError(RuntimeError):
    """A git command failed."""


class GitMirrors:
    """Bare full clones under one directory, one per repository; a mirror is never modified after it is cloned.

    A full clone, never a partial one: tests and builds run offline, so every object must be local.
    """

    def __init__(self, root: Path, runner: ProcessRunner) -> None:
        self._root = root
        self._runner = runner

    def path_of(self, repository: str) -> Path:
        return self._root / (repository.replace("/", "__") + MIRROR_SUFFIX)

    def ensure(self, repository: str) -> Path:
        path = self.path_of(repository)
        if not path.is_dir():
            self._root.mkdir(parents=True, exist_ok=True)
            result = self._runner.run(
                [GIT, "clone", "--bare", "--quiet", GITHUB_URL.format(repository=repository), str(path)]
            )
            if result.returncode != 0:
                raise GitError(f"cannot clone {repository}: {result.stderr.strip()}")
        return path


class GitHistory:
    """Read access to the commits of one bare clone."""

    def __init__(self, clone: Path, runner: ProcessRunner) -> None:
        self._clone = clone
        self._runner = runner

    def _git(self, *arguments: str) -> str:
        result = self._runner.run([GIT, "-C", str(self._clone), *arguments])
        if result.returncode != 0:
            raise GitError(f"git {' '.join(arguments)}: {result.stderr.strip()}")
        return result.stdout

    def has_commit(self, sha: str) -> bool:
        return self._runner.run([GIT, "-C", str(self._clone), "cat-file", "-e", f"{sha}^{{commit}}"]).returncode == 0

    def first_parent(self, sha: str) -> str:
        return self._git("rev-parse", f"{sha}^1").strip()

    def changed_paths(self, base: str, merge: str) -> frozenset[str]:
        return frozenset(self._git("diff", "--name-only", base, merge).splitlines())

    def unified_diff(self, base: str, merge: str, paths: Sequence[str]) -> str:
        return self._git("diff", "-U0", "--no-color", base, merge, "--", *paths)

    def file_at(self, sha: str, path: str) -> str | None:
        result = self._runner.run([GIT, "-C", str(self._clone), "show", f"{sha}:{path}"])
        return result.stdout if result.returncode == 0 else None
