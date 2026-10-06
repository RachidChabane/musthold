"""Verb `run-luna`: the exploratory luna stratum inside its window (ADR 0006).

Canary, test writer T_L and mutant generator M_L (one call per criterion each), F0 to F2 for the
luna mutants, and every writer's kill vectors over every evaluated mutant. Oracle classification
of the new mutants is the analysis side's `oracle-classify`.
"""

from __future__ import annotations

from collections.abc import Callable
from dataclasses import dataclass, replace

from pilot.application.arm_t import ensure_plan, generate_mutants
from pilot.application.blind_spot_stage import BlindSpotDependencies, BlindSpotPreconditionFailed, process_source_mutants
from pilot.application.exploratory_calls import Clock, ExploratoryLedger, WindowedCaller, WindowNotOpen, utc_now
from pilot.application.provenance import ensure_manifest
from pilot.application.test_writing_stage import filter_against_reference, write_suites
from pilot.application.writer_kill_stage import ensure_writer_kills
from pilot.domain.arms import ARM_L
from pilot.domain.exploratory import LUNA_WINDOW, STREAM_LUNA, Window
from pilot.domain.model_slate import PRIMARY_WRITERS, MutantSource, Writer
from pilot.domain.pipeline_constants import BINDINGS_FILENAME, PROCESSING_FILENAME, REFERENCE_FILENAME
from pilot.domain.run_plan import KIND_GENERATE, KIND_WRITE_TESTS


@dataclass(frozen=True)
class LunaDependencies:
    blind_spot: BlindSpotDependencies
    ledger: ExploratoryLedger
    run_canary: Callable[[], None]
    window: Window = LUNA_WINDOW
    clock: Clock = utc_now


def run_luna(deps: LunaDependencies) -> None:
    core = deps.blind_spot.core
    base = core.ctx
    for required in (BINDINGS_FILENAME, REFERENCE_FILENAME, PROCESSING_FILENAME):
        if not base.store.exists(required):
            raise BlindSpotPreconditionFailed(f"{required} is missing: run run-core on this run id first")
    if deps.window.not_yet_open(deps.clock()):
        raise WindowNotOpen(f"{STREAM_LUNA} opens at {deps.window.opens}")
    ensure_manifest(base.store, core.provenance, core.versions, core.manifest_inputs)
    plan = ensure_plan(base, base.criteria())
    ctx = replace(base, caller=WindowedCaller(base.caller, STREAM_LUNA, deps.ledger, deps.window, deps.clock))
    if deps.window.is_open(deps.clock()):
        with core.timer.stage("luna-canary"):
            deps.run_canary()
    with core.timer.stage("luna-calls"):
        write_suites(ctx, [call for call in plan.luna if call.kind == KIND_WRITE_TESTS], core.pins)
        filter_against_reference(ctx, deps.blind_spot.suites, Writer.LUNA)
        mutants = generate_mutants(ctx, [call for call in plan.luna if call.kind == KIND_GENERATE], core.pins, ARM_L)
    with core.timer.stage("luna-process"):
        process_source_mutants(replace(core, ctx=ctx), [(MutantSource.LUNA, mutants)])
    with core.timer.stage("luna-kills"):
        ensure_writer_kills(ctx, deps.blind_spot.suites, [*PRIMARY_WRITERS, Writer.LUNA])
