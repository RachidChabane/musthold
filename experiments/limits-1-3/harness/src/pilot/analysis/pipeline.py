"""The analysis, end to end: a context in, every endpoint, table and verdict out."""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from dataclasses import dataclass

from pilot.analysis.attribution import (
    ErrorMultiplication,
    endpoint_attribution,
    error_multiplication,
    run_substitutions,
)
from pilot.analysis.composite import Composite, apply_preconditions, composite_verdict
from pilot.analysis.amendment_tables import calibration, isolation_report, same_model_correlation
from pilot.analysis.blind_spot import BlindSpotResult
from pilot.analysis.blind_spot_run import blind_spot, blind_spot_tables
from pilot.analysis.cost import cost_lines, cpu_seconds, wall_clock_seconds
from pilot.analysis.endpoints import (
    compute_matrix_nulls,
    cross_check_endpoint,
    gate_endpoints_from,
    judge_endpoint,
    judge_verdicts,
    matrix_endpoint,
    matrix_signatures,
    validity_endpoints,
)
from pilot.analysis.failure_modes import failure_table
from pilot.analysis.feasibility import FeasibilityItem, evaluate_feasibility
from pilot.analysis.model import Context, IncompleteInputs
from pilot.analysis.preconditions import Precondition, evaluate_preconditions
from pilot.analysis.results import EndpointResult
from pilot.analysis.scenarios import GateVariant, TruthKind, build_scenarios, simulate, traceability_passes
from pilot.analysis.sensitivity import SensitivityRow, sensitivity_rows
from pilot.analysis.settings import AnalysisSettings
from pilot.analysis.table import Table
from pilot.analysis.tables import (
    all_signatures,
    confusion_label_answer,
    confusion_label_signature,
    confusion_label_verdict,
    contrasts,
    cross_check,
    full_set_outcomes,
    inter_rater,
    limit2_tables,
    limit3_tables,
    oracle_x_pipeline,
    probe_table,
    single_test_gate,
    violation_signatures,
)
from pilot.domain import analysis_constants as k
from pilot.domain.alignment_aggregate import AlignmentVerdict
from pilot.domain.gate import G3
from pilot.domain.matrix import TestSignature

PRIMARY_VARIANT = GateVariant("G3", G3)
PRIMARY_TRUTH = TruthKind.ORACLE


@dataclass(frozen=True)
class AnalysisOutput:
    endpoints: Sequence[EndpointResult]
    composite: Composite
    preconditions: Sequence[Precondition]
    feasibility: Sequence[FeasibilityItem]
    feasibility_verdict: str
    tables: Sequence[Table]
    cost_table: Table
    timing_table: Table
    failure_table: Table
    sensitivity: Sequence[SensitivityRow]
    error_multiplication: ErrorMultiplication
    blind_spot: BlindSpotResult
    blind_spot_tables: Sequence[Table]
    scenario_count: int
    full_set_count: int
    settings: AnalysisSettings


def require_complete(ctx: Context) -> None:
    problems = []
    if len(ctx.declared) != len(ctx.nodeids):
        problems.append(f"bindings cover {len(ctx.declared)} of {len(ctx.nodeids)} labelled tests")
    missing = ctx.missing_oracle()
    if missing:
        problems.append(f"{len(missing)} non-stillborn mutant(s) lack an oracle record; run oracle-classify first")
    if not ctx.truth.d8_criteria:
        problems.append("prereg/isolation_table.json holds no C_D8; run isolation-table")
    if not ctx.run.reference_runs:
        problems.append("reference.json holds no runs")
    if problems:
        raise IncompleteInputs("; ".join(problems))


