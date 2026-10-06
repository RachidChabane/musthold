"""Verb `run-secondary`: J-H NAME-SHOWN, J-X and the mutant audit, inside the secondary window (ADR 0006, run order 6)."""

from __future__ import annotations

from dataclasses import dataclass, replace

from pilot.application.alignment_stage import aggregate_and_prove, run_alignment
from pilot.application.context import PipelineContext
from pilot.application.core_run import CoreDependencies
from pilot.application.exploratory_calls import Clock, ExploratoryLedger, WindowedCaller, utc_now
from pilot.application.mutant_records import mutant_path
from pilot.application.provenance import ensure_manifest
from pilot.application.validity_stage import judge_validity
from pilot.domain.alignment_input import Arm
from pilot.domain.constants import PACKAGE_NAME
from pilot.domain.exploratory import SECONDARY_WINDOW, STREAM_SECONDARY, Window
from pilot.domain.model_slate import Judge
from pilot.domain.pipeline_constants import (
    AUDIT_SAMPLE_FILENAME,
    BINDINGS_FILENAME,
    PROCESSING_FILENAME,
    REFERENCE_FILENAME,
    VALIDITY_FILENAME,
)

SECONDARY_ALIGNMENT = (
    (Judge.J_H, Arm.NAME_SHOWN),
    (Judge.J_X, Arm.NAME_MASKED),
)


class SecondaryPreconditionFailed(Exception):
    pass


@dataclass(frozen=True)
class SecondaryDependencies:
    core: CoreDependencies
    ledger: ExploratoryLedger
    window: Window = SECONDARY_WINDOW
    clock: Clock = utc_now


def run_secondary(deps: SecondaryDependencies) -> None:
    core = deps.core
    audit_ids = _preflight(core.ctx)
    ensure_manifest(core.ctx.store, core.provenance, core.versions, core.manifest_inputs)
    ctx = replace(core.ctx, caller=WindowedCaller(core.ctx.caller, STREAM_SECONDARY, deps.ledger, deps.window, deps.clock))
    bindings = ctx.store.read_json(BINDINGS_FILENAME)
    with core.timer.stage("secondary-alignment"):
        run_alignment(ctx, SECONDARY_ALIGNMENT, sorted(bindings), PACKAGE_NAME)
        aggregate_and_prove(ctx, bindings, ctx.criteria())
    with core.timer.stage("audit"):
        judge_validity(ctx, core.witnesses, [Judge.A_OPUS], audit_ids)


def _preflight(ctx: PipelineContext) -> list[str]:
    for required in (BINDINGS_FILENAME, REFERENCE_FILENAME, PROCESSING_FILENAME, VALIDITY_FILENAME):
        if not ctx.store.exists(required):
            raise SecondaryPreconditionFailed(f"{required} is missing: run run-core on this run id first")
    if not ctx.store.exists(AUDIT_SAMPLE_FILENAME):
        raise SecondaryPreconditionFailed(
            f"{AUDIT_SAMPLE_FILENAME} is missing: run audit-sample on this run id before run-secondary"
        )
    ids = ctx.store.read_json(AUDIT_SAMPLE_FILENAME)["mutant_ids"]
    unknown = [mid for mid in ids if not ctx.store.exists(mutant_path(mid))]
    if unknown:
        raise SecondaryPreconditionFailed(f"{AUDIT_SAMPLE_FILENAME} names unknown mutants: {unknown[:5]}")
    return sorted(ids)
