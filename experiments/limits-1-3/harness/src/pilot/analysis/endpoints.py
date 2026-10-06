"""Endpoints D1 to D11 (PREREG 10), each computed from a context and a choice of evidence.

Every function returns `EndpointResult` objects with the band outcome before any
precondition cap; caps and attribution are applied by the composite stage.
"""

from __future__ import annotations

import math
from collections import defaultdict
from collections.abc import Collection, Mapping, Sequence
from dataclasses import dataclass

import numpy as np

from pilot.analysis import bands
from pilot.analysis.bands import Estimate, Outcome
from pilot.analysis.model import Context, matrix_pools
from pilot.analysis.nulls import (
    balanced_scores,
    matrix_predicted_bad,
    n2_always_misaligned_precision,
    n3_null_mean_dc,
    n6_null_balanced_accuracy,
    n7_base_rate,
)
from pilot.analysis.results import (
    EndpointResult,
    OutcomeAt,
    rescaled_wilson,
    scaled_interval,
)
from pilot.analysis.scenarios import (
    GateVariant,
    Scenario,
    ScenarioOutcome,
    TruthKind,
    alignment_verdicts,
    is_adequate,
    simulate,
)
from pilot.analysis.settings import AnalysisSettings
from pilot.analysis.stats import (
    FloatArray,
    Interval,
    balanced_accuracy_statistic,
    bootstrap_interval,
    cohen_kappa_statistic,
    cohen_kappa_tables,
    null_mean,
    null_threshold,
    ratio_statistic,
    wilson,
)
from pilot.domain import analysis_constants as k
from pilot.domain.alignment_aggregate import AlignmentVerdict
from pilot.domain.gate import GateOutcome
from pilot.domain.matrix import TestSignature, build_evidence, classify_tests, mean_dc
from pilot.domain.refs import criterion_ref

NAN = float("nan")
NUMERATOR, DENOMINATOR = 0, 1


@dataclass(frozen=True)
class RateCounts:
    """Numerator and denominator per cluster (criterion)."""

    per_cluster: Mapping[int, tuple[int, int]]

    @property
    def numerator(self) -> int:
        return sum(n for n, _ in self.per_cluster.values())

    @property
    def denominator(self) -> int:
        return sum(d for _, d in self.per_cluster.values())

    @property
    def point(self) -> float:
        return self.numerator / self.denominator if self.denominator else NAN

    def array(self) -> FloatArray:
        return np.array([list(v) for _, v in sorted(self.per_cluster.items())], dtype=np.float64).reshape(-1, 2)


def rate_estimate(counts: RateCounts, settings: AnalysisSettings) -> Estimate:
    """Point estimate with the wider of the Wilson and cluster-bootstrap intervals."""
    wilson_interval = wilson(counts.numerator, counts.denominator)
    boot = bootstrap_interval(counts.array(), ratio_statistic(NUMERATOR, DENOMINATOR), settings.resamples, settings.seed)
    return Estimate(counts.point, _wider(wilson_interval, boot))


def _wider(first: Interval, second: Interval) -> Interval:
    defined = [i for i in (first, second) if i.is_defined]
    return max(defined, key=lambda i: i.width) if defined else Interval(NAN, NAN)


def _result(
    endpoint_id: str,
    limit: int,
    measure: str,
    n: int,
    estimate: Estimate,
    band_go: str,
    band_nogo: str,
    outcome: Outcome,
    margin: float,
    details: Mapping[str, object],
    outcome_at: OutcomeAt | None,
    components: Sequence[Mapping[str, object]] = (),
) -> EndpointResult:
    return EndpointResult(
        id=endpoint_id,
        limit=limit,
        measure=measure,
        n=n,
        point=estimate.point,
        lo=estimate.interval.lo,
        hi=estimate.interval.hi,
        band_go=band_go,
        band_nogo=band_nogo,
        outcome=outcome,
        uncapped_outcome=outcome,
        components=tuple(components),
        details=dict(details),
        margin=margin,
        outcome_at=outcome_at,
    )


# ---------------------------------------------------------------- gate endpoints D1 to D5


