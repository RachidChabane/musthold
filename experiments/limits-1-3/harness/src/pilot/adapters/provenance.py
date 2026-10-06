"""Run provenance: the peeled pre-registration commits and content digests of the harness, the fixture and the imports."""

from __future__ import annotations

import hashlib
from pathlib import Path

from pilot.adapters.git_drift import preregistration_commit
from pilot.adapters.process import ProcessRunner
from pilot.domain.preregistration import PREREG_TAG, SUPERSEDED_PREREG_TAG
from pilot.domain.import_manifest import IMPORTS_FILENAME

IGNORED_PARTS = ("__pycache__", ".hypothesis", ".pytest_cache", ".venv")
IGNORED_SUFFIXES = (".pyc",)


class GitProvenance:
    """Implements the `ProvenanceProbe` port; tag commits are peeled, never read from the file history."""

    def __init__(
        self,
        process: ProcessRunner,
        root: Path,
        harness_source: Path,
        fixture: Path,
        run_dir: Path | None = None,
    ) -> None:
        self._process = process
        self._root = root
        self._harness_source = harness_source
        self._fixture = fixture
        self._run_dir = run_dir

    def prereg_tag(self) -> str:
        return PREREG_TAG

    def prereg_commit(self) -> str:
        return preregistration_commit(self._root, PREREG_TAG, self._process)

    def superseded_prereg_tag(self) -> str:
        return SUPERSEDED_PREREG_TAG

    def superseded_prereg_commit(self) -> str:
        return preregistration_commit(self._root, SUPERSEDED_PREREG_TAG, self._process)

    def harness_digest(self) -> str:
        return tree_digest(self._harness_source)

    def fixture_digest(self) -> str:
        return tree_digest(self._fixture)

    def imports_digest(self) -> str | None:
        if self._run_dir is None:
            return None
        path = self._run_dir / IMPORTS_FILENAME
        return hashlib.sha256(path.read_bytes()).hexdigest() if path.is_file() else None


def tree_digest(directory: Path) -> str:
    digest = hashlib.sha256()
    for path in sorted(p for p in directory.rglob("*") if p.is_file() and not _ignored(p)):
        digest.update(path.relative_to(directory).as_posix().encode())
        digest.update(b"\0")
        digest.update(path.read_bytes())
        digest.update(b"\0")
    return digest.hexdigest()


def _ignored(path: Path) -> bool:
    return any(part in IGNORED_PARTS for part in path.parts) or path.suffix in IGNORED_SUFFIXES
