"""The metrics of one (kind, source, model) cell (§8): calibration, discrimination, risk-coverage and the K3 and K4
quantities, each with the interval §8.5 prescribes."""

from __future__ import annotations

from collections.abc import Callable, Mapping, Sequence
from dataclasses import dataclass
from typing import Final

import numpy as np

from calibration.analysis.model import CellData, Decision, Kind
from calibration.analysis.stats.arrays import NAN, Flags, Floats, Mask, Rows, exact_mean
from calibration.analysis.stats.bootstrap import ClusterIndex, Resamples, percentile_interval
from calibration.analysis.stats.calibration import (
    ReliabilityRow,
    accuracy_in_window,
    brier,
    brier_skill,
    ece_equal_mass,
    ece_equal_width,
    reliability_table,
)
from calibration.analysis.stats.intervals import (
    BOOTSTRAP_METHOD,
    DECISION_METHOD,
    UNDEFINED_INTERVAL,
    Estimate,
    Interval,
    Proportion,
    decision_interval,
)
from calibration.analysis.stats.ranking import auroc
from calibration.analysis.stats.risk_coverage import (
    RiskCoverageCurve,
    aurc,
    c05,
    c05u,
    false_aligned_rate,
    risk_coverage_curve,
    risk_upper_bounds,
)
from calibration.domain.constants import (
    BOOTSTRAP_RESAMPLES,
    ECE_BINS,
    EQUAL_MASS_BINS_K4,
    EQUAL_MASS_BINS_K13,
    GATE_THRESHOLD_S1,
    Q09_HIGH,
    Q09_LOW,
    SCREEN_GRID,
    SEED,
)

STRATUM_SCREEN: Final = "screen"
YES: Final = "yes"
POINT_ONLY_METHOD: Final = "decision-interval rule"

Statistic = Callable[[Rows], float]


@dataclass(frozen=True)
class MetricSettings:
    resamples: int = BOOTSTRAP_RESAMPLES
    seed: int = SEED


@dataclass(frozen=True)
class MetricInput:
    """The decisions with a prediction, as arrays: confidence, correctness, primary event and truth."""

    kind: Kind
    clusters: tuple[str, ...]
    confidence: Floats
    correct: Flags
    event: Floats
    truth: Flags
    predicted_positive: Mask
    screen_stratum: Mask

    @classmethod
    def of(cls, kind: Kind, decisions: Sequence[Decision]) -> MetricInput:
        return cls(
            kind=kind,
            clusters=tuple(d.cluster for d in decisions),
            confidence=np.array([d.confidence for d in decisions], dtype=np.float64),
            correct=np.array([d.correct for d in decisions], dtype=np.int64),
            event=np.array([_nan(d.event_probability) for d in decisions], dtype=np.float64),
            truth=np.array([bool(d.truth_positive) for d in decisions], dtype=np.int64),
            predicted_positive=np.array([bool(d.predicted_positive) for d in decisions], dtype=np.bool_),
            screen_stratum=np.array([d.tags.get(STRATUM_SCREEN) == YES for d in decisions], dtype=np.bool_),
        )

    @property
    def size(self) -> int:
        return len(self.clusters)

    @property
    def primary(self) -> tuple[Floats, Flags]:
        """K1: the top-label confidence against correctness; K2 to K4: the positive-event probability against truth."""
        if self.kind is Kind.K1:
            return self.confidence, self.correct
        return self.event, self.truth


def _nan(value: float | None) -> float:
    return NAN if value is None else value


@dataclass(frozen=True)
class GridPoint:
    """The screen quantities at one tau: the miss and flag rates with their decision intervals."""

    tau: float
    miss: Estimate
    flag: Estimate
    positives: int
    rows: int


@dataclass(frozen=True)
class CellMetrics:
    n: int
    clusters: int
    accuracy: Estimate
    majority_accuracy: float
    base_rate: float
    auroc: Estimate
    ece: Estimate
    ece_top_label: Estimate | None
    ece_equal_mass: Estimate | None
    brier: Estimate
    brier_skill: Estimate
    q09: Estimate
    q09_n: int
    aurc: Estimate
    c05: Estimate
    c05u: float
    reliability: tuple[ReliabilityRow, ...]
    curve: RiskCoverageCurve
    screen_all: tuple[GridPoint, ...]
    screen_stratum: tuple[GridPoint, ...]
    false_aligned: Estimate | None
    false_aligned_threshold: float | None
    false_aligned_counts: tuple[int, int] | None
    baselines: Mapping[str, float]