def gate_endpoints(
    ctx: Context,
    scenarios: Sequence[Scenario],
    variant: GateVariant,
    truth: TruthKind,
    settings: AnalysisSettings,
) -> list[EndpointResult]:
    outcomes = simulate(ctx, list(scenarios), variant)
    return gate_endpoints_from(ctx, outcomes, truth, settings, variant.name)


def gate_endpoints_from(
    ctx: Context,
    outcomes: Sequence[ScenarioOutcome],
    truth: TruthKind,
    settings: AnalysisSettings,
    policy_name: str,
) -> list[EndpointResult]:
    adequate = {o.scenario.key: is_adequate(ctx, o.scenario, truth) for o in outcomes}
    measure_suffix = f"policy {policy_name}, {truth.value} truth"
    return [
        _d1(outcomes, measure_suffix),
        _d2(outcomes, adequate, measure_suffix),
        _d3(outcomes, adequate, settings, measure_suffix),
        _d4(outcomes, adequate, settings, measure_suffix),
        _d5(outcomes, settings, measure_suffix),
    ]


def _d1(outcomes: Sequence[ScenarioOutcome], suffix: str) -> EndpointResult:
    full = [o for o in outcomes if o.scenario.is_full and o.scenario.criterion in k.UNCOVERED_CRITERIA]
    passes = sum(1 for o in full if o.outcome is GateOutcome.PASS)
    point = passes / len(full) if full else NAN
    estimate = Estimate(point, wilson(passes, len(full)))
    return _result(
        "D1", 1, f"full-set PASS on an uncovered criterion {{6, 7, 10}} ({suffix})", len(full), estimate,
        "0/3", ">= 1/3", bands.full_set_false_pass(passes, len(full)), -point if full else NAN,
        {"passes": passes, "criteria": [c for c in k.UNCOVERED_CRITERIA]},
        lambda n: bands.full_set_false_pass(round(point * n), n),
    )


def _d2(outcomes: Sequence[ScenarioOutcome], adequate: Mapping[str, bool], suffix: str) -> EndpointResult:
    full = [o for o in outcomes if o.scenario.is_full and adequate[o.scenario.key]]
    blocks = sum(1 for o in full if o.outcome is GateOutcome.BLOCK)
    point = blocks / len(full) if full else NAN
    estimate = Estimate(point, wilson(blocks, len(full)))
    return _result(
        "D2", 1, f"full-set false block over ADEQUATE full sets ({suffix})", len(full), estimate,
        "<= floor(0.15 n_A)", ">= ceil(0.5 n_A)", bands.full_set_false_block(blocks, len(full)),
        k.D2_GO_FRACTION_PERCENT / 100 - point if full else NAN,
        {"false_blocks": blocks, "n_adequate": len(full)},
        lambda n: bands.full_set_false_block(round(point * n), n),
    )


def _scenario_rate(
    outcomes: Sequence[ScenarioOutcome],
    selected: Collection[str],
    predicate: GateOutcome,
    settings: AnalysisSettings,
) -> tuple[RateCounts, Estimate]:
    per_cluster: dict[int, list[int]] = defaultdict(lambda: [0, 0])
    for o in outcomes:
        cell = per_cluster[o.scenario.criterion]
        if o.scenario.key in selected:
            cell[DENOMINATOR] += 1
            cell[NUMERATOR] += int(o.outcome is predicate)
    counts = RateCounts({c: (v[0], v[1]) for c, v in per_cluster.items()})
    return counts, rate_estimate(counts, settings)


def _rate_endpoint(
    endpoint_id: str,
    measure: str,
    counts: RateCounts,
    estimate: Estimate,
    go_point: float,
    go_upper: float,
    no_go_lower: float,
    go_text: str,
    nogo_text: str,
) -> EndpointResult:
    point = estimate.point

    def outcome_at(n: int) -> Outcome:
        return bands.lower_is_better_rate(Estimate(point, rescaled_wilson(point, n)), go_point, go_upper, no_go_lower)

    return _result(
        endpoint_id, 1, measure, counts.denominator, estimate, go_text, nogo_text,
        bands.lower_is_better_rate(estimate, go_point, go_upper, no_go_lower), go_upper - estimate.interval.hi,
        {"events": counts.numerator, "units": counts.denominator}, outcome_at,
    )


