"""Verb `run-core`: traceability, arm T and P, F0 to F3, validity judges, J-SONNET masked and probe."""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from dataclasses import dataclass

from pilot.application.alignment_stage import aggregate_and_prove, run_alignment, run_probe
from pilot.application.arm_p import ArmPItems, planted_mutants
from pilot.application.arm_t import ensure_plan, generate_mutants
from pilot.application.classes_stage import classify_mutants
from pilot.application.context import PipelineContext
from pilot.application.mutant_records import evaluated_mutant_ids, record_processing, write_mutants
from pilot.application.process_mutants import MutantProcessor
from pilot.application.proofs_stage import write_discriminating_power_proofs
from pilot.application.provenance import ManifestInputs, ProvenanceProbe, ensure_manifest
from pilot.application.ports import CliVersionProbe, VariantRunner, WitnessExecutor
from pilot.application.reference_stage import ensure_reference
from pilot.application.stage_timer import StageTimer
from pilot.application.traceability_stage import RawTraceability, ensure_traceability
from pilot.application.validity_stage import judge_validity
from pilot.domain.alignment_input import Arm
from pilot.domain.constants import PACKAGE_NAME
from pilot.domain.model_call import ModelPin
from pilot.domain.model_slate import VALIDITY_JUDGES, Judge
from pilot.domain.mutants import Candidate, MutantStatus
from pilot.domain.pipeline_constants import PROCESSING_FILENAME, VALIDITY_FILENAME


@dataclass(frozen=True)
class CoreDependencies:
    ctx: PipelineContext
    timer: StageTimer
    provenance: ProvenanceProbe
    versions: CliVersionProbe
    manifest_inputs: ManifestInputs
    traceability: RawTraceability
    runner: VariantRunner
    processor: MutantProcessor
    witnesses: WitnessExecutor
    arm_p: ArmPItems
    pins: Mapping[str, ModelPin]
    expected_tests: int
    expected_criteria: int


def run_core(deps: CoreDependencies) -> None:
    ctx = deps.ctx
    ensure_manifest(ctx.store, deps.provenance, deps.versions, deps.manifest_inputs)
    with deps.timer.stage("traceability"):
        bindings = ensure_traceability(ctx.store, deps.traceability, deps.expected_tests, deps.expected_criteria)
        ctx.log(f"[traceability] {len(bindings)} tests bound")
    with deps.timer.stage("reference"):
        nodeids = ensure_reference(ctx.store, deps.runner)
        ctx.log(f"[reference] {len(nodeids)} tests passed on every run")
    with deps.timer.stage("generate"):
        candidates = _candidates(deps)
    with deps.timer.stage("process"):
        _process(deps, candidates, nodeids)
    with deps.timer.stage("validity"):
        _validity(deps)
        classify_mutants(ctx)
    with deps.timer.stage("alignment"):
        run_alignment(ctx, [(Judge.J_SONNET, Arm.NAME_MASKED)], sorted(bindings), PACKAGE_NAME)
        aggregate_and_prove(ctx, bindings, ctx.criteria())
    with deps.timer.stage("probe"):
        run_probe(ctx, [Judge.J_SONNET], sorted(bindings))
    with deps.timer.stage("proofs"):
        write_discriminating_power_proofs(ctx, bindings)


def _candidates(deps: CoreDependencies) -> list[Candidate]:
    ctx = deps.ctx
    plan = ensure_plan(ctx, ctx.criteria())
    generated = generate_mutants(ctx, plan, deps.pins)
    planted = planted_mutants(deps.arm_p)
    candidates = write_mutants(ctx.store, [*generated, *planted], ctx.log)
    ctx.log(f"[mutants] {len(candidates)} candidates ({len(generated)} arm T, {len(planted)} arm P)")
    return candidates


def _process(deps: CoreDependencies, candidates: Sequence[Candidate], nodeids: Sequence[str]) -> None:
    ctx = deps.ctx
    done = {row["mutant_id"] for row in ctx.store.read_jsonl(PROCESSING_FILENAME)} if ctx.store.exists(PROCESSING_FILENAME) else set()
    pending = [c for c in candidates if c.mutant_id not in done]
    if not pending:
        ctx.log("[process] already complete")
        return
    ctx.log(f"[process] {len(pending)} candidates through F0 to F3")
    records = deps.processor.process(pending, nodeids)
    record_processing(ctx.store, records)
    stillborn = sum(1 for r in records if r.status is MutantStatus.STILLBORN)
    ctx.log(f"[process] {len(records)} processed, {stillborn} stillborn")


def _validity(deps: CoreDependencies) -> None:
    ctx = deps.ctx
    ids = evaluated_mutant_ids(ctx.store, ("T", "P"))
    if _validity_complete(ctx, ids):
        ctx.log("[validity] already complete")
        return
    judge_validity(ctx, deps.witnesses, VALIDITY_JUDGES, ids)


def _validity_complete(ctx: PipelineContext, ids: Sequence[str]) -> bool:
    if not ctx.store.exists(VALIDITY_FILENAME):
        return False
    have = {(row["mutant_id"], row["judge"]) for row in ctx.store.read_jsonl(VALIDITY_FILENAME)}
    return all((mid, judge.value) in have for mid in ids for judge in VALIDITY_JUDGES)