def _bootstrap(resamples: Resamples, statistic: Statistic) -> Estimate:
    interval = resamples.interval(statistic)
    return Estimate(statistic(resamples.index.all_rows()), interval.lo, interval.hi, BOOTSTRAP_METHOD)


def _proportion_decision(resamples: Resamples, proportion: Proportion, statistic: Statistic) -> Estimate:
    """A proportion's decision interval: the wider of Wilson and the cluster bootstrap (§8.5)."""
    if proportion.total == 0:
        return Estimate(NAN, NAN, NAN, DECISION_METHOD)
    interval = decision_interval(proportion.wilson, resamples.interval(statistic))
    return Estimate(proportion.point, interval.lo, interval.hi, DECISION_METHOD)


def _ratio(numerator: Floats, denominator: Floats) -> Floats:
    out = np.full(numerator.shape, NAN)
    np.divide(numerator, denominator, out=out, where=denominator != 0)
    return out


def _decision(proportion: Proportion, bootstrap: Interval) -> Interval:
    if proportion.total == 0:
        return UNDEFINED_INTERVAL
    return decision_interval(proportion.wilson, bootstrap)


def screen_grid(resamples: Resamples, p: Floats, y: Flags, members: Mask) -> tuple[GridPoint, ...]:
    """Miss and flag rates over the screen grid on the member rows (§8.3), bootstrapped for every tau at once with
    the draws of the whole cell; a cluster without member rows contributes nothing."""
    if not members.any():
        return ()
    taus = np.array(SCREEN_GRID, dtype=np.float64)
    inside = members.astype(np.float64)
    positives = (y == 1).astype(np.float64) * inside
    below = p[:, np.newaxis] < taus[np.newaxis, :]
    columns = np.concatenate(
        (
            positives[:, np.newaxis],
            inside[:, np.newaxis],
            positives[:, np.newaxis] * below,
            inside[:, np.newaxis] * (~below),
        ),
        axis=1,
    )
    summed = resamples.summed_counts(resamples.index.cluster_sums(columns))
    width = taus.size
    miss_boot = _ratio(summed[:, 2 : 2 + width], summed[:, 0:1])
    flag_boot = _ratio(summed[:, 2 + width :], summed[:, 1:2])
    points: list[GridPoint] = []
    for position, tau in enumerate(SCREEN_GRID):
        miss = Proportion(int((positives * (p < tau)).sum()), int(positives.sum()))
        flag = Proportion(int((inside * (p >= tau)).sum()), int(inside.sum()))
        miss_interval = _decision(miss, percentile_interval(miss_boot[:, position]))
        flag_interval = _decision(flag, percentile_interval(flag_boot[:, position]))
        points.append(
            GridPoint(
                tau,
                Estimate(miss.point, miss_interval.lo, miss_interval.hi, DECISION_METHOD),
                Estimate(flag.point, flag_interval.lo, flag_interval.hi, DECISION_METHOD),
                int(positives.sum()),
                int(inside.sum()),
            )
        )
    return tuple(points)


def majority_accuracy(decisions: Sequence[Decision], kind: Kind) -> float:
    """The accuracy of the best constant predictor on the same items."""
    if not decisions:
        return NAN
    if kind is Kind.K1:
        references = {ref for d in decisions for ref in (d.accepted or frozenset())}
        return max(
            exact_mean(np.array([ref in (d.accepted or frozenset()) for d in decisions], dtype=np.float64))
            for ref in references
        )
    base = exact_mean(np.array([bool(d.truth_positive) for d in decisions], dtype=np.float64))
    return max(base, 1.0 - base)


def baseline_figures(decisions: Sequence[Decision], kind: Kind) -> dict[str, float]:
    """Trivial baselines of §9: the majority constant, 'always D(t)' for K1, 'flag everything' and 'flag nothing'."""
    if not decisions:
        return {}
    figures: dict[str, float] = {"majority_accuracy": majority_accuracy(decisions, kind)}
    if kind is Kind.K1:
        figures["always_declared_accuracy"] = exact_mean(
            np.array([d.declared_ref in (d.accepted or frozenset()) for d in decisions], dtype=np.float64)
        )
        return figures
    base = exact_mean(np.array([bool(d.truth_positive) for d in decisions], dtype=np.float64))
    figures["base_rate"] = base
    figures["majority_brier"] = base * (1.0 - base)
    if kind is Kind.K4:
        figures["flag_everything_accuracy"] = base
        figures["flag_nothing_accuracy"] = 1.0 - base
    return figures


