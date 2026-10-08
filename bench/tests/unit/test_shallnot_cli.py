from __future__ import annotations

import json
from collections.abc import Mapping, Sequence
from pathlib import Path

import pytest

from specbench.adapters.shallnot_cli import (
    ShallnotCli,
    ShallnotFailed,
    init_argv,
    report_schema_argv,
    shallnot_gate_argv,
)
from specbench.application.ports import ProcessResult
from specbench.domain.canonical import JsonValue

BINARY = Path("/tools/bin/shallnot")
REPORT: dict[str, object] = {"schema_version": "1.2", "verdict": "pass", "exit_code": 0}


class WritingRunner:
    """Answers `shallnot schema report` with a schema and `shallnot gate` by writing its `--json-out` file."""

    def __init__(self, gate_exit: int, report: dict[str, object]) -> None:
        self.gate_exit = gate_exit
        self.report = report
        self.calls: list[list[str]] = []

    def run(self, argv: Sequence[str], cwd: Path | None = None, env: Mapping[str, str] | None = None) -> ProcessResult:
        self.calls.append(list(argv))
        if argv[1] == "schema":
            return ProcessResult(0, json.dumps({"type": "object"}), "")
        if argv[1] == "gate":
            Path(argv[argv.index("--json-out") + 1]).write_text(json.dumps(self.report))
        return ProcessResult(self.gate_exit if argv[1] == "gate" else 0, "", "boom")


class Checker:
    def __init__(self, problems: list[str]) -> None:
        self._problems = problems

    def problems(self, schema: JsonValue, document: JsonValue) -> list[str]:
        return self._problems


def test_the_argv_of_init_gate_and_schema() -> None:
    assert init_argv(BINARY, Path("/t")) == ["/tools/bin/shallnot", "init", "--dir", "/t", "--hooks", "none"]
    assert shallnot_gate_argv(BINARY, Path("/t/shallnot.al.yaml"), Path("/r.json")) == [
        "/tools/bin/shallnot", "gate", "--config", "/t/shallnot.al.yaml", "--json-out", "/r.json",
    ]  # fmt: skip
    assert report_schema_argv(BINARY) == ["/tools/bin/shallnot", "schema", "report"]


@pytest.mark.parametrize("exit_code", [0, 1])
def test_a_clean_and_a_blocked_gate_both_give_a_report(tmp_path: Path, exit_code: int) -> None:
    runner = WritingRunner(exit_code, REPORT)
    gate = ShallnotCli(runner, BINARY, Checker([])).gate(tmp_path, tmp_path / "c.yaml", tmp_path / "r.json", {})
    assert gate.exit_code == exit_code
    assert gate.report == REPORT


def test_a_tool_failure_of_the_gate_is_an_error_with_the_tail_of_its_output(tmp_path: Path) -> None:
    runner = WritingRunner(2, REPORT)
    with pytest.raises(ShallnotFailed, match="exited 2: boom"):
        ShallnotCli(runner, BINARY, Checker([])).gate(tmp_path, tmp_path / "c.yaml", tmp_path / "r.json", {})


def test_a_report_that_breaks_the_schema_of_the_binary_is_refused(tmp_path: Path) -> None:
    runner = WritingRunner(0, REPORT)
    with pytest.raises(ShallnotFailed, match="breaks the schema"):
        ShallnotCli(runner, BINARY, Checker(["/verdict: missing"])).gate(
            tmp_path, tmp_path / "c.yaml", tmp_path / "r.json", {}
        )


def test_the_schema_is_asked_of_the_binary_once(tmp_path: Path) -> None:
    runner = WritingRunner(0, REPORT)
    cli = ShallnotCli(runner, BINARY, Checker([]))
    for _ in range(2):
        cli.gate(tmp_path, tmp_path / "c.yaml", tmp_path / "r.json", {})
    assert [call[1] for call in runner.calls].count("schema") == 1
