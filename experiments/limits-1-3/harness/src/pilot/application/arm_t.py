"""Arm T: criterion-targeted generation, five calls per criterion (PREREG 6.1)."""

from __future__ import annotations

import uuid
from collections.abc import Callable, Mapping, Sequence
from dataclasses import dataclass
from typing import Any

from pilot.application.context import PipelineContext
from pilot.application.model_caller import CallSpec
from pilot.application.mutant_records import NewMutant
from pilot.domain.call_type import CallType
from pilot.domain.edits import EditSet
from pilot.domain.model_call import MissingResult, ModelPin, ModelResult
from pilot.domain.model_slate import GENERATORS
from pilot.domain.pipeline_constants import GENERATION_SLOTS, PLAN_FILENAME
from pilot.domain.prompts import generation_item
from pilot.domain.spec import Criterion

ARM_T = "T"
CALL_ID_TEMPLATE = "generate__{ref}__{index}"
NonceFactory = Callable[[], str]


def _new_nonce() -> str:
    return uuid.uuid4().hex


@dataclass(frozen=True)
class PlannedCall:
    call_id: str
    criterion: str
    model: str
    cli: str
    nonce: str

    def to_mapping(self) -> dict[str, str]:
        return {
            "call_id": self.call_id,
            "criterion": self.criterion,
            "model": self.model,
            "cli": self.cli,
            "nonce": self.nonce,
        }


def ensure_plan(
    ctx: PipelineContext,
    criteria: Sequence[Criterion],
    generators: Sequence[ModelPin] = GENERATORS,
    nonce_factory: NonceFactory = _new_nonce,
) -> list[PlannedCall]:
    """Persists call ids and nonces before any call, so a resume reuses the same nonces."""
    if ctx.store.exists(PLAN_FILENAME):
        recorded = ctx.store.read_json(PLAN_FILENAME)
        return [PlannedCall(**entry) for entry in recorded["arm_t"]]
    plan = [
        PlannedCall(
            call_id=CALL_ID_TEMPLATE.format(ref=criterion.ref, index=index),
            criterion=criterion.ref,
            model=pin.model,
            cli=pin.cli.value,
            nonce=nonce_factory(),
        )
        for criterion in criteria
        for index, pin in enumerate(generators, start=1)
    ]
    ctx.store.write_json(PLAN_FILENAME, {"arm_t": [call.to_mapping() for call in plan]})
    return plan


def generation_problems(payload: Any) -> list[str]:
    """Constraints the schema cannot express: the three slots are S1, S2 and S3, each once."""
    slots = sorted(mutant["slot"] for mutant in payload["mutants"])
    if slots != sorted(GENERATION_SLOTS):
        return [f"slots {slots}, expected {list(GENERATION_SLOTS)}"]
    return []


def generate_mutants(
    ctx: PipelineContext, plan: Sequence[PlannedCall], pins: Mapping[str, ModelPin]
) -> list[NewMutant]:
    criteria = {c.ref: c for c in ctx.criteria()}
    spec = ctx.sources.spec_text()
    source = ctx.sources.source_files()
    template = ctx.prompts.template(CallType.GENERATE)
    schema = ctx.prompts.schema(CallType.GENERATE)
    specs = [
        CallSpec(
            call_id=call.call_id,
            pin=pins[call.model],
            call_type=CallType.GENERATE,
            item=generation_item(
                template, spec, call.criterion, criteria[call.criterion].statement, source, call.nonce
            ),
            schema_text=schema,
            validate=generation_problems,
        )
        for call in plan
    ]
    outcomes = ctx.caller.call_all("generate", specs)
    mutants: list[NewMutant] = []
    missing = 0
    for call in plan:
        outcome = outcomes[call.call_id]
        if isinstance(outcome, MissingResult):
            missing += 1
            continue
        mutants.extend(_mutants_of(call, outcome))
    ctx.log(f"[generate] {len(mutants)} candidates from {len(plan) - missing}/{len(plan)} calls ({missing} missing)")
    return mutants


def _mutants_of(call: PlannedCall, outcome: ModelResult) -> list[NewMutant]:
    generator = {"model": call.model, "cli": call.cli, "call_id": call.call_id, "nonce": call.nonce}
    return [
        NewMutant(
            arm=ARM_T,
            edit_set=EditSet.from_mapping({"edits": entry["edits"]}),
            target=call.criterion,
            slot=entry["slot"],
            generator=generator,
            mechanism_summary=entry["mechanism_summary"],
        )
        for entry in sorted(outcome.payload["mutants"], key=lambda m: m["slot"])
    ]
