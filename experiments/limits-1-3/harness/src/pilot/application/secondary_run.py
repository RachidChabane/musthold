"""Verb `run-secondary`: the remaining judges, arm O and the mutant audit (PREREG 14, step 5)."""

from __future__ import annotations

from collections.abc import Sequence

from pilot.application.alignment_stage import aggregate_and_prove, run_alignment, run_probe
from pilot.application.arm_o import operator_candidates
from pilot.application.context import PipelineContext
from pilot.application.core_run import CoreDependencies
from pilot.application.mutant_records import mutant_path, record_processing, write_mutants
from pilot.application.provenance import ensure_manifest
from pilot.application.validity_stage import judge_validity
from pilot.domain.alignment_input import Arm
from pilot.domain.constants import PACKAGE_NAME
from pilot.domain.model_slate import Judge
from pilot.domain.mutants import Candidate, MutantStatus
from pilot.domain.pipeline_constants import (
    AUDIT_SAMPLE_FILENAME,
    BINDINGS_FILENAME,
    PROCESSING_FILENAME,
    REFERENCE_FILENAME,
    VALIDITY_FILENAME,
)

SECONDARY_ALIGNMENT = (
    (Judge.J_SONNET, Arm.NAME_SHOWN),
    (Judge.J_GPT, Arm.NAME_MASKED),
    (Judge.J_GPT, Arm.NAME_SHOWN),
)


class SecondaryPreconditionFailed(Exception):
    pass


def run_secondary(core: CoreDependencies) -> None:
    ctx = core.ctx
    audit_ids = _preflight(ctx)
    ensure_manifest(ctx.store, core.provenance, core.versions, core.manifest_inputs)
    bindings = ctx.store.read_json(BINDINGS_FILENAME)
    nodeids = sorted(bindings)
    with core.timer.stage("secondary-alignment"):
        run_alignment(ctx, SECONDARY_ALIGNMENT, nodeids, PACKAGE_NAME)
        aggregate_and_prove(ctx, bindings, ctx.criteria())
    with core.timer.stage("secondary-probe"):
        run_probe(ctx, [Judge.J_GPT], nodeids)
    with core.timer.stage("arm-o"):
        _process_arm_o(core)
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


def _process_arm_o(core: CoreDependencies) -> None:
    ctx = core.ctx
    candidates = write_mutants(ctx.store, operator_candidates(ctx.sources.source_files()), ctx.log)
    nodeids = sorted(ctx.store.read_json(BINDINGS_FILENAME))
    done = {row["mutant_id"] for row in ctx.store.read_jsonl(PROCESSING_FILENAME)}
    pending: Sequence[Candidate] = [c for c in candidates if c.mutant_id not in done]
    if not pending:
        ctx.log("[arm-o] already complete")
        return
    ctx.log(f"[arm-o] {len(pending)} operator mutants through F0 to F3")
    records = core.processor.process(pending, nodeids)
    record_processing(ctx.store, records)
    stillborn = sum(1 for r in records if r.status is MutantStatus.STILLBORN)
    ctx.log(f"[arm-o] {len(records)} processed, {stillborn} stillborn")

