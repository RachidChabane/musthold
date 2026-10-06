"""`prereg/models.lock.json`, installed-CLI probing, schema validation and canary storage."""

from __future__ import annotations

import json
import re
from collections.abc import Mapping
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

import jsonschema

from pilot.adapters.process import ProcessRunner
from pilot.domain.call_type import CallType
from pilot.domain.model_call import CanaryBaseline, Cli, ModelPin

VERSION_PATTERN = re.compile(r"\d+\.\d+\.\d+")
VERSION_FLAG = "--version"


@dataclass(frozen=True)
class ModelsLock:
    cli_versions: dict[Cli, str]
    pins: tuple[ModelPin, ...]
    roles: Mapping[ModelPin, tuple[str, ...]] = field(default_factory=dict)


def load_models_lock(path: Path) -> ModelsLock:
    raw = json.loads(path.read_text(encoding="utf-8"))
    return ModelsLock(
        cli_versions={Cli(name): version for name, version in raw["cli_versions"].items()},
        pins=tuple(ModelPin(entry["model"], Cli(entry["cli"])) for entry in raw["models"]),
        roles={
            ModelPin(entry["model"], Cli(entry["cli"])): tuple(entry.get("roles", ()))
            for entry in raw["models"]
        },
    )


class InstalledCliVersions:
    """Implements the `CliVersionProbe` port by running `<cli> --version`."""

    def __init__(self, process: ProcessRunner, cwd: Path) -> None:
        self._process = process
        self._cwd = cwd

    def installed_version(self, cli: Cli) -> str:
        result = self._process.run([cli.value, VERSION_FLAG], self._cwd)
        match = VERSION_PATTERN.search(result.stdout)
        if result.returncode != 0 or match is None:
            raise RuntimeError(f"cannot read {cli.value} version: {result.stderr.strip()}")
        return match.group(0)


class JsonSchemaValidator:
    """Implements the `SchemaValidator` port with JSON Schema 2020-12."""

    def violations(self, schema_text: str, payload: Any) -> list[str]:
        validator = jsonschema.Draft202012Validator(json.loads(schema_text))
        return sorted(error.message for error in validator.iter_errors(payload))


class DirectoryCanaryStore:
    """Implements the `CanaryStore` port: one JSON file per (pin, call type) under `runs/<run-id>/canary/`."""

    def __init__(self, directory: Path) -> None:
        self._directory = directory

    def save(
        self, pin: ModelPin, call_type: CallType, baseline: CanaryBaseline, cli_version: str
    ) -> None:
        self._directory.mkdir(parents=True, exist_ok=True)
        record = {
            "input_tokens": baseline.input_tokens,
            "num_turns": baseline.num_turns,
            "cli_version": cli_version,
        }
        path = self._directory / f"{pin.key}__{call_type.value}.json"
        path.write_text(json.dumps(record, indent=2, sort_keys=True), encoding="utf-8")


def load_baselines(directory: Path) -> dict[tuple[str, CallType], CanaryBaseline]:
    """Baselines per (pin key, call type) from a run's canary directory."""
    found: dict[tuple[str, CallType], CanaryBaseline] = {}
    for path in sorted(directory.glob("*.json")) if directory.is_dir() else []:
        pin_key, call_type = path.stem.rsplit("__", 1)
        record = json.loads(path.read_text(encoding="utf-8"))
        found[(pin_key, CallType(call_type))] = CanaryBaseline(record["input_tokens"], record["num_turns"])
    return found
