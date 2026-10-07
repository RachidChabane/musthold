"""Materializes trees from a local clone: `git archive` of a commit, patches applied in order, files overlaid."""

from __future__ import annotations

import io
import shutil
import subprocess
import tarfile
import tempfile
from collections.abc import Generator
from contextlib import contextmanager
from pathlib import Path
from typing import Final

from specbench.application.ports import TreeSpec

GIT: Final = "git"
APPLY_ARGUMENTS: Final = ("apply", "--whitespace=nowarn", "-")


class TreeBuildError(RuntimeError):
    """A commit that cannot be archived, or a patch that does not apply."""


class GitTreeFactory:
    """`TreeFactory` over a local clone that holds the base and merge commits of the task."""

    def __init__(self, clone: Path, scratch_root: Path | None = None) -> None:
        self._clone = clone
        self._scratch_root = scratch_root

    @contextmanager
    def materialize(self, spec: TreeSpec) -> Generator[Path]:
        scratch = Path(tempfile.mkdtemp(prefix="specbench-tree-", dir=self._scratch_root))
        try:
            self._extract(spec.commit, scratch)
            for patch in spec.patches:
                self._apply(patch, scratch)
            for relative, content in (spec.overlay or {}).items():
                target = scratch / relative
                target.parent.mkdir(parents=True, exist_ok=True)
                target.write_bytes(content)
            yield scratch
        finally:
            shutil.rmtree(scratch, ignore_errors=True)

    def _extract(self, commit: str, destination: Path) -> None:
        archive = subprocess.run(
            [GIT, "-C", str(self._clone), "archive", "--format=tar", commit], capture_output=True, check=False
        )
        if archive.returncode != 0:
            raise TreeBuildError(f"cannot archive {commit}: {archive.stderr.decode(errors='replace').strip()}")
        with tarfile.open(fileobj=io.BytesIO(archive.stdout), mode="r") as tar:
            tar.extractall(destination, filter="data")

    @staticmethod
    def _apply(patch: bytes, tree: Path) -> None:
        result = subprocess.run([GIT, *APPLY_ARGUMENTS], input=patch, cwd=tree, capture_output=True, check=False)
        if result.returncode != 0:
            raise TreeBuildError(f"patch does not apply: {result.stderr.decode(errors='replace').strip()}")
