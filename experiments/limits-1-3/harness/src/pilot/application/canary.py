"""Use case `canary`: one synthetic call per (model, CLI) pair (PREREG 4.4)."""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from dataclasses import dataclass

from pilot.application.ports import CanaryStore, CliVersionProbe, ModelPort, SchemaValidator
from pilot.domain.model_call import (
    CanaryBaseline,
    Cli,
    Effort,
    MissingResult,
    ModelPin,
    ModelRequest,
)

CANARY_CALL_PREFIX = "canary"


class CanaryFailed(Exception):
    """Any canary failure aborts the run."""


@dataclass(frozen=True)
class CanarySpec:
    pins: Sequence[ModelPin]
    locked_cli_versions: Mapping[Cli, str]
    effort: Effort
    system_prompt: str
    item: str
    schema_text: str


def run_canary(
    model: ModelPort,
    probe: CliVersionProbe,
    validator: SchemaValidator,
    store: CanaryStore,
    spec: CanarySpec,
) -> dict[str, CanaryBaseline]:
    baselines = {}
    for pin in spec.pins:
        version = _checked_version(probe, spec, pin)
        baseline = _measure(model, validator, spec, pin)
        store.save(pin, baseline, version)
        baselines[pin.key] = baseline
    return baselines


def _checked_version(probe: CliVersionProbe, spec: CanarySpec, pin: ModelPin) -> str:
    installed = probe.installed_version(pin.cli)
    locked = spec.locked_cli_versions[pin.cli]
    if installed != locked:
        raise CanaryFailed(f"{pin.cli.value} installed {installed}, locked {locked}")
    return installed


def _measure(
    model: ModelPort, validator: SchemaValidator, spec: CanarySpec, pin: ModelPin
) -> CanaryBaseline:
    request = ModelRequest(
        call_id=f"{CANARY_CALL_PREFIX}/{pin.key}",
        pin=pin,
        effort=spec.effort,
        item=spec.item,
        schema_text=spec.schema_text,
        system_prompt=spec.system_prompt,
    )
    outcome = model.call(request, None)
    if isinstance(outcome, MissingResult):
        raise CanaryFailed(f"{pin.key}: {outcome.failures} {outcome.isolation_violations}")
    problems = validator.violations(spec.schema_text, outcome.payload)
    if problems:
        raise CanaryFailed(f"{pin.key}: payload violates schema: {problems}")
    return CanaryBaseline(outcome.usage.input_tokens, outcome.num_turns)