def _d3(
    outcomes: Sequence[ScenarioOutcome], adequate: Mapping[str, bool], settings: AnalysisSettings, suffix: str
) -> EndpointResult:
    inadequate = {key for key, ok in adequate.items() if not ok}
    counts, estimate = _scenario_rate(outcomes, inadequate, GateOutcome.PASS, settings)
    return _rate_endpoint(
        "D3", f"scenario false-pass rate over INADEQUATE scenarios ({suffix})", counts, estimate,
        k.D3_GO_POINT, k.D3_GO_UPPER, k.D3_NO_GO_LOWER, "point <= 0.05 and upper <= 0.20", "lower > 0.20",
    )


def _d4(
    outcomes: Sequence[ScenarioOutcome], adequate: Mapping[str, bool], settings: AnalysisSettings, suffix: str
) -> EndpointResult:
    ok = {key for key, flag in adequate.items() if flag}
    counts, estimate = _scenario_rate(outcomes, ok, GateOutcome.BLOCK, settings)
    return _rate_endpoint(
        "D4", f"scenario false-block rate over ADEQUATE scenarios ({suffix})", counts, estimate,
        k.D4_GO_POINT, k.D4_GO_UPPER, k.D4_NO_GO_LOWER, "point <= 0.10 and upper <= 0.25", "lower > 0.25",
    )


def _d5(outcomes: Sequence[ScenarioOutcome], settings: AnalysisSettings, suffix: str) -> EndpointResult:
    everything = {o.scenario.key for o in outcomes}
    counts, estimate = _scenario_rate(outcomes, everything, GateOutcome.ESCALATE, settings)
    return _rate_endpoint(
        "D5", f"escalation rate over all scenarios ({suffix})", counts, estimate,
        k.D5_GO_POINT, k.D5_GO_UPPER, k.D5_NO_GO_LOWER, "point <= 0.30 and upper <= 0.45", "lower > 0.50",
    )


# ---------------------------------------------------------------- validity endpoints D6 to D8


def validity_endpoints(ctx: Context, settings: AnalysisSettings) -> list[EndpointResult]:
    return [_d6(ctx, settings), _d7(ctx), _d8(ctx)]


def _d6(ctx: Context, settings: AnalysisSettings) -> EndpointResult:
    per_cluster: dict[int, list[int]] = defaultdict(lambda: [0, 0])
    for view in ctx.arm_t_evaluated():
        if view.primary_criterion is None:
            continue
        cell = per_cluster[view.primary_criterion]
        cell[DENOMINATOR] += 1
        cell[NUMERATOR] += int(ctx.oracle_valid_for(view.mutant_id, view.target))
    counts = RateCounts({c: (v[0], v[1]) for c, v in per_cluster.items()})
    estimate = rate_estimate(counts, settings)
    base_valid, base_total = n7_base_rate(ctx)
    base_rate = base_valid / base_total if base_total else NAN
    point = estimate.point

    def outcome_at(n: int) -> Outcome:
        return bands.valid_precision(Estimate(point, rescaled_wilson(point, n)), base_rate)

    margin = estimate.interval.lo - k.D6_GO_LOWER
    return _result(
        "D6", 2, "VALID precision against oracle-valid-for-target, arm T (primary-matrix VALID mutants)",
        counts.denominator, estimate,
        "point >= 0.80, lower >= 0.70, precision - N7 >= 0.10 (waived if N7 >= 0.90)", "upper < 0.70",
        bands.valid_precision(estimate, base_rate), margin,
        {"oracle_valid": counts.numerator, "valid": counts.denominator, "n7": base_rate,
         "n7_counts": [base_valid, base_total]},
        outcome_at,
    )


