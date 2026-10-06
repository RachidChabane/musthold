"""Shallnot adapter: the only source of declared links D(t) (PREREG 2.6 and 5)."""

from __future__ import annotations

import json
import shutil
from pathlib import Path
from typing import Any

from pilot.adapters.process import ProcessRunner, child_environment
from pilot.adapters.project_copy import ProjectWorkspace
from pilot.adapters.pytest_runner import PytestJunitRunner
from pilot.domain.constants import (
    JUNIT_FILENAME,
    SPEC_DIRNAME,
    TESTS_DIRNAME,
    TRACEABILITY_FILENAME,
)
from pilot.domain.traceability import TraceabilityReport, report_from_json

SHALLNOT_TOOL_FAILURE = 2


class ShallnotError(RuntimeError):
    pass


def shallnot_argv(junit_path: Path, json_out: Path) -> list[str]:
    return [
        "shallnot",
        "check",
        "--no-config",
        "--strict",
        "--specs",
        SPEC_DIRNAME,
        "--focus",
        SPEC_DIRNAME,
        "--tests",
        TESTS_DIRNAME,
        "--results",
        str(junit_path),
        "--format",
        "json",
        "--json-out",
        str(json_out),
    ]


parse_report = report_from_json


class ShallnotTraceability:
    """Runs the fixture on the reference, then `shallnot check` on its JUnit results."""

    def __init__(
        self,
        workspace: ProjectWorkspace,
        runner: PytestJunitRunner,
        process: ProcessRunner,
        output_dir: Path | None = None,
    ) -> None:
        self._workspace = workspace
        self._runner = runner
        self._process = process
        self._output_dir = output_dir

    def check(self) -> TraceabilityReport:
        return report_from_json(self.raw_report())

    def raw_report(self) -> dict[str, Any]:
        with self._workspace.materialized() as copy:
            self._runner.run(copy)
            json_out = copy / TRACEABILITY_FILENAME
            result = self._process.run(
                shallnot_argv(copy / JUNIT_FILENAME, json_out), copy, env=child_environment()
            )
            if result.returncode == SHALLNOT_TOOL_FAILURE or not json_out.is_file():
                raise ShallnotError(f"shallnot failed ({result.returncode}): {result.stderr.strip()}")
            self._keep(copy)
            return json.loads(json_out.read_text(encoding="utf-8"))

    def _keep(self, copy: Path) -> None:
        if self._output_dir is None:
            return
        self._output_dir.mkdir(parents=True, exist_ok=True)
        for name in (TRACEABILITY_FILENAME, JUNIT_FILENAME):
            shutil.copy(copy / name, self._output_dir / name)
