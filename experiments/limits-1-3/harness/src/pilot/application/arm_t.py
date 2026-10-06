"""Criterion-targeted generation and the run plan: arm T (host), the contrast sources X and R, luna (PREREG 6.1, 6.8)."""

from __future__ import annotations

import uuid
from collections.abc import Mapping, Sequence
from typing import Any

from pilot.application.context import PipelineContext
from pilot.application.model_caller import CallSpec
from pilot.application.mutant_records import NewMutant
from pilot.domain.arms import ARM_T
from pilot.domain.call_type import CallType
from pilot.domain.edits import EditSet
from pilot.domain.import_manifest import IMPORTS_FILENAME
from pilot.domain.model_call import MissingResult, ModelPin, ModelResult
from pilot.domain.pipeline_constants import GENERATION_SLOTS, PLAN_FILENAME
from pilot.domain.prompts import generation_item
from pilot.domain.run_plan import NonceFactory, PlannedCall, RunPlan, build_plan
from pilot.domain.spec import Criterion


def _new_nonce() -> str:
    return uuid.uuid4().hex


def ensure_plan(
    ctx: PipelineContext,
    criteria: Sequence[Criterion],
    nonce_factory: NonceFactory = _new_nonce,
) -> RunPlan:
    """Persists call ids and nonces before any call, so a resume reuses the same nonces."""
    if ctx.store.exists(PLAN_FILENAME):
        return RunPlan.from_mapping(ctx.store.read_json(PLAN_FILENAME))
    imported = ctx.store.read_json(IMPORTS_FILENAME)["nonces"] if ctx.store.exists(IMPORTS_FILENAME) else {}
    plan = build_plan([criterion.ref for criterion in criteria], imported, nonce_factory)
    ctx.store.write_json(PLAN_FILENAME, plan.to_mapping())
    return plan


def generation_problems(payload: Any) -> list[str]:
    """Constraints the schema cannot express: the three slots are S1, S2 and S3, each once."""
    slots = sorted(mutant["slot"] for mutant in payload["mutants"])
    if slots != sorted(GENERATION_SLOTS):
        return [f"slots {slots}, expected {list(GENERATION_SLOTS)}"]
    return []


def generate_mutants(
    ctx: PipelineContext, calls: Sequence[PlannedCall], pins: Mapping[str, ModelPin], arm: str = ARM_T
) -> list[NewMutant]:
    """One generation call per planned call (resumed when recorded); mutants of the answered calls, in slot order."""
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
        for call in calls
    ]
    outcomes = ctx.caller.call_all(f"generate:{arm}", specs)
    mutants: list[NewMutant] = []
    missing = 0
    for call in calls:
        outcome = outcomes[call.call_id]
        if isinstance(outcome, MissingResult):
            missing += 1
            continue
        mutants.extend(_mutants_of(call, outcome, arm))
    ctx.log(f"[generate:{arm}] {len(mutants)} candidates from {len(calls) - missing}/{len(calls)} calls ({missing} missing)")
    return mutants


def _mutants_of(call: PlannedCall, outcome: ModelResult, arm: str) -> list[NewMutant]:
    generator = {"model": call.model, "cli": call.cli, "call_id": call.call_id, "nonce": call.nonce}
    return [
        NewMutant(
            arm=arm,
            edit_set=EditSet.from_mapping({"edits": entry["edits"]}),
            target=call.criterion,
            slot=entry["slot"],
            generator=generator,
            mechanism_summary=entry["mechanism_summary"],
        )
        for entry in sorted(outcome.payload["mutants"], key=lambda m: m["slot"])
    ]
