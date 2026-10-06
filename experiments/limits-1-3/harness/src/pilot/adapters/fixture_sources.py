"""Reads the fixture project for the pipeline: spec, source tree and tests."""

from __future__ import annotations

from collections.abc import Mapping
from pathlib import Path

from pilot.domain.constants import SOURCE_DIRNAME, TESTS_DIRNAME

SPEC_GLOB = "*.md"
SOURCE_GLOB = "*.py"
TEST_GLOB = "test_*.py"
IGNORED_PARTS = ("__pycache__",)


class DirectoryFixtureSources:
    """Implements the `FixtureSources` port; paths are relative to the project root."""

    def __init__(self, project: Path, spec_dir: Path) -> None:
        self._project = project
        self._spec_dir = spec_dir

    def spec_text(self) -> str:
        return "\n".join(p.read_text(encoding="utf-8") for p in sorted(self._spec_dir.glob(SPEC_GLOB)))

    def source_files(self) -> Mapping[str, str]:
        return self._read_tree(self._project / SOURCE_DIRNAME, SOURCE_GLOB)

    def test_files(self) -> Mapping[str, str]:
        return self._read_tree(self._project / TESTS_DIRNAME, TEST_GLOB)

    def _read_tree(self, directory: Path, pattern: str) -> dict[str, str]:
        return {
            path.relative_to(self._project).as_posix(): path.read_text(encoding="utf-8")
            for path in sorted(directory.rglob(pattern))
            if not any(part in IGNORED_PARTS for part in path.parts)
        }
