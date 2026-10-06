"""F3 differential test: reference versus mutant on Hypothesis-generated inputs (PREREG 6.4).

Inputs are generated once, deterministically, in the harness process; both trees then
evaluate the same cases in isolated interpreters, so the project code never shares a
process with the harness. Return values compare by value (Decimals numerically).
"""

from __future__ import annotations

import ast
import json
import sys
import tempfile
from datetime import datetime, timedelta
from decimal import Decimal
from pathlib import Path
from typing import Any

from hypothesis import HealthCheck, given, settings
from hypothesis import strategies as st

from pilot.adapters.process import ProcessRunner, child_environment
from pilot.adapters.project_copy import ProjectWorkspace
from pilot.domain.constants import (
    DIFFERENTIAL_EVAL_TIMEOUT_SECONDS,
    DIFFERENTIAL_MAX_AMOUNT,
    DIFFERENTIAL_MAX_DURATION_HOURS,
    DIFFERENTIAL_MAX_EXAMPLES,
    DIFFERENTIAL_MAX_YEAR,
    DIFFERENTIAL_MIN_YEAR,
    PACKAGE_NAME,
    PUBLIC_FUNCTIONS,
    SOURCE_DIRNAME,
)
from pilot.domain.edits import EditSet
from pilot.domain.mutants import DifferentialKind, DifferentialResult

MINUTES_PER_HOUR = 60
DECIMAL_PLACES = 2
LAST_MINUTE_OF_DAY = (23, 59)
SUPPRESSED_HEALTH_CHECKS = [
    HealthCheck.too_slow,
    HealthCheck.data_too_large,
    HealthCheck.filter_too_much,
    HealthCheck.large_base_example,
]
CASES_FILENAME = "cases.json"
ISOLATED_FLAG = "-I"
_DefNode = ast.FunctionDef | ast.AsyncFunctionDef
Params = list[tuple[str, str]]

EVALUATION_DRIVER = """
import importlib, json, sys
from datetime import datetime
from decimal import Decimal

package, function_name, cases_path, source_dir = sys.argv[1:5]
sys.path.insert(0, source_dir)
function = getattr(importlib.import_module(package), function_name)
converters = {"datetime": datetime.fromisoformat, "Decimal": Decimal, "bool": bool}


def encode(value):
    if isinstance(value, Decimal):
        return ["decimal", str(value.normalize())]
    if isinstance(value, str):
        return ["str", value]
    return ["repr", repr(value)]


outputs = []
document = json.load(open(cases_path))
for case in document["cases"]:
    kwargs = {name: converters[document["types"][name]](raw) for name, raw in case.items()}
    try:
        outputs.append({"ok": encode(function(**kwargs))})
    except Exception as error:
        outputs.append({"exc": type(error).__name__})
print(json.dumps(outputs))
"""

_MINUTE_DATETIMES = st.datetimes(
    min_value=datetime(DIFFERENTIAL_MIN_YEAR, 1, 1),
    max_value=datetime(DIFFERENTIAL_MAX_YEAR, 12, 31, *LAST_MINUTE_OF_DAY),
).map(lambda moment: moment.replace(second=0, microsecond=0))
_DURATION_MINUTES = st.integers(
    -DIFFERENTIAL_MAX_DURATION_HOURS * MINUTES_PER_HOUR,
    DIFFERENTIAL_MAX_DURATION_HOURS * MINUTES_PER_HOUR,
)
_AMOUNTS = st.decimals(min_value=0, max_value=DIFFERENTIAL_MAX_AMOUNT, places=DECIMAL_PLACES)


class UnsupportedAnnotation(Exception):
    pass


class DriverFailed(Exception):
    pass


@st.composite
def _case(draw: st.DrawFn, params: Params) -> dict[str, Any]:
    values: dict[str, Any] = {}
    anchor: datetime | None = None
    for name, annotation in params:
        if annotation == "datetime":
            if anchor is None:
                anchor = draw(_MINUTE_DATETIMES)
                values[name] = anchor
            else:
                values[name] = anchor + timedelta(minutes=draw(_DURATION_MINUTES))
        elif annotation == "bool":
            values[name] = draw(st.booleans())
        elif annotation == "Decimal":
            values[name] = draw(_AMOUNTS)
        else:
            raise UnsupportedAnnotation(f"{name}: {annotation}")
    return values


def _encode_case(case: dict[str, Any]) -> dict[str, Any]:
    return {
        name: value.isoformat() if isinstance(value, datetime) else str(value) if isinstance(value, Decimal) else value
        for name, value in case.items()
    }


def generate_cases(params: Params) -> list[dict[str, Any]]:
    cases: list[dict[str, Any]] = []

    @settings(
        derandomize=True,
        max_examples=DIFFERENTIAL_MAX_EXAMPLES,
        database=None,
        deadline=None,
        suppress_health_check=SUPPRESSED_HEALTH_CHECKS,
    )
    @given(_case(params))
    def collect(case: dict[str, Any]) -> None:
        cases.append(_encode_case(case))

    collect()
    return cases


