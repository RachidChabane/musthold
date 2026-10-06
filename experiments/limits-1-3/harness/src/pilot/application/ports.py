"""Ports: the interfaces the use cases need from the outside world."""

from __future__ import annotations

from collections.abc import Mapping
from dataclasses import dataclass
from pathlib import Path
from typing import Protocol

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
    def save(self, pin: ModelPin, baseline: CanaryBaseline, cli_version: str) -> None: ...


class TestSourceReader(Protocol):
    __test__ = False

    def read(self) -> Mapping[str, str]: ...

    def read_conftest(self) -> str | None: ...
