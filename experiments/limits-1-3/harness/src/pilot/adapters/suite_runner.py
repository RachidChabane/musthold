"""Writer suites on the harness runner: a project copy whose tests are the writer's files (ADR 0006)."""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from pathlib import Path

from pilot.adapters.project_copy import ProjectWorkspace
from pilot.adapters.pytest_runner import PytestJunitRunner, WorkspaceVariantRunner
from pilot.application.ports import VariantRunner
from pilot.domain.constants import MAX_CONCURRENT_PYTEST, TESTS_DIRNAME
from pilot.domain.outcomes import RunResult

TEST_FILE_GLOB = "test_*.py"
ENCODING = "utf-8"


class _SuiteInstaller:
    """Replaces the project's tests with the writer's files, then runs pytest in the copy."""

    def __init__(self, files: Mapping[str, str], pytest_runner: PytestJunitRunner) -> None:
        self._files = dict(files)
        self._pytest = pytest_runner

    def run(self, project_dir: Path, nodeids: Sequence[str]) -> RunResult:
        for existing in (project_dir / TESTS_DIRNAME).glob(TEST_FILE_GLOB):
            existing.unlink()
        for relative, text in self._files.items():
            (project_dir / relative).write_text(text, encoding=ENCODING)
        return self._pytest.run(project_dir, nodeids)


class WriterSuiteRunners:
    """Implements the `SuiteRunners` port."""

    def __init__(self, workspace: ProjectWorkspace, pytest_runner: PytestJunitRunner) -> None:
        self._workspace = workspace
        self._pytest = pytest_runner

    def runner(self, files: Mapping[str, str]) -> VariantRunner:
        return WorkspaceVariantRunner(self._workspace, _SuiteInstaller(files, self._pytest).run, MAX_CONCURRENT_PYTEST)
