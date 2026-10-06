"""Harness matrix runner: pytest with JUnit output, bounded concurrency (PREREG 6.4 F2)."""

from __future__ import annotations

import sys
import threading
from collections.abc import Callable, Mapping, Sequence
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

from pilot.application.ports import RunRequest
from pilot.adapters.junit import parse_outcomes
from pilot.adapters.process import ProcessRunner, child_environment
from pilot.adapters.project_copy import ProjectWorkspace
from pilot.domain.constants import (
    JUNIT_FILENAME,
    MAX_CONCURRENT_PYTEST,
    PYTEST_RUN_DEADLINE_SECONDS,
    PYTEST_TIMEOUT_SECONDS,
)
from pilot.domain.outcomes import RunResult

CopyRunner = Callable[[Path, Sequence[str]], RunResult]


def pytest_argv(timeout_seconds: int, junit_path: Path, nodeids: Sequence[str]) -> list[str]:
    return [
        sys.executable,
        "-m",
        "pytest",
        "-p",
        "no:cacheprovider",
        f"--timeout={timeout_seconds}",
        f"--junitxml={junit_path}",
        *nodeids,
    ]


class PytestJunitRunner:
    """Runs pytest inside one project copy; at most MAX_CONCURRENT_PYTEST at a time."""

    def __init__(
        self,
        process: ProcessRunner,
        semaphore: threading.BoundedSemaphore | None = None,
    ) -> None:
        self._process = process
        self._semaphore = semaphore or threading.BoundedSemaphore(MAX_CONCURRENT_PYTEST)

    def run(self, project_dir: Path, nodeids: Sequence[str] = ()) -> RunResult:
        junit_path = project_dir / JUNIT_FILENAME
        argv = pytest_argv(PYTEST_TIMEOUT_SECONDS, junit_path, nodeids)
        with self._semaphore:
            result = self._process.run(
                argv,
                project_dir,
                env=child_environment(),
                deadline_seconds=PYTEST_RUN_DEADLINE_SECONDS,
            )
        return RunResult(
            outcomes=parse_outcomes(junit_path, project_dir),
            returncode=result.returncode,
            deadline_exceeded=result.deadline_exceeded,
        )


class WorkspaceVariantRunner:
    """Implements the `VariantRunner` port over a workspace and a per-copy test runner."""

    def __init__(
        self,
        workspace: ProjectWorkspace,
        run_in_copy: CopyRunner,
        max_workers: int = MAX_CONCURRENT_PYTEST,
    ) -> None:
        self._workspace = workspace
        self._run_in_copy = run_in_copy
        self._max_workers = max_workers

    def run_many(self, requests: Mapping[str, RunRequest]) -> dict[str, RunResult]:
        with ThreadPoolExecutor(max_workers=self._max_workers) as pool:
            futures = {name: pool.submit(self._run_one, request) for name, request in requests.items()}
            return {name: future.result() for name, future in futures.items()}

    def _run_one(self, request: RunRequest) -> RunResult:
        with self._workspace.materialized(request.edit_set) as copy:
            return self._run_in_copy(copy, request.nodeids)
