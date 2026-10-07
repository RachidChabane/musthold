"""Sensitivity analyses (§13): every one is reported and none feeds a band or a rule."""

from __future__ import annotations

from collections.abc import Callable, Sequence
from dataclasses import replace
from typing import Final

from calibration.analysis.cell_metrics import MetricInput, MetricSettings
from calibration.analysis.model import CellData, Decision, Kind
from calibration.analysis.stats.arrays import Rows, exact_mean
from calibration.analysis.stats.bootstrap import ClusterIndex, Resamples
from calibration.analysis.stats.calibration import brier, ece_equal_width
from calibration.analysis.stats.intervals import BOOTSTRAP_METHOD, Estimate, Proportion, decision_interval
from calibration.analysis.stats.ranking import auroc
from calibration.domain.canonical import JsonValue
from calibration.domain.constants import ECE_BINS

TAG_ARM: Final = "arm"
TAG_F3: Final = "f3"
TAG_TARGET: Final = "target"
TAG_KILL: Final = "kill"
F3_DIFFERS: Final = "DIFFERS"
STRATA_TAGS: Final = (TAG_ARM, TAG_F3, TAG_TARGET, TAG_KILL)

Selector = Callable[[Decision], bool]


def summary(kind: Kind, decisions: Sequence[Decision], settings: MetricSettings) -> dict[str, JsonValue]:
    """n, clusters, accuracy, AUROC, ECE and Brier of a set of decisions with their intervals."""
    predicted = [d for d in decisions if d.has_prediction]
    if not predicted:
        return {"n": 0}
    data = MetricInput.of(kind, predicted)
    p, y = data.primary
    correct = data.correct
    index = ClusterIndex.build(data.clusters)
    resamples = Resamples.draw(index, settings.resamples, settings.seed)

    def bootstrap(statistic: Callable[[Rows], float]) -> dict[str, float | str | None]:
        interval = resamples.interval(statistic)
        return dict(Estimate(statistic(index.all_rows()), interval.lo, interval.hi, BOOTSTRAP_METHOD).to_record())

    accuracy = Proportion(int(correct.sum()), data.size)
    accuracy_interval = decision_interval(accuracy.wilson, resamples.interval(lambda r: exact_mean(correct[r])))
    return {
        "n": data.size,
        "clusters": index.size,
        "accuracy": dict(Estimate(accuracy.point, accuracy_interval.lo, accuracy_interval.hi, "decision").to_record()),
        "auroc": bootstrap(lambda r: auroc(p[r], y[r])),
        "ece": bootstrap(lambda r: ece_equal_width(p[r], y[r], ECE_BINS)),
        "brier": bootstrap(lambda r: brier(p[r], y[r])),
    }


def with_returned_probability(decisions: Sequence[Decision]) -> list[Decision]:
    """The returned-probability measure in place of the product confidence (S1, K1 to K3) or the weakest stated
    confidence in place of the agreement share (VH2); decisions without the alternative measure drop out."""
    swapped: list[Decision] = []
    for decision in decisions:
        if decision.alt_confidence is None:
            continue
        swapped.append(
            replace(
                decision,
                confidence=decision.alt_confidence,
                event_probability=decision.alt_event_probability
                if decision.alt_event_probability is not None
                else decision.event_probability,
            )
        )
    return swapped


def with_lenient_truth(decisions: Sequence[Decision]) -> list[Decision]:
    """K1 with the lenient truth variant of §3.3."""
    return [replace(d, correct=bool(d.lenient_correct)) for d in decisions if d.lenient_correct is not None]


def clustered_by_criterion(decisions: Sequence[Decision]) -> list[Decision]:
    """K4 clustered by the criterion c of each pair instead of by mutant source."""
    return [replace(d, cluster=d.unit.rsplit("|", 1)[-1]) for d in decisions]


def select(decisions: Sequence[Decision], keep: Selector) -> list[Decision]:
    return [d for d in decisions if keep(d)]


def consensus_only(decisions: Sequence[Decision]) -> list[Decision]:
    return select(decisions, lambda d: d.consensus)


def stratum(decisions: Sequence[Decision], tag: str, value: str) -> list[Decision]:
    return select(decisions, lambda d: d.tags.get(tag) == value)


def strata_values(decisions: Sequence[Decision], tag: str) -> list[str]:
    return sorted({d.tags[tag] for d in decisions if tag in d.tags})


def cell_sensitivities(cell: CellData, settings: MetricSettings) -> dict[str, JsonValue]:
    """The sensitivity summaries of one cell, each next to the PRIMARY summary it varies."""
    kind, decisions = cell.kind, cell.predicted
    record: dict[str, JsonValue] = {"primary": summary(kind, decisions, settings)}
    if kind is not Kind.K4:
        record["consensus"] = summary(kind, consensus_only(decisions), settings)
    if kind is Kind.K1:
        record["lenient"] = summary(kind, with_lenient_truth(decisions), settings)
    if any(d.alt_confidence is not None for d in decisions):
        record["alternative_measure"] = summary(kind, with_returned_probability(decisions), settings)
    if kind is Kind.K4:
        record["clustered_by_criterion"] = summary(kind, clustered_by_criterion(decisions), settings)
        record["f3_differs"] = summary(kind, stratum(decisions, TAG_F3, F3_DIFFERS), settings)
        strata: dict[str, JsonValue] = {}
        for tag in STRATA_TAGS:
            for value in strata_values(decisions, tag):
                strata[f"{tag}={value}"] = summary(kind, stratum(decisions, tag, value), settings)
        record["strata"] = strata
    return record