def _false_aligned(resamples: Resamples, data: MetricInput, threshold: float) -> tuple[Estimate, float, Proportion]:
    """The false-ALIGNED rate at a confidence threshold with its decision interval (§8.3)."""
    predicted = data.predicted_positive
    truth = data.truth == 1
    proportion = false_aligned_rate(data.confidence, predicted, truth, threshold)

    def rate(rows: Rows) -> float:
        return false_aligned_rate(data.confidence[rows], predicted[rows], truth[rows], threshold).point

    return _proportion_decision(resamples, proportion, rate), threshold, proportion


def compute_cell_metrics(
    cell: CellData, settings: MetricSettings, *, equal_mass: bool, gate_threshold: float = GATE_THRESHOLD_S1
) -> CellMetrics | None:
    """Every metric of §8 for the cell's decisions that carry a prediction; None when there is none."""
    decisions = cell.predicted
    if not decisions:
        return None
    kind = cell.kind
    data = MetricInput.of(kind, decisions)
    p, y = data.primary
    index = ClusterIndex.build(data.clusters)
    resamples = Resamples.draw(index, settings.resamples, settings.seed)
    confidence, correct = data.confidence, data.correct
    accuracy = Proportion(int(correct.sum()), data.size)
    curve = risk_coverage_curve(confidence, correct)
    bootstrap_upper = risk_upper_bounds(resamples, confidence, correct, [point.threshold for point in curve])
    window = accuracy_in_window(confidence, correct, Q09_LOW, Q09_HIGH)
    bins = EQUAL_MASS_BINS_K4 if kind is Kind.K4 else EQUAL_MASS_BINS_K13
    ece_top_label = (
        None
        if kind is Kind.K1
        else _bootstrap(resamples, lambda r: ece_equal_width(confidence[r], correct[r], ECE_BINS))
    )
    ece_mass = _bootstrap(resamples, lambda r: ece_equal_mass(p[r], y[r], bins)) if equal_mass else None
    false_aligned = _false_aligned(resamples, data, gate_threshold) if kind is Kind.K3 else None
    everything = np.ones(data.size, dtype=np.bool_)
    return CellMetrics(
        n=data.size,
        clusters=index.size,
        accuracy=_proportion_decision(resamples, accuracy, lambda r: exact_mean(correct[r])),
        majority_accuracy=majority_accuracy(decisions, kind),
        base_rate=exact_mean(data.truth.astype(np.float64)),
        auroc=_bootstrap(resamples, lambda r: auroc(p[r], y[r])),
        ece=_bootstrap(resamples, lambda r: ece_equal_width(p[r], y[r], ECE_BINS)),
        ece_top_label=ece_top_label,
        ece_equal_mass=ece_mass,
        brier=_bootstrap(resamples, lambda r: brier(p[r], y[r])),
        brier_skill=_bootstrap(resamples, lambda r: brier_skill(p[r], y[r])),
        q09=_proportion_decision(
            resamples, window, lambda r: accuracy_in_window(confidence[r], correct[r], Q09_LOW, Q09_HIGH).point
        ),
        q09_n=window.total,
        aurc=_bootstrap(resamples, lambda r: aurc(risk_coverage_curve(confidence[r], correct[r]))),
        c05=_bootstrap(resamples, lambda r: c05(risk_coverage_curve(confidence[r], correct[r]))),
        c05u=c05u(curve, bootstrap_upper),
        reliability=tuple(reliability_table(p, y, ECE_BINS)),
        curve=curve,
        screen_all=screen_grid(resamples, data.event, data.truth, everything) if kind is Kind.K4 else (),
        screen_stratum=screen_grid(resamples, data.event, data.truth, data.screen_stratum) if kind is Kind.K4 else (),
        false_aligned=false_aligned[0] if false_aligned else None,
        false_aligned_threshold=false_aligned[1] if false_aligned else None,
        false_aligned_counts=(false_aligned[2].successes, false_aligned[2].total) if false_aligned else None,
        baselines=baseline_figures(decisions, kind),
    )