def _d7(ctx: Context) -> EndpointResult:
    regression_valid = regression_total = neutral_valid = neutral_total = 0
    for item in ctx.truth.planted.values():
        view = ctx.run.classes.get(item.mutant_id)
        if item.kind == k.KIND_REGRESSION:
            regression_total += 1
            regression_valid += int(view is not None and view.verdict_for(item.declared_target) == k.MUTANT_VALID)
        else:
            neutral_total += 1
            neutral_valid += int(view is not None and bool(view.valid_criteria))
    outcome = bands.planted_controls(regression_valid, regression_total, neutral_valid, neutral_total)
    point = regression_valid / regression_total if regression_total else NAN
    neutral_point = neutral_valid / neutral_total if neutral_total else NAN
    margin = min(point - k.D7_GO_REGRESSION_MIN / 20, k.D7_GO_NEUTRAL_MAX / 10 - neutral_point)
    estimate = Estimate(point, wilson(regression_valid, regression_total))
    components = [
        {"name": "r-items VALID for their criterion", "point": point, "lo": estimate.interval.lo,
         "hi": estimate.interval.hi, "count": regression_valid, "total": regression_total},
        {"name": "n1-items VALID for any criterion", "point": neutral_point,
         "lo": wilson(neutral_valid, neutral_total).lo, "hi": wilson(neutral_valid, neutral_total).hi,
         "count": neutral_valid, "total": neutral_total},
    ]
    return _result(
        "D7", 2, "planted controls: r-items VALID for their criterion, n1-items VALID for any",
        regression_total + neutral_total, estimate, ">= 16/20 r-items and <= 1/10 n1-items",
        "<= 10/20 r-items, or >= 3/10 n1-items", outcome, margin,
        {"r_valid": regression_valid, "r_total": regression_total, "n1_valid": neutral_valid,
         "n1_total": neutral_total},
        None, components,
    )


def _d8(ctx: Context) -> EndpointResult:
    valid_by_criterion: dict[int, int] = defaultdict(int)
    for view in ctx.arm_t_evaluated():
        if view.primary_criterion is not None:
            valid_by_criterion[view.primary_criterion] += 1
    enough = sum(1 for c in ctx.criteria if valid_by_criterion[c] >= k.D8_MIN_VALID_PER_CRITERION)
    total = len(ctx.criteria)
    point = enough / total if total else NAN
    return _result(
        "D8", 2, "criteria with >= 2 VALID arm-T mutants", total, Estimate(point, wilson(enough, total)),
        "10/10", "<= 7/10", bands.criteria_with_valid_mutants(enough, total),
        enough / k.D8_GO_CRITERIA - 1 if total else NAN,
        {"valid_by_criterion": {criterion_ref(c): valid_by_criterion[c] for c in ctx.criteria}},
        None,
    )


# ---------------------------------------------------------------- judge endpoints D9 to D11


def judge_verdicts(ctx: Context, judge: str, arm: str, use_s1: bool) -> dict[str, AlignmentVerdict]:
    variant = GateVariant(name=f"{judge} {arm}", judge=judge, alignment_arm=arm, use_s1=use_s1)
    return alignment_verdicts(ctx, variant)


def _tests_of(ctx: Context, subset: Collection[str] | None) -> list[str]:
    return [n for n in ctx.nodeids if subset is None or n in subset]


