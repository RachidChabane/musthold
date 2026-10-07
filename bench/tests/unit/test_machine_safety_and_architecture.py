from __future__ import annotations

import ast
from collections.abc import Mapping, Sequence
from pathlib import Path

import pytest

import specbench
from specbench.adapters.process import FREE_PERCENT_LINE, MachineUnsafe, MemoryFloorRunner
from specbench.application.ports import ProcessResult

SOURCE_ROOT = Path(specbench.__file__).parent
GATE_RULE_NAMES = ("gate_", "combine_", "decide_", "verdict_of", "weakest_")


class CountingRunner:
    def __init__(self) -> None:
        self.calls = 0

    def run(self, argv: Sequence[str], cwd: Path | None = None, env: Mapping[str, str] | None = None) -> ProcessResult:
        self.calls += 1
        return ProcessResult(0, "", "")


def test_a_command_is_refused_below_the_free_memory_floor() -> None:
    inner = CountingRunner()
    with pytest.raises(MachineUnsafe):
        MemoryFloorRunner(inner, lambda: 39).run(["go", "build"])
    assert inner.calls == 0
    MemoryFloorRunner(inner, lambda: 40).run(["go", "build"])
    assert inner.calls == 1


def test_memory_pressure_output_is_parsed() -> None:
    match = FREE_PERCENT_LINE.search(
        "The system has 25769803776 (1572864 pages)\nSystem-wide memory free percentage: 71%\n"
    )
    assert match is not None and match.group(1) == "71"


def python_sources() -> list[Path]:
    return sorted(path for path in SOURCE_ROOT.rglob("*.py"))


def test_no_gate_rule_is_implemented_in_specbench() -> None:
    """Verdicts are read from Shallnot reports; no function or class here decides one."""
    offenders: list[str] = []
    for path in python_sources():
        for node in ast.walk(ast.parse(path.read_text())):
            if isinstance(node, (ast.FunctionDef, ast.ClassDef)) and node.name.startswith(GATE_RULE_NAMES):
                offenders.append(f"{path.name}:{node.name}")
    assert offenders == []


def test_specbench_never_imports_the_pilot() -> None:
    for path in python_sources():
        for node in ast.walk(ast.parse(path.read_text())):
            names = (
                [a.name for a in node.names]
                if isinstance(node, ast.Import)
                else [node.module or ""]
                if isinstance(node, ast.ImportFrom)
                else []
            )
            assert not any(name == "pilot" or name.startswith("pilot.") for name in names), path


def test_the_clock_is_read_in_one_adapter_only() -> None:
    readers = [p.name for p in python_sources() if "datetime.now(" in p.read_text() or "time.time(" in p.read_text()]
    assert readers == ["clock.py"]
