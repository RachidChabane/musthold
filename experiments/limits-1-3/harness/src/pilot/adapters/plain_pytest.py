"""Plain pytest path of `verify-labels`: `pytest -q -rA`, parsed from the summary lines.

Skipped tests print no node id in that summary, so they surface as missing outcomes.

Deliberately independent of the JUnit matrix runner so that the runner cross-check
compares two separate measurement paths.
"""

from __future__ import annotations

import re
import shutil
import sys
from collections.abc import Sequence
from pathlib import Path

from pilot.adapters.process import ProcessRunner, child_environment
from pilot.domain.constants import (
    PYTEST_RUN_DEADLINE_SECONDS,
    PYTEST_TIMEOUT_SECONDS,
    TESTS_DIRNAME,
)
from pilot.domain.outcomes import RunResult, TestOutcome

_SUMMARY_HEADER = "short test summary info"
_SUMMARY_LINE = re.compile(r"^(PASSED|FAILED|ERROR)\s+(\S+)")
_BY_WORD = {
    "PASSED": TestOutcome.PASSED,
    "FAILED": TestOutcome.FAILED,
    "ERROR": TestOutcome.ERROR,
}


def parse_summary(output: str) -> dict[str, TestOutcome]:
    outcomes: dict[str, TestOutcome] = {}
    in_summary = False
    for line in output.splitlines():
        if _SUMMARY_HEADER in line:
            in_summary = True
            continue
        match = _SUMMARY_LINE.match(line) if in_summary else None
        if match:
            outcomes[match.group(2)] = _BY_WORD[match.group(1)]
    return outcomes


class PlainPytestRunner:
    """Runs `pytest -q -rA` in a project copy, with extra test files copied into `tests/`."""

    def __init__(self, process: ProcessRunner, extra_test_files: Sequence[Path] = ()) -> None:
        self._process = process
        self._extra_test_files = tuple(extra_test_files)

    def __call__(self, project_dir: Path, nodeids: Sequence[str] = ()) -> RunResult:
        for extra in self._extra_test_files:
            shutil.copy(extra, project_dir / TESTS_DIRNAME / extra.name)
        argv = [
            sys.executable,
            "-m",
            "pytest",
            "-q",
            "-rA",
            "-p",
            "no:cacheprovider",
            f"--timeout={PYTEST_TIMEOUT_SECONDS}",
            *nodeids,
        ]
        result = self._process.run(
            argv,
            project_dir,
            env=child_environment(),
            deadline_seconds=PYTEST_RUN_DEADLINE_SECONDS,
        )
        return RunResult(
            outcomes=parse_summary(result.stdout),
            returncode=result.returncode,
            deadline_exceeded=result.deadline_exceeded,
        )
