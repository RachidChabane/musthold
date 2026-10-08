"""Typed wrapper around the pinned Shallnot binary: `init`, `gate --json-out` and the report schema.

The verdict is read from Shallnot's report; nothing here computes one. A report that does not conform to the schema
the binary itself prints is refused.
"""

from __future__ import annotations

import json
from collections.abc import Mapping
from dataclasses import dataclass
from pathlib import Path
from typing import Final

from specbench.application.ports import ProcessResult, ProcessRunner, SchemaChecker
from specbench.domain.canonical import JsonValue

EXIT_CLEAN: Final = 0
EXIT_BLOCKED: Final = 1
EXIT_TOOL_FAILURE: Final = 2
GATE_EXITS: Final = frozenset({EXIT_CLEAN, EXIT_BLOCKED})
SCHEMA_VERB: Final = "schema"
REPORT_SCHEMA: Final = "report"
HOOKS_NONE: Final = "none"


class ShallnotFailed(RuntimeError):
    """Shallnot ended with a tool failure, or wrote a report that breaks its own schema."""


@dataclass(frozen=True)
class ShallnotGate:
    exit_code: int
    report: JsonValue


def init_argv(binary: Path, tree: Path) -> list[str]:
    return [str(binary), "init", "--dir", str(tree), "--hooks", HOOKS_NONE]


def shallnot_gate_argv(binary: Path, config: Path, json_out: Path) -> list[str]:
    return [str(binary), "gate", "--config", str(config), "--json-out", str(json_out)]


def report_schema_argv(binary: Path) -> list[str]:
    return [str(binary), SCHEMA_VERB, REPORT_SCHEMA]


class ShallnotCli:
    def __init__(self, runner: ProcessRunner, binary: Path, checker: SchemaChecker) -> None:
        self._runner = runner
        self._binary = binary
        self._checker = checker
        self._schema: JsonValue | None = None

    def init(self, tree: Path, env: Mapping[str, str]) -> None:
        self._succeed(init_argv(self._binary, tree), tree, env)

    def gate(self, tree: Path, config: Path, json_out: Path, env: Mapping[str, str]) -> ShallnotGate:
        """Runs the project's tests and gates them under `config`; exit 0 and 1 are both a verdict."""
        argv = shallnot_gate_argv(self._binary, config, json_out)
        result = self._runner.run(argv, tree, env)
        if result.returncode not in GATE_EXITS:
            raise ShallnotFailed(_failure(argv, result))
        report: JsonValue = json.loads(json_out.read_text(encoding="utf-8"))
        problems = self._checker.problems(self.report_schema(tree, env), report)
        if problems:
            raise ShallnotFailed(f"the report breaks the schema of {self._binary.name}: {problems[:3]}")
        return ShallnotGate(result.returncode, report)

    def report_schema(self, cwd: Path, env: Mapping[str, str]) -> JsonValue:
        if self._schema is None:
            argv = report_schema_argv(self._binary)
            result = self._runner.run(argv, cwd, env)
            if result.returncode != EXIT_CLEAN:
                raise ShallnotFailed(_failure(argv, result))
            self._schema = json.loads(result.stdout)
        return self._schema

    def _succeed(self, argv: list[str], cwd: Path, env: Mapping[str, str]) -> None:
        result = self._runner.run(argv, cwd, env)
        if result.returncode != EXIT_CLEAN:
            raise ShallnotFailed(_failure(argv, result))


def _failure(argv: list[str], result: ProcessResult) -> str:
    return f"{' '.join(argv)} exited {result.returncode}: {(result.stderr or result.stdout).strip()[-500:]}"
