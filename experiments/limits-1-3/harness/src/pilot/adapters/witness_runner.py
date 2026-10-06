"""Witness execution: one concrete call on the reference and on the mutant, in isolated interpreters (PREREG 6.5)."""

from __future__ import annotations

import json
import sys
import tempfile
from collections.abc import Sequence
from pathlib import Path

from pilot.adapters.differential import CASES_FILENAME, EVALUATION_DRIVER, ISOLATED_FLAG
from pilot.adapters.process import ProcessResult, ProcessRunner, child_environment
from pilot.adapters.project_copy import ProjectWorkspace
from pilot.application.ports import WitnessExecution
from pilot.domain.constants import PACKAGE_NAME, SOURCE_DIRNAME
from pilot.domain.edits import EditSet
from pilot.domain.pipeline_constants import WITNESS_TIMEOUT_SECONDS
from pilot.domain.witness import Output, Witness, WitnessInvalid, output_from_driver


class SubprocessWitnessExecutor:
    """Implements the `WitnessExecutor` port; arguments are converted by type and never evaluated."""

    def __init__(self, workspace: ProjectWorkspace, process: ProcessRunner, package: str = PACKAGE_NAME) -> None:
        self._workspace = workspace
        self._process = process
        self._package = package

    def execute(self, edit_set: EditSet, witnesses: Sequence[Witness]) -> list[WitnessExecution]:
        with self._workspace.materialized() as reference, self._workspace.materialized(edit_set) as mutant:
            return [self._execute_one(witness, reference, mutant) for witness in witnesses]

    def _execute_one(self, witness: Witness, reference: Path, mutant: Path) -> WitnessExecution:
        try:
            call = witness.call()
        except WitnessInvalid as error:
            return WitnessExecution(None, None, f"invalid witness: {error}")
        document = {"types": dict(call.types), "cases": [dict(call.arguments)]}
        outputs: list[Output] = []
        for tree in (reference, mutant):
            output, error = self._evaluate(tree, call.function, document)
            if output is None:
                return WitnessExecution(None, None, error)
            outputs.append(output)
        return WitnessExecution(outputs[0], outputs[1], None)

    def _evaluate(self, tree: Path, function: str, document: dict[str, object]) -> tuple[Output | None, str | None]:
        with tempfile.TemporaryDirectory() as scratch:
            cases = Path(scratch) / CASES_FILENAME
            cases.write_text(json.dumps(document), encoding="utf-8")
            argv = [sys.executable, ISOLATED_FLAG, "-c", EVALUATION_DRIVER, self._package,
                    function, str(cases), SOURCE_DIRNAME]
            result = self._process.run(
                argv, tree, env=child_environment(), deadline_seconds=WITNESS_TIMEOUT_SECONDS
            )
        return self._parse(result)

    @staticmethod
    def _parse(result: ProcessResult) -> tuple[Output | None, str | None]:
        if result.deadline_exceeded:
            return None, f"timeout after {WITNESS_TIMEOUT_SECONDS}s"
        if result.returncode != 0:
            lines = result.stderr.strip().splitlines()
            return None, lines[-1] if lines else f"exit {result.returncode}"
        return output_from_driver(json.loads(result.stdout)[0]), None