def judge_endpoint(
    ctx: Context,
    verdicts: Mapping[str, AlignmentVerdict],
    settings: AnalysisSettings,
    subset: Collection[str] | None = None,
    label: str = "J-SONNET NAME-MASKED",
) -> EndpointResult:
    """D9: 'predicted bad' (MISALIGNED or UNDETERMINED) against gold bad tests."""
    tests = _tests_of(ctx, subset)
    cluster: dict[int, list[int]] = defaultdict(lambda: [0, 0, 0])
    for nodeid in tests:
        cell = cluster[ctx.labels[nodeid].cites]
        gold_bad = not ctx.gold_good(nodeid)
        predicted_bad = verdicts[nodeid] is not AlignmentVerdict.ALIGNED
        cell[0] += int(gold_bad and predicted_bad)
        cell[1] += int(gold_bad)
        cell[2] += int(predicted_bad)
    array = np.array(list(cluster.values()), dtype=np.float64).reshape(-1, 3)
    true_positive, bad_total, predicted_total = (int(array[:, i].sum()) for i in range(3))
    recall = _ratio_estimate(true_positive, bad_total, array[:, [0, 1]], settings)
    precision = _ratio_estimate(true_positive, predicted_total, array[:, [0, 2]], settings)
    n2 = n2_always_misaligned_precision([not ctx.gold_good(n) for n in tests])
    outcome = bands.bad_test_detection(recall, precision, n2)
    least = recall if recall.interval.lo <= precision.interval.lo else precision

    def outcome_at(n: int) -> Outcome:
        scaled_recall = Estimate(recall.point, rescaled_wilson(recall.point, n))
        scaled_precision = Estimate(precision.point, rescaled_wilson(precision.point, n))
        return bands.bad_test_detection(scaled_recall, scaled_precision, n2)

    components = [_component("recall", recall, true_positive, bad_total),
                  _component("precision", precision, true_positive, predicted_total)]
    return _result(
        "D9", 3, f"{label} bad-test detection (predicted bad = MISALIGNED or UNDETERMINED; gold bad n = {bad_total})",
        len(tests), least,
        "recall and precision point >= 0.80, lower >= 0.60, precision - N2 >= 0.10", "either upper < 0.70",
        outcome, min(recall.interval.lo, precision.interval.lo) - k.D9_GO_LOWER,
        {"n2_precision": n2, "true_positive": true_positive, "predicted_bad": predicted_total,
         "gold_bad": bad_total, "recall_point": recall.point, "precision_point": precision.point},
        outcome_at, components,
    )


def _ratio_estimate(num: int, den: int, clusters: FloatArray, settings: AnalysisSettings) -> Estimate:
    boot = bootstrap_interval(clusters, ratio_statistic(0, 1), settings.resamples, settings.seed)
    return Estimate(num / den if den else NAN, _wider(wilson(num, den), boot))


def _component(name: str, estimate: Estimate, count: int, total: int) -> dict[str, object]:
    return {"name": name, "point": estimate.point, "lo": estimate.interval.lo, "hi": estimate.interval.hi,
            "count": count, "total": total}


@dataclass(frozen=True)
class MatrixNulls:
    mean_dc_null: FloatArray
    balanced_accuracy_null: FloatArray

    @property
    def dc_threshold(self) -> float:
        return null_threshold(self.mean_dc_null)

    @property
    def dc_mean(self) -> float:
        return null_mean(self.mean_dc_null)

    @property
    def balanced_accuracy_mean(self) -> float:
        return null_mean(self.balanced_accuracy_null)


def compute_matrix_nulls(ctx: Context, settings: AnalysisSettings) -> MatrixNulls:
    return MatrixNulls(
        n3_null_mean_dc(ctx, settings.permutations, settings.seed),
        n6_null_balanced_accuracy(ctx, settings.permutations, settings.seed),
    )


def matrix_signatures(ctx: Context) -> dict[str, TestSignature]:
    pool, unattributed = matrix_pools(ctx)
    return classify_tests(build_evidence(ctx.declared, pool, unattributed), ctx.declared)


