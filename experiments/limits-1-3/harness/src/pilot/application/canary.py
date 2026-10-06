"""Use case `canary`: one synthetic call per (model, call type) actually used (PREREG 4.4)."""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from dataclasses import dataclass

from pilot.application.ports import CanaryStore, CliVersionProbe, ModelPort, PromptLibrary, SchemaValidator
from pilot.domain.alignment_input import alignment_schema
from pilot.domain.call_type import CALL_EFFORT, CallType, call_types_of_roles
from pilot.domain.canary_items import CANARY_TOKENS, canary_item
from pilot.domain.model_call import (
    CanaryBaseline,
    Cli,
    MissingResult,
    ModelPin,
    ModelRequest,
)

CANARY_CALL_PREFIX = "canary"
CALL_ID_TEMPLATE = "{prefix}/{pin}__{call_type}"

BaselineKey = tuple[str, CallType]


class CanaryFailed(Exception):
    """Any canary failure aborts the run."""


@dataclass(frozen=True)
class CanaryTask:
    pin: ModelPin
    call_type: CallType
    item: str
    schema_text: str


@dataclass(frozen=True)
class CanaryReport:
    baselines: Mapping[BaselineKey, CanaryBaseline]
    versions: Mapping[Cli, str]


def plan_canary(
    roles: Mapping[ModelPin, tuple[str, ...]],
    prompts: PromptLibrary,
    models: Sequence[str] = (),
    call_types: Sequence[CallType] = (),
) -> list[CanaryTask]:
    """One task per (pin, call type) of the lock roles, optionally narrowed by model and call type."""
    templates = {call_type: prompts.template(call_type) for call_type in CallType}
    tasks = []
    for pin in sorted(roles, key=lambda p: p.key):
        for call_type in call_types_of_roles(roles[pin]):
            if (models and pin.model not in models) or (call_types and call_type not in call_types):
                continue
            tasks.append(
                CanaryTask(pin, call_type, canary_item(call_type, templates), _schema(prompts, call_type))
            )
    return tasks


def _schema(prompts: PromptLibrary, call_type: CallType) -> str:
    schema = prompts.schema(call_type)
    if call_type is CallType.ALIGNMENT:
        return alignment_schema(schema, CANARY_TOKENS)
    return schema


def run_canary(
    model: ModelPort,
    probe: CliVersionProbe,
    validator: SchemaValidator,
    store: CanaryStore,
    tasks: Sequence[CanaryTask],
    system_prompt: str,
) -> CanaryReport:
    versions = {cli: probe.installed_version(cli) for cli in sorted({t.pin.cli for t in tasks}, key=lambda c: c.value)}
    baselines: dict[BaselineKey, CanaryBaseline] = {}
    for task in tasks:
        baseline = _measure(model, validator, task, system_prompt)
        store.save(task.pin, task.call_type, baseline, versions[task.pin.cli])
        baselines[(task.pin.key, task.call_type)] = baseline
    return CanaryReport(baselines, versions)


def _measure(
    model: ModelPort, validator: SchemaValidator, task: CanaryTask, system_prompt: str
) -> CanaryBaseline:
    label = f"{task.pin.key}/{task.call_type.value}"
    request = ModelRequest(
        call_id=CALL_ID_TEMPLATE.format(
            prefix=CANARY_CALL_PREFIX, pin=task.pin.key, call_type=task.call_type.value
        ),
        pin=task.pin,
        effort=CALL_EFFORT[task.call_type],
        item=task.item,
        schema_text=task.schema_text,
        system_prompt=system_prompt,
    )
    outcome = model.call(request, None)
    if isinstance(outcome, MissingResult):
        raise CanaryFailed(f"{label}: {outcome.failures} {outcome.isolation_violations}")
    problems = validator.violations(task.schema_text, outcome.payload)
    if problems:
        raise CanaryFailed(f"{label}: payload violates schema: {problems}")
    return CanaryBaseline(outcome.usage.input_tokens, outcome.num_turns)
