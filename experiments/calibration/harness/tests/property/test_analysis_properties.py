"""Properties of the analysis path that must hold for every input: bounds, determinism, band rules, monotonicity."""

from __future__ import annotations

import math
from dataclasses import dataclass, replace
from random import Random
from typing import Final

from hypothesis import given, settings
from hypothesis import strategies as st

from calibration.analysis.bands import READY_AUROC_LOWER, Band, band_of
from calibration.analysis.cell_metrics import CellMetrics, MetricSettings, compute_cell_metrics
from calibration.analysis.model import CellData, Decision, Kind
from calibration.analysis.source3 import RULE_CONFIDENCE, OperatingPoint, undetermined_figures
from calibration.analysis.stats.intervals import Estimate
from calibration.analysis.votes import JudgeVote, aggregate_judge_votes
from calibration.domain.deviations import Standing
from calibration.domain.sources import SourceId
from support.check_cases import cell, decision

PROFILE = settings(max_examples=40, deadline=None)
RESAMPLES: Final = 20
METRIC_SETTINGS: Final = MetricSettings(resamples=RESAMPLES)
CLUSTER_COUNT: Final = 4
UNIT_INTERVAL = st.floats(min_value=0.0, max_value=1.0, allow_nan=False)
ESTIMATE_TOLERANCE: Final = 1e-9
METHOD: Final = "bootstrap"
REFS: Final = ("PARK-1~1", "PARK-2~1", "NONE")
LEVELS: Final = ("NONE", "WEAK", "PARTIAL", "FULL")
STATED_CONFIDENCES: Final = (0.5, 0.6, 0.75, 0.9)
FIXED_ROWS: Final = 24


@dataclass(frozen=True)
class Row:
    cluster: int
    confidence: float
    event: float
    correct: bool
    truth: bool


ROWS = st.builds(
    Row,
    st.integers(min_value=0, max_value=CLUSTER_COUNT - 1),
    UNIT_INTERVAL,
    UNIT_INTERVAL,
    st.booleans(),
    st.booleans(),
)


def _decisions(rows: list[Row]) -> list[Decision]:
    return [
        decision(
            f"u{index}",
            f"c{row.cluster}",
            row.confidence,
            row.correct,
            event=row.event,
            truth=row.truth,
            predicted=row.event >= 0.5,
        )
        for index, row in enumerate(rows)
    ]


def _metrics(rows: list[Row], kind: Kind = Kind.K3) -> CellMetrics:
    metrics = compute_cell_metrics(cell(kind, _decisions(rows)), METRIC_SETTINGS, equal_mass=False)
    assert metrics is not None
    return metrics


def _same(first: Estimate | None, second: Estimate | None) -> bool:
    if first is None or second is None:
        return first is second

    def close(a: float, b: float) -> bool:
        return (math.isnan(a) and math.isnan(b)) or math.isclose(a, b, abs_tol=ESTIMATE_TOLERANCE)

    return close(first.point, second.point) and close(first.lo, second.lo) and close(first.hi, second.hi)


class TestBounds:
    @PROFILE
    @given(st.lists(ROWS, min_size=1, max_size=30))
    def test_ece_and_its_interval_stay_in_the_unit_interval(self, rows: list[Row]) -> None:
        metrics = _metrics(rows)
        for estimate in (metrics.ece, metrics.ece_top_label):
            assert estimate is not None
            for value in (estimate.point, estimate.lo, estimate.hi):
                assert math.isnan(value) or 0.0 <= value <= 1.0

    @PROFILE
    @given(st.lists(ROWS, min_size=1, max_size=30))
    def test_brier_and_accuracy_stay_in_the_unit_interval(self, rows: list[Row]) -> None:
        metrics = _metrics(rows)
        assert 0.0 <= metrics.brier.point <= 1.0
        assert 0.0 <= metrics.accuracy.point <= 1.0