def public_signatures(package_dir: Path, names: tuple[str, ...]) -> dict[str, Params]:
    found: dict[str, Params] = {}
    for module in sorted(package_dir.rglob("*.py")):
        for node in ast.parse(module.read_text(encoding="utf-8")).body:
            if isinstance(node, _DefNode) and node.name in names:
                found[node.name] = _params(node)
    return found


def _params(node: _DefNode) -> Params:
    arguments = node.args
    return [
        (arg.arg, ast.unparse(arg.annotation) if arg.annotation else "")
        for arg in (*arguments.posonlyargs, *arguments.args, *arguments.kwonlyargs)
    ]


def call_graph(package_dir: Path) -> dict[str, set[str]]:
    graph: dict[str, set[str]] = {}
    for module in sorted(package_dir.rglob("*.py")):
        for node in ast.walk(ast.parse(module.read_text(encoding="utf-8"))):
            if isinstance(node, _DefNode):
                graph.setdefault(node.name, set()).update(_called_names(node))
    return graph


def _called_names(node: ast.AST) -> set[str]:
    names: set[str] = set()
    for child in ast.walk(node):
        if isinstance(child, ast.Call):
            target = child.func
            if isinstance(target, ast.Name):
                names.add(target.id)
            elif isinstance(target, ast.Attribute):
                names.add(target.attr)
    return names


def reachable(graph: dict[str, set[str]], start: str) -> set[str]:
    seen = {start}
    pending = [start]
    while pending:
        for callee in graph.get(pending.pop(), ()):
            if callee not in seen:
                seen.add(callee)
                pending.append(callee)
    return seen


def edited_names(edit_set: EditSet) -> set[str]:
    return {part for edit in edit_set.edits for part in edit.qualname.split(".")}


class HypothesisDifferential:
    """Implements the `DifferentialTester` port."""

    def __init__(
        self,
        workspace: ProjectWorkspace,
        process: ProcessRunner,
        project_root: Path,
        public_functions: tuple[str, ...] = PUBLIC_FUNCTIONS,
        package: str = PACKAGE_NAME,
    ) -> None:
        self._workspace = workspace
        self._process = process
        self._package = package
        self._package_dir = project_root / SOURCE_DIRNAME / package
        self._public = public_functions
        self._cases: dict[str, list[dict[str, Any]]] = {}
        self._reference: dict[str, list[dict[str, Any]]] = {}

    def compare(self, edit_set: EditSet) -> DifferentialResult:
        touched = self._touched(edit_set)
        if not touched:
            return DifferentialResult(DifferentialKind.SKIPPED, reason="no public function touched")
        reasons: list[str] = []
        with self._workspace.materialized(edit_set) as mutant_copy:
            for function in touched:
                try:
                    cases = self._cases_for(function)
                except UnsupportedAnnotation as error:
                    reasons.append(f"{function}: unsupported annotation {error}")
                    continue
                try:
                    difference = self._first_difference(function, cases, mutant_copy)
                except DriverFailed as error:
                    reasons.append(f"{function}: driver failed: {error}")
                    continue
                if difference is not None:
                    return difference
        if reasons:
            return DifferentialResult(DifferentialKind.SKIPPED, reason="; ".join(reasons))
        return DifferentialResult(DifferentialKind.NO_DIFF_FOUND)

    def _touched(self, edit_set: EditSet) -> list[str]:
        graph = call_graph(self._package_dir)
        names = edited_names(edit_set)
        return [f for f in self._public if f in graph and names & reachable(graph, f)]

    def _cases_for(self, function: str) -> list[dict[str, Any]]:
        if function not in self._cases:
            params = public_signatures(self._package_dir, self._public)[function]
            self._cases[function] = generate_cases(params)
        return self._cases[function]

    def _first_difference(
        self, function: str, cases: list[dict[str, Any]], mutant_copy: Path
    ) -> DifferentialResult | None:
        if function not in self._reference:
            with self._workspace.materialized() as reference_copy:
                self._reference[function] = self._evaluate(reference_copy, function, cases)
        mutant = self._evaluate(mutant_copy, function, cases)
        for case, original, changed in zip(cases, self._reference[function], mutant, strict=True):
            if original != changed:
                return DifferentialResult(
                    DifferentialKind.DIFFERS,
                    function=function,
                    args=case,
                    original_out=original,
                    mutant_out=changed,
                )
        return None

    def _evaluate(
        self, project_copy: Path, function: str, cases: list[dict[str, Any]]
    ) -> list[dict[str, Any]]:
        types = dict(public_signatures(self._package_dir, self._public)[function])
        with tempfile.TemporaryDirectory() as scratch:
            cases_path = Path(scratch) / CASES_FILENAME
            cases_path.write_text(json.dumps({"types": types, "cases": cases}), encoding="utf-8")
            argv = [sys.executable, ISOLATED_FLAG, "-c", EVALUATION_DRIVER, self._package,
                    function, str(cases_path), SOURCE_DIRNAME]
            result = self._process.run(
                argv,
                project_copy,
                env=child_environment(),
                deadline_seconds=DIFFERENTIAL_EVAL_TIMEOUT_SECONDS,
            )
        if result.returncode != 0:
            raise DriverFailed(result.stderr.strip() or f"exit {result.returncode}")
        return json.loads(result.stdout)