def matrix_endpoint(
    ctx: Context,
    signatures: Mapping[str, TestSignature],
    nulls: MatrixNulls,
    settings: AnalysisSettings,
    subset: Collection[str] | None = None,
) -> EndpointResult:
    """D10: balanced accuracy of the matrix binary verdict over the tests."""
    tests = _tests_of(ctx, subset)
    predicted = matrix_predicted_bad(signatures, tests)
    gold_bad = [not ctx.gold_good(n) for n in tests]
    scores = balanced_scores(gold_bad, predicted)
    cluster: dict[int, list[int]] = defaultdict(lambda: [0, 0, 0, 0])
    for nodeid, is_bad, is_predicted_bad in zip(tests, gold_bad, predicted, strict=True):
        cell = cluster[ctx.labels[nodeid].cites]
        cell[0 if is_bad else 2] += int(is_predicted_bad == is_bad)
        cell[1 if is_bad else 3] += 1
    array = np.array(list(cluster.values()), dtype=np.float64).reshape(-1, 4)
    bad_total, good_total = int(array[:, 1].sum()), int(array[:, 3].sum())
    wilson_sens = wilson(int(array[:, 0].sum()), bad_total)
    wilson_spec = wilson(int(array[:, 2].sum()), good_total)
    wilson_mean = Interval((wilson_sens.lo + wilson_spec.lo) / 2, (wilson_sens.hi + wilson_spec.hi) / 2)
    boot = bootstrap_interval(array, balanced_accuracy_statistic, settings.resamples, settings.seed)
    estimate = Estimate(scores.balanced_accuracy, _wider(wilson_mean, boot))
    observed_dc = mean_dc(signatures[n] for n in tests)
    threshold, dc_chance = nulls.dc_threshold, nulls.dc_mean
    n6 = nulls.balanced_accuracy_mean
    outcome = bands.matrix_balanced_accuracy(estimate, n6, observed_dc, threshold, dc_chance)
    point = estimate.point

    def outcome_at(n: int) -> Outcome:
        shrunk = scaled_interval(point, estimate.interval, len(tests), n)
        return bands.matrix_balanced_accuracy(Estimate(point, shrunk), n6, observed_dc, threshold, dc_chance)

    return _result(
        "D10", 3, "matrix binary verdict, balanced accuracy over the tests (UNDETERMINED counts as predicted bad)",
        len(tests), estimate,
        "point >= 0.80, lower >= 0.65, lower - N6 mean > 0, mean DC above the N3 95th percentile",
        "upper < 0.70, or mean DC not above the N3 mean", outcome, estimate.interval.lo - k.D10_GO_LOWER,
        {"sensitivity": scores.sensitivity, "specificity": scores.specificity, "mean_dc": observed_dc,
         "n3_p95": threshold, "n3_mean": dc_chance, "n6_mean": n6, "n6_p95": null_threshold(nulls.balanced_accuracy_null)},
        outcome_at,
    )


def cross_check_endpoint(
    ctx: Context,
    verdicts: Mapping[str, AlignmentVerdict],
    signatures: Mapping[str, TestSignature],
    settings: AnalysisSettings,
    subset: Collection[str] | None = None,
    label: str = "J-SONNET",
) -> EndpointResult:
    """D11: Cohen's kappa between the judge's and the matrix's binary verdict on determinate tests."""
    determinate = (AlignmentVerdict.ALIGNED, AlignmentVerdict.MISALIGNED)
    cluster: dict[int, np.ndarray] = defaultdict(lambda: np.zeros(4))
    used = 0
    for nodeid in _tests_of(ctx, subset):
        judge, matrix = verdicts[nodeid], signatures[nodeid].verdict
        if judge not in determinate or matrix not in determinate:
            continue
        used += 1
        cluster[ctx.labels[nodeid].cites][determinate.index(judge) * 2 + determinate.index(matrix)] += 1
    array = np.array(list(cluster.values()), dtype=np.float64).reshape(-1, 4)
    point = float(cohen_kappa_tables(array.sum(axis=0).reshape(1, 2, 2))[0]) if used else NAN
    interval = bootstrap_interval(array, cohen_kappa_statistic(2), settings.resamples, settings.seed)
    estimate = Estimate(point, interval)
    outcome = bands.cross_check_kappa(estimate)

    def outcome_at(n: int) -> Outcome:
        return bands.cross_check_kappa(Estimate(point, scaled_interval(point, interval, max(used, 1), n)))

    return _result(
        "D11", 3, f"Cohen's kappa, {label} binary verdict vs matrix binary verdict (determinate items)",
        used, estimate, "point >= 0.60, lower >= 0.30", "upper < 0.40", outcome,
        interval.lo - k.D11_GO_LOWER if interval.is_defined else NAN,
        {"determinate_items": used, "table": array.sum(axis=0).tolist()}, outcome_at,
    )


def matrix_diagnostics(signatures: Mapping[str, TestSignature]) -> dict[str, float | None]:
    return {"mean_dc": mean_dc(signatures.values())}


def nan_to_none(value: float) -> float | None:
    return None if math.isnan(value) else value
