"""`OracleRunner` strategies: pytest through JUnit XML for Python, `go test -json` for Go.

A requested test that the run does not report (collection error, import error, build
failure) is MISSING, which counts as a failure, as ADR 0004's defect definition states. A parameter-free pytest id
stands for every parametrized case of that test, as a Go top-level test stands for its sub-tests.
"""

from __future__ import annotations

import json
import os
import tempfile
from collections.abc import Mapping, Sequence
from pathlib import Path
from typing import Final
from xml.etree import ElementTree

from specbench.application.ports import ProcessRunner
from specbench.domain.oracle import OracleTest, Outcome, requested_outcomes

NODE_SEPARATOR: Final = "::"
JUNIT_FILE: Final = "report.xml"
GO_MODULE_FILE: Final = "go.mod"
GO_PACKAGE_SEPARATOR: Final = "::"
GO_ACTIONS: Final = {"pass": Outcome.PASSED, "fail": Outcome.FAILED}
PYTHON_PATH_ENV: Final = "PYTHONPATH"
PYTHON_SOURCE_ROOTS: Final = ("", "src")
PYTEST_ARGUMENTS: Final = (
    "-m",
    "pytest",
    "-p",
    "no:cacheprovider",
    "-p",
    "no:randomly",
    "-q",
    "--continue-on-collection-errors",
)


def python_path_for(tree: Path) -> dict[str, str]:
    """The tree itself and its `src` directory on `PYTHONPATH`: the project is imported from the tree, never installed."""
    roots = [str(tree / root) if root else str(tree) for root in PYTHON_SOURCE_ROOTS]
    return {PYTHON_PATH_ENV: os.pathsep.join(roots)}


def _unique(items: Sequence[str]) -> list[str]:
    return list(dict.fromkeys(items))


def _node_id(tree: Path, classname: str, name: str) -> str:
    """The pytest node id of a JUnit testcase: the longest module path that exists, then classes, then the name."""
    parts = classname.split(".") if classname else []
    for length in range(len(parts), 0, -1):
        module = "/".join(parts[:length]) + ".py"
        if (tree / module).is_file():
            return NODE_SEPARATOR.join([module, *parts[length:], name])
    return NODE_SEPARATOR.join([*parts, name])


def parse_junit(tree: Path, xml_text: str) -> dict[str, Outcome]:
    """Outcome per node id from a pytest JUnit report."""
    outcomes: dict[str, Outcome] = {}
    for case in ElementTree.fromstring(xml_text).iter("testcase"):
        node = _node_id(tree, case.get("classname", ""), case.get("name", ""))
        if case.find("failure") is not None:
            outcomes[node] = Outcome.FAILED
        elif case.find("error") is not None:
            outcomes[node] = Outcome.ERROR
        elif case.find("skipped") is not None:
            outcomes[node] = Outcome.MISSING
        else:
            outcomes[node] = Outcome.PASSED
    return outcomes


class PytestOracleRunner:
    """Runs the test files of the requested nodes with the task environment's interpreter."""

    def __init__(self, python: Path, runner: ProcessRunner) -> None:
        self._python = python
        self._runner = runner

    def run(self, tree: Path, tests: Sequence[OracleTest]) -> Mapping[str, Outcome]:
        files = _unique([t.test_id.split(NODE_SEPARATOR)[0] for t in tests])
        with tempfile.TemporaryDirectory(prefix="specbench-junit-") as scratch:
            report = Path(scratch) / JUNIT_FILE
            self._runner.run(
                [str(self._python), *PYTEST_ARGUMENTS, f"--junitxml={report}", *files], tree, python_path_for(tree)
            )
            reported = parse_junit(tree, report.read_text(encoding="utf-8")) if report.is_file() else {}
        return requested_outcomes([t.test_id for t in tests], reported)


class PytestSuiteRunner:
    """Runs the whole suite of a tree with the task environment's interpreter and reports every test it ran."""

    def __init__(self, python: Path, runner: ProcessRunner) -> None:
        self._python = python
        self._runner = runner

    def run_all(self, tree: Path) -> Mapping[str, Outcome]:
        with tempfile.TemporaryDirectory(prefix="specbench-junit-") as scratch:
            report = Path(scratch) / JUNIT_FILE
            self._runner.run(
                [str(self._python), *PYTEST_ARGUMENTS, f"--junitxml={report}"], tree, python_path_for(tree)
            )
            return parse_junit(tree, report.read_text(encoding="utf-8")) if report.is_file() else {}


def _module_path(tree: Path) -> str:
    for line in (tree / GO_MODULE_FILE).read_text(encoding="utf-8").splitlines():
        if line.startswith("module "):
            return line.removeprefix("module ").strip()
    raise ValueError(f"{GO_MODULE_FILE} names no module")


def parse_go_events(module: str, output: str) -> dict[str, Outcome]:
    """Outcome per `<package dir>::<TestName>` from `go test -json`; sub-tests are folded into their top-level test."""
    outcomes: dict[str, Outcome] = {}
    for line in output.splitlines():
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue
        test, action, package = event.get("Test"), event.get("Action"), event.get("Package", "")
        if test is None or "/" in test or action not in GO_ACTIONS:
            continue
        directory = package.removeprefix(module).lstrip("/") or "."
        outcomes[f"{directory}{GO_PACKAGE_SEPARATOR}{test}"] = GO_ACTIONS[action]
    return outcomes


class GoTestOracleRunner:
    """Runs the requested top-level tests of each package with `-p 1`, so one build is in flight at a time."""

    def __init__(self, runner: ProcessRunner, environment: Mapping[str, str]) -> None:
        self._runner = runner
        self._environment = dict(environment)

    def run(self, tree: Path, tests: Sequence[OracleTest]) -> Mapping[str, Outcome]:
        module = _module_path(tree)
        by_directory: dict[str, list[str]] = {}
        for test in tests:
            directory, name = test.test_id.split(GO_PACKAGE_SEPARATOR)
            by_directory.setdefault(directory, []).append(name)
        reported: dict[str, Outcome] = {}
        for directory, names in sorted(by_directory.items()):
            pattern = "^(" + "|".join(names) + ")$"
            target = "./" + directory if directory != "." else "."
            result = self._runner.run(
                ["go", "test", "-json", "-count=1", "-p", "1", "-run", pattern, target], cwd=tree, env=self._environment
            )
            reported.update(parse_go_events(module, result.stdout))
        return {t.test_id: reported.get(t.test_id, Outcome.MISSING) for t in tests}


class GoSuiteRunner:
    """Runs `go test ./...` with one package build in flight and reports every top-level test."""

    def __init__(self, runner: ProcessRunner, environment: Mapping[str, str]) -> None:
        self._runner = runner
        self._environment = dict(environment)

    def run_all(self, tree: Path) -> Mapping[str, Outcome]:
        module = _module_path(tree)
        result = self._runner.run(
            ["go", "test", "-json", "-count=1", "-p", "1", "./..."], cwd=tree, env=self._environment
        )
        return parse_go_events(module, result.stdout)
