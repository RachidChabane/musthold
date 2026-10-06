"""Temporary project copies under a run directory."""

from __future__ import annotations

import shutil
import tempfile
import uuid
from collections.abc import Iterator
from contextlib import contextmanager
from pathlib import Path

from pilot.adapters.ast_edits import AstEditApplier
from pilot.domain.edits import EditSet

_IGNORED_NAMES = shutil.ignore_patterns(
    "__pycache__", ".hypothesis", ".pytest_cache", "*.pyc", "*.egg-info", ".venv"
)
_WORK_PREFIX = "pilot-work-"


def copy_project(source: Path, destination: Path) -> None:
    shutil.copytree(source, destination, ignore=_IGNORED_NAMES)


class ProjectWorkspace:
    """Materialises (project + optional edit set) as a throwaway copy."""

    def __init__(self, project_root: Path, work_root: Path, applier: AstEditApplier) -> None:
        self._project_root = project_root
        self._work_root = work_root
        self._applier = applier

    @contextmanager
    def materialized(self, edit_set: EditSet | None = None) -> Iterator[Path]:
        self._work_root.mkdir(parents=True, exist_ok=True)
        copy = self._work_root / uuid.uuid4().hex
        copy_project(self._project_root, copy)
        try:
            if edit_set is not None:
                self._applier.apply(copy, edit_set)
            yield copy
        finally:
            shutil.rmtree(copy, ignore_errors=True)


@contextmanager
def temporary_work_root() -> Iterator[Path]:
    path = Path(tempfile.mkdtemp(prefix=_WORK_PREFIX))
    try:
        yield path
    finally:
        shutil.rmtree(path, ignore_errors=True)
