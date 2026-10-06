"""Verb `run-blindspot`: contrast mutants, test writers S and O, operator mutants, F0 to F2 and writer kill vectors (ADR 0006)."""

from __future__ import annotations

from collections.abc import Sequence
from dataclasses import dataclass

from pilot.application.arm_o import operator_candidates
from pilot.application.arm_t import ensure_plan, generate_mutants
from pilot.application.context import PipelineContext
from pilot.application.core_run import CoreDependencies
from pilot.application.mutant_records import (
    NewMutant,
    mutant_path,
    record_processing,
    write_mutants,
)
from pilot.application.ports import SuiteRunners
from pilot.application.provenance import ensure_manifest
from pilot.application.test_writing_stage import filter_against_reference, write_suites
from pilot.application.writer_kill_stage import ensure_writer_kills, record_sources
from pilot.domain.arms import ARM_O, ARM_R, ARM_T, ARM_X
from pilot.domain.model_slate import PRIMARY_WRITERS, MutantSource
from pilot.domain.mutants import Candidate, MutantStatus
from pilot.domain.pipeline_constants import (
    BINDINGS_FILENAME,
    PROCESSING_FILENAME,
    REFERENCE_FILENAME,
)


@dataclass(frozen=True)
class BlindSpotDependencies:
    core: CoreDependencies
    suites: SuiteRunners


class BlindSpotPreconditionFailed(Exception):
    pass


def run_blindspot(deps: BlindSpotDependencies) -> None:
    core = deps.core
    ctx = core.ctx
    _preflight(ctx)
    ensure_manifest(ctx.store, core.provenance, core.versions, core.manifest_inputs)
    plan = ensure_plan(ctx, ctx.criteria())
    with core.timer.stage("blindspot-generate"):
        contrast = generate_mutants(ctx, plan.contrast_x, core.pins, ARM_X)
        terra = generate_mutants(ctx, plan.terra_x, core.pins, ARM_R)
    with core.timer.stage("blindspot-writers"):
        write_suites(ctx, plan.writers, core.pins)
        for writer in PRIMARY_WRITERS:
            filter_against_reference(ctx, deps.suites, writer)
    with core.timer.stage("blindspot-arm-o"):
        operators = operator_candidates(ctx.sources.source_files())
        ctx.log(f"[arm-o] {len(operators)} operator mutants enumerated")
    with core.timer.stage("blindspot-process"):
        process_source_mutants(core, [(MutantSource.OPUS, contrast), (MutantSource.TERRA, terra), (MutantSource.OPERATOR, operators)])
        record_sources(ctx.store, MutantSource.SONNET, arm_members(ctx, ARM_T))
    with core.timer.stage("blindspot-kills"):
        ensure_writer_kills(ctx, deps.suites, PRIMARY_WRITERS)


def _preflight(ctx: PipelineContext) -> None:
    for required in (BINDINGS_FILENAME, REFERENCE_FILENAME, PROCESSING_FILENAME):
        if not ctx.store.exists(required):
            raise BlindSpotPreconditionFailed(f"{required} is missing: run run-core on this run id first")


def arm_members(ctx: PipelineContext, arm: str) -> list[str]:
    """Ids of the mutant documents written under one arm."""
    names = ctx.store.list_files("mutants")
    ids = [name.removesuffix(".json") for name in names]
    return [mutant_id for mutant_id in ids if ctx.store.read_json(mutant_path(mutant_id))["arm"] == arm]


def process_source_mutants(core: CoreDependencies, sources: Sequence[tuple[MutantSource, Sequence[NewMutant]]]) -> None:
    """Writes the mutants of each source, records the membership and runs F0 to F2 on those not yet processed."""
    ctx = core.ctx
    candidates: list[Candidate] = []
    for source, mutants in sources:
        candidates += write_mutants(ctx.store, mutants, ctx.log)
        record_sources(ctx.store, source, [mutant.mutant_id for mutant in mutants])
    done = {row["mutant_id"] for row in ctx.store.read_jsonl(PROCESSING_FILENAME)}
    pending = [candidate for candidate in candidates if candidate.mutant_id not in done]
    if not pending:
        ctx.log("[process-sources] already complete")
        return
    nodeids = sorted(ctx.store.read_json(BINDINGS_FILENAME))
    ctx.log(f"[process-sources] {len(pending)} mutants through F0 to F2")
    records = core.processor.process(pending, nodeids, differential=False)
    record_processing(ctx.store, records)
    stillborn = sum(1 for record in records if record.status is MutantStatus.STILLBORN)
    ctx.log(f"[process-sources] {len(records)} processed, {stillborn} stillborn")
