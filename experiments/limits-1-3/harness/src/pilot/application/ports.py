"""Ports: the interfaces the use cases need from the outside world."""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Protocol

from pilot.domain.call_type import CallType
from pilot.domain.edits import EditSet
from pilot.domain.mutants import DifferentialResult
from pilot.domain.model_call import (
    CanaryBaseline,
    Cli,
    ModelOutcome,
    ModelPin,
    ModelRequest,
)
from pilot.domain.outcomes import RunResult
from pilot.domain.traceability import TraceabilityReport
from pilot.domain.witness import Output, Witness


@dataclass(frozen=True)
class RunRequest:
    """One test-suite run: the reference (no edit set) or an edited copy, optionally scoped."""

    edit_set: EditSet | None = None
    nodeids: tuple[str, ...] = ()


class VariantRunner(Protocol):
    """Runs the project's test suite for several requests with bounded concurrency."""

    def run_many(self, requests: Mapping[str, RunRequest]) -> dict[str, RunResult]: ...


class EditPreview(Protocol):
    """Static view of an edit set against the reference tree."""

    def preview(self, project_root: Path, edit_set: EditSet) -> Mapping[str, tuple[str, str]]: ...


class ImportSmoke(Protocol):
    def passes(self, edit_set: EditSet) -> bool: ...


class DifferentialTester(Protocol):
    def compare(self, edit_set: EditSet) -> DifferentialResult: ...


class TraceabilityPort(Protocol):
    def check(self) -> TraceabilityReport: ...


class ModelPort(Protocol):
    def call(self, request: ModelRequest, baseline: CanaryBaseline | None) -> ModelOutcome: ...


class CliVersionProbe(Protocol):
    def installed_version(self, cli: Cli) -> str: ...


class SchemaValidator(Protocol):
    def violations(self, schema_text: str, payload: object) -> list[str]: ...


class CanaryStore(Protocol):
    def save(
        self, pin: ModelPin, call_type: CallType, baseline: CanaryBaseline, cli_version: str
    ) -> None: ...


class RunStore(Protocol):
    """Reads and writes files of one run directory (paths relative to `runs/<run-id>/`)."""

    def exists(self, relative: str) -> bool: ...

    def read_text(self, relative: str) -> str: ...

    def write_text(self, relative: str, text: str) -> None: ...

    def append_text(self, relative: str, text: str) -> None: ...

    def read_json(self, relative: str) -> Any: ...

    def write_json(self, relative: str, value: Any) -> None: ...

    def read_jsonl(self, relative: str) -> list[dict[str, Any]]: ...

    def write_jsonl(self, relative: str, rows: Sequence[Mapping[str, Any]], key: Sequence[str]) -> None: ...

    def list_files(self, relative: str) -> list[str]: ...


class FixtureSources(Protocol):
    """Reads the fixture project: spec, source tree and tests, keyed by project-relative path."""

    def spec_text(self) -> str: ...

    def source_files(self) -> Mapping[str, str]: ...

    def test_files(self) -> Mapping[str, str]: ...


class PromptLibrary(Protocol):
    """The pre-registered templates, schemas and system prompt."""

    def template(self, call_type: CallType) -> str: ...

    def schema(self, call_type: CallType) -> str: ...

    def system_prompt(self) -> str: ...

    def codebook(self) -> str: ...


class WitnessExecutor(Protocol):
    """Runs witness calls on the reference and on one mutant, each call bounded in time."""

    def execute(
        self, edit_set: EditSet, witnesses: Sequence[Witness]
    ) -> list[WitnessExecution]: ...


class TestSourceReader(Protocol):
    __test__ = False

    def read(self) -> Mapping[str, str]: ...

    def read_conftest(self) -> str | None: ...


@dataclass(frozen=True)
class WitnessExecution:
    reference: Output | None
    mutant: Output | None
    error: str | None