def run_analysis(
    ctx: Context,
    lock: Mapping[str, object],
    settings: AnalysisSettings,
) -> AnalysisOutput:
    require_complete(ctx)
    scenarios = build_scenarios(ctx)
    primary_outcomes = simulate(ctx, scenarios, PRIMARY_VARIANT)
    runs = run_substitutions(ctx, scenarios, PRIMARY_VARIANT, PRIMARY_TRUTH)
    signatures = matrix_signatures(ctx)
    nulls = compute_matrix_nulls(ctx, settings)
    masked = judge_verdicts(ctx, k.JUDGE_H, k.ALIGNMENT_ARM_MASKED, False)

    gate = [
        r.with_attribution(endpoint_attribution(r.id, runs))
        for r in gate_endpoints_from(ctx, primary_outcomes, PRIMARY_TRUTH, settings, PRIMARY_VARIANT.name)
    ]
    validity = validity_endpoints(ctx, settings)
    judge = judge_endpoint(ctx, masked, settings)
    matrix = matrix_endpoint(ctx, signatures, nulls, settings)
    cross = cross_check_endpoint(ctx, masked, signatures, settings)
    base = [*gate, *validity, judge, matrix, cross]

    preconditions = evaluate_preconditions(ctx, lock, settings)
    endpoints = apply_preconditions(base, preconditions)
    composite = composite_verdict(endpoints, preconditions)
    d8 = next(e for e in base if e.id == "D8")
    valid_criteria = int(d8.details["frozen_reading"]["criteria_with_two_valid_w"])  # type: ignore[index]
    crosscheck = ctx.truth.crosscheck_differences
    feasibility, feasibility_verdict = evaluate_feasibility(
        ctx, valid_criteria, None if crosscheck is None else not crosscheck
    )
    by_id = {e.id: e for e in base}
    spot = blind_spot(ctx, settings)
    return AnalysisOutput(
        endpoints=endpoints,
        composite=composite,
        preconditions=preconditions,
        feasibility=feasibility,
        feasibility_verdict=feasibility_verdict,
        tables=_descriptive_tables(ctx, signatures, masked, settings),
        cost_table=_cost_table(ctx),
        timing_table=_timing_table(ctx),
        failure_table=failure_table(ctx, signatures, runs),
        sensitivity=sensitivity_rows(ctx, scenarios, by_id, nulls, settings),
        error_multiplication=error_multiplication(ctx, PRIMARY_VARIANT, settings, traceability_passes(ctx)),
        blind_spot=spot,
        blind_spot_tables=blind_spot_tables(ctx, spot),
        scenario_count=len(scenarios),
        full_set_count=sum(1 for s in scenarios if s.is_full),
        settings=settings,
    )


def _descriptive_tables(
    ctx: Context,
    signatures: Mapping[str, TestSignature],
    verdicts: Mapping[str, AlignmentVerdict],
    settings: AnalysisSettings,
) -> list[Table]:
    return [
        confusion_label_signature(ctx, signatures, "confusion_label_x_signature_arm_T",
                                  "6-class label x matrix signature, arm T mutants"),
        confusion_label_signature(ctx, violation_signatures(ctx), "confusion_label_x_signature_violations",
                                  "6-class label x matrix signature, the 30 violations as the mutant pool"),
        confusion_label_verdict(ctx),
        confusion_label_answer(ctx),
        single_test_gate(ctx),
        full_set_outcomes(ctx),
        oracle_x_pipeline(ctx, k.ARM_T),
        oracle_x_pipeline(ctx, k.ARM_P),
        oracle_x_pipeline(ctx, k.ARM_O),
        cross_check(ctx, verdicts, signatures),
        *limit2_tables(ctx, settings),
        *limit3_tables(ctx, settings),
        probe_table(ctx),
        isolation_report(ctx),
        same_model_correlation(ctx),
        calibration(ctx),
        contrasts(ctx, all_signatures(ctx)),
        inter_rater(ctx, settings),
    ]


def _cost_table(ctx: Context) -> Table:
    rows = [
        [line.group, f"PARK-{line.criterion}" if line.criterion else "", line.calls, line.claude_usd,
         line.claude_tokens, line.codex_tokens]
        for line in cost_lines(ctx)
    ]
    return Table("cost", "model cost: production path per criterion, then experiment overhead",
                 ["group", "criterion", "calls", "Claude USD", "Claude tokens", "codex tokens"], rows)


def _timing_table(ctx: Context) -> Table:
    rows = [
        [stage, record.get("seconds"), record.get("cpu_seconds")]
        for stage, record in sorted(ctx.run.timing.items())
    ]
    rows.append(["total wall clock", wall_clock_seconds(ctx.run.timing), cpu_seconds(ctx.run.timing)])
    return Table("timing", "wall clock and pytest CPU seconds per stage", ["stage", "seconds", "cpu seconds"], rows)