class TestDeterminism:
    @PROFILE
    @given(st.lists(ROWS, min_size=2, max_size=30), st.randoms(use_true_random=False))
    def test_the_metrics_do_not_depend_on_the_order_of_the_decisions(self, rows: list[Row], random: Random) -> None:
        shuffled = list(rows)
        random.shuffle(shuffled)
        first, second = _metrics(rows), _metrics(shuffled)
        for name in ("accuracy", "auroc", "ece", "brier", "brier_skill", "q09", "aurc", "c05"):
            assert _same(getattr(first, name), getattr(second, name)), name

    @PROFILE
    @given(st.lists(ROWS, min_size=1, max_size=30))
    def test_the_same_input_gives_the_same_metrics(self, rows: list[Row]) -> None:
        first, second = _metrics(rows), _metrics(rows)
        assert _same(first.auroc, second.auroc)
        assert _same(first.ece, second.ece)


def _ready_metrics() -> CellMetrics:
    rows = [
        Row((index // 2) % CLUSTER_COUNT, 1.0, float(index % 2), True, index % 2 == 1) for index in range(FIXED_ROWS)
    ]
    return _metrics(rows)


class TestBands:
    @PROFILE
    @given(st.floats(min_value=0.0, max_value=READY_AUROC_LOWER - 1e-6), UNIT_INTERVAL)
    def test_no_cell_with_an_auroc_lower_bound_below_the_ready_limit_is_threshold_ready(
        self, lower: float, upper_gap: float
    ) -> None:
        base = _ready_metrics()
        auroc = Estimate(lower, lower, lower + (1.0 - lower) * upper_gap, METHOD)
        data = cell(Kind.K3, _decisions([Row(0, 1.0, 1.0, True, True)]))
        verdict = band_of(data, replace(base, auroc=auroc), None, Standing(blocked=False, protocol=False))
        assert verdict.band is not Band.THRESHOLD_READY

    def test_the_reference_metrics_are_ready_when_the_lower_bound_allows_it(self) -> None:
        data = cell(Kind.K3, _decisions([Row(0, 1.0, 1.0, True, True)]))
        verdict = band_of(data, _ready_metrics(), None, Standing(blocked=False, protocol=False))
        assert verdict.band is Band.THRESHOLD_READY


class TestUndetermined:
    @PROFILE
    @given(
        st.lists(st.tuples(UNIT_INTERVAL, st.booleans()), min_size=1, max_size=30),
        UNIT_INTERVAL,
        UNIT_INTERVAL,
    )
    def test_the_undetermined_rate_does_not_decrease_with_the_threshold(
        self, rows: list[tuple[float, bool]], first: float, second: float
    ) -> None:
        low, high = sorted((first, second))
        decisions = [
            decision(f"u{index}", "c", confidence, True, has_prediction=predicted)
            for index, (confidence, predicted) in enumerate(rows)
        ]
        data: CellData = cell(Kind.K3, decisions, source=SourceId.S1)
        rates = [
            undetermined_figures(data, OperatingPoint(RULE_CONFIDENCE, threshold, ""))["undetermined_rate"]
            for threshold in (low, high)
        ]
        assert isinstance(rates[0], float)
        assert isinstance(rates[1], float)
        assert rates[0] <= rates[1]


VOTES = st.lists(
    st.builds(
        JudgeVote,
        st.sampled_from(REFS),
        st.sampled_from(LEVELS),
        st.sampled_from(STATED_CONFIDENCES),
    ),
    min_size=1,
    max_size=5,
)


class TestJudgeAggregate:
    @PROFILE
    @given(VOTES, st.randoms(use_true_random=False), st.integers(min_value=0, max_value=2))
    def test_the_aggregate_does_not_depend_on_the_order_of_the_votes(
        self, votes: list[JudgeVote], random: Random, missing: int
    ) -> None:
        shuffled = list(votes)
        random.shuffle(shuffled)
        total = len(votes) + missing
        assert aggregate_judge_votes(votes, total) == aggregate_judge_votes(shuffled, total)
