"""Run provenance: the pre-registration commit and content digests of the harness and the fixture."""

from __future__ import annotations

import hashlib
from pathlib import Path

from pilot.adapters.process import ProcessRunner
from pilot.domain.constants import PREREG_DIRNAME

PREREG_FILE = "PREREG.md"
IGNORED_PARTS = ("__pycache__", ".hypothesis", ".pytest_cache", ".venv")
IGNORED_SUFFIXES = (".pyc",)


class PreregNotCommitted(Exception):
    pass


class GitProvenance:
    """Implements the `ProvenanceProbe` port."""

    def __init__(self, process: ProcessRunner, root: Path, harness_source: Path, fixture: Path) -> None:
        self._process = process
        self._root = root
        self._harness_source = harness_source
        self._fixture = fixture

    def prereg_commit(self) -> str:
        argv = ["git", "log", "-1", "--format=%H", "--", f"{PREREG_DIRNAME}/{PREREG_FILE}"]
        commit = self._process.run(argv, self._root).stdout.strip()
        if not commit:
            raise PreregNotCommitted(f"{PREREG_DIRNAME}/{PREREG_FILE} is not committed")
        return commit

    def harness_digest(self) -> str:
        return tree_digest(self._harness_source)

    def fixture_digest(self) -> str:
        return tree_digest(self._fixture)


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
