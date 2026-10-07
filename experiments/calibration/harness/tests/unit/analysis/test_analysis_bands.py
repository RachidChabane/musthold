"""The reading bands of PREREG section 11: one table of cells per band, and the order of the rules."""

from __future__ import annotations

from dataclasses import dataclass, replace
from typing import Final

import pytest

from calibration.analysis.bands import (
    Band,
    admission_capped,
    band_of,
    inconclusive_for_data,
)
from calibration.analysis.cell_metrics import CellMetrics, GridPoint
from calibration.analysis.model import CellData, Kind
from calibration.analysis.stats.admission import AdmissionOutcome
from calibration.analysis.stats.intervals import Estimate
from calibration.domain.deviations import Standing
from calibration.domain.sources import SourceId
from support.metric_cases import base_metrics, make_cell, mixed_event_cell, mixed_top_label_cell

METHOD: Final = "test"
NO_STANDING: Final = Standing(blocked=False, protocol=False)
PROTOCOL_STANDING: Final = Standing(blocked=False, protocol=True)
BLOCKED_STANDING: Final = Standing(blocked=True, protocol=False)
SCREEN_TAU: Final = 0.20
OFF_GRID_TAU: Final = 0.25


def _estimate(point: float, lo: float | None = None, hi: float | None = None) -> Estimate:
    return Estimate(point, point if lo is None else lo, point if hi is None else hi, METHOD)


@dataclass(frozen=True)
class Figures:
    """The figures a rule reads, with defaults that satisfy every THRESHOLD_READY condition."""

    auroc_lo: float = 0.80
    auroc_hi: float = 0.95
    ece_point: float = 0.03
    ece_hi: float = 0.08
    miss_hi: float = 0.05
    flag_point: float = 0.30
    accuracy_hi: float = 0.95
    majority: float = 0.50


def _screen_point(figures: Figures, tau: float = SCREEN_TAU) -> GridPoint:
    return GridPoint(tau, _estimate(0.0, 0.0, figures.miss_hi), _estimate(figures.flag_point), 10, 20)


def _metrics(kind: Kind, figures: Figures, tau: float = SCREEN_TAU) -> CellMetrics:
    return replace(
        base_metrics(kind),
        auroc=_estimate((figures.auroc_lo + figures.auroc_hi) / 2, figures.auroc_lo, figures.auroc_hi),
        ece=_estimate(figures.ece_point, 0.0, figures.ece_hi),
        accuracy=_estimate(0.5, 0.2, figures.accuracy_hi),
        majority_accuracy=figures.majority,
        screen_stratum=(_screen_point(figures, tau),),
    )


def _cell(kind: Kind) -> CellData:
    return mixed_top_label_cell() if kind is Kind.K1 else mixed_event_cell(kind)


@dataclass(frozen=True)
class Case:
    name: str
    kind: Kind
    figures: Figures
    band: Band
    standing: Standing = NO_STANDING


TABLE: Final = (
    Case("ready", Kind.K3, Figures(), Band.THRESHOLD_READY),
    Case("ready at the auroc boundary", Kind.K3, Figures(auroc_lo=0.75), Band.THRESHOLD_READY),
    Case("ready at the ece boundaries", Kind.K3, Figures(ece_point=0.05, ece_hi=0.10), Band.THRESHOLD_READY),
    Case("ece point above 0.05", Kind.K3, Figures(ece_point=0.0501), Band.ROUTE_ONLY),
    Case("ece upper above 0.10", Kind.K3, Figures(ece_hi=0.1001), Band.ROUTE_ONLY),
    Case("auroc lower below 0.75 but above 0.55", Kind.K2, Figures(auroc_lo=0.7499), Band.ROUTE_ONLY),
    Case("route only just above 0.55", Kind.K2, Figures(auroc_lo=0.5501, ece_point=0.3), Band.ROUTE_ONLY),
    Case(
        "route only needs a strictly greater bound", Kind.K2, Figures(auroc_lo=0.55, ece_point=0.3), Band.INCONCLUSIVE
    ),
    Case("useless below 0.60", Kind.K3, Figures(auroc_lo=0.1, auroc_hi=0.5999), Band.USELESS),
    Case("upper bound of 0.60 is not useless", Kind.K3, Figures(auroc_lo=0.1, auroc_hi=0.60), Band.INCONCLUSIVE),
    Case("useless wins over ready", Kind.K3, Figures(auroc_lo=0.8, auroc_hi=0.59), Band.USELESS),
    Case("k1 accuracy upper below the majority", Kind.K1, Figures(accuracy_hi=0.49, majority=0.5), Band.USELESS),
    Case("k1 accuracy upper at the majority", Kind.K1, Figures(accuracy_hi=0.5, majority=0.5), Band.THRESHOLD_READY),
    Case("k3 ignores the majority accuracy", Kind.K3, Figures(accuracy_hi=0.1, majority=0.5), Band.THRESHOLD_READY),
    Case("k4 ready", Kind.K4, Figures(miss_hi=0.10, flag_point=0.50), Band.THRESHOLD_READY),
    Case("k4 miss upper above 0.10", Kind.K4, Figures(miss_hi=0.1001), Band.ROUTE_ONLY),
    Case("k4 flag point above 0.50", Kind.K4, Figures(flag_point=0.5001), Band.ROUTE_ONLY),
    Case("k4 without the tau 0.20 point", Kind.K4, Figures(), Band.ROUTE_ONLY),
    Case("protocol caps ready", Kind.K3, Figures(), Band.INCONCLUSIVE, PROTOCOL_STANDING),
    Case("protocol does not touch route only", Kind.K3, Figures(ece_point=0.3), Band.ROUTE_ONLY, PROTOCOL_STANDING),
    Case("protocol does not touch useless", Kind.K3, Figures(auroc_hi=0.5), Band.USELESS, PROTOCOL_STANDING),
    Case("nothing places the cell", Kind.K3, Figures(auroc_lo=0.3, auroc_hi=0.9, ece_point=0.3), Band.INCONCLUSIVE),
)


@pytest.mark.parametrize("case", TABLE, ids=[case.name for case in TABLE])
def test_each_cell_of_the_table_lands_in_its_band(case: Case) -> None:
    tau = OFF_GRID_TAU if case.name == "k4 without the tau 0.20 point" else SCREEN_TAU
    verdict = band_of(_cell(case.kind), _metrics(case.kind, case.figures, tau), None, case.standing)
    assert verdict.band is case.band


def test_a_protocol_cap_names_itself() -> None:
    verdict = band_of(_cell(Kind.K3), _metrics(Kind.K3, Figures()), None, PROTOCOL_STANDING)
    assert verdict.capped_by == ("protocol",)
    uncapped = band_of(_cell(Kind.K3), _metrics(Kind.K3, Figures()), None, NO_STANDING)
    assert uncapped.capped_by == ()


def test_a_blocked_reason_comes_first_whatever_the_figures() -> None:
    verdict = band_of(_cell(Kind.K3), _metrics(Kind.K3, Figures()), "memory-fit", NO_STANDING)
    assert verdict.band is Band.BLOCKED
    assert verdict.reason == "memory-fit"


def test_a_blocked_entry_in_scope_blocks_the_cell() -> None:
    verdict = band_of(_cell(Kind.K3), _metrics(Kind.K3, Figures()), None, BLOCKED_STANDING)
    assert verdict.band is Band.BLOCKED


def test_blocked_wins_over_the_data_rule() -> None:
    cell = make_cell(Kind.K3, _cell(Kind.K3).decisions, units=12, fall_through={"MISSING": 12})
    assert band_of(cell, None, "x", NO_STANDING).band is Band.BLOCKED


def test_the_data_rule_wins_over_the_metric_rules() -> None:
    cell = make_cell(Kind.K3, _cell(Kind.K3).decisions, units=20, fall_through={"MISSING": 3})
    verdict = band_of(cell, _metrics(Kind.K3, Figures()), None, NO_STANDING)
    assert verdict.band is Band.INCONCLUSIVE_DATA


def test_a_cell_without_metrics_is_inconclusive() -> None:
    verdict = band_of(_cell(Kind.K3), None, None, NO_STANDING)
    assert verdict.band is Band.INCONCLUSIVE


@pytest.mark.parametrize(
    ("units", "lost", "expected"),
    [(10, 1, False), (100, 11, True), (100, 10, False), (20, 3, True), (20, 2, False), (10, 0, False)],
)
def test_fall_throughs_above_a_tenth_of_the_units_are_inconclusive_for_data(
    units: int, lost: int, expected: bool
) -> None:
    cell = make_cell(Kind.K3, [], units=units, fall_through={"MISSING": lost})
    assert inconclusive_for_data(cell) is expected


def test_the_three_fall_through_types_add_up() -> None:
    cell = make_cell(Kind.K1, [], units=20, fall_through={"MISSING": 1, "STATE_TOO_LARGE": 1, "RELAY_REFUSED": 1})
    assert inconclusive_for_data(cell)


@pytest.mark.parametrize(("without", "expected"), [(1, False), (2, True)])
def test_units_without_votes_count_for_source_two(without: int, expected: bool) -> None:
    cell = make_cell(
        Kind.K3, [], source=SourceId.JH5, model="host", units=10 + without - 1, units_without_votes=without
    )
    assert inconclusive_for_data(cell) is expected


def test_exactly_a_tenth_of_votes_missing_is_not_inconclusive_for_data() -> None:
    cell = make_cell(Kind.K3, [], source=SourceId.VH2, model="host", units=10, units_without_votes=1)
    assert not inconclusive_for_data(cell)


def test_a_cell_without_units_has_no_missing_share() -> None:
    assert not inconclusive_for_data(make_cell(Kind.K3, [], units=0))


def test_a_protocol_entry_turns_met_into_inconclusive() -> None:
    assert admission_capped(AdmissionOutcome.MET, PROTOCOL_STANDING) == (AdmissionOutcome.INCONCLUSIVE, ("protocol",))


@pytest.mark.parametrize(
    "outcome", [AdmissionOutcome.NOT_MET, AdmissionOutcome.UNREACHABLE, AdmissionOutcome.INCONCLUSIVE]
)
def test_a_protocol_entry_leaves_the_other_admission_outcomes(outcome: AdmissionOutcome) -> None:
    assert admission_capped(outcome, PROTOCOL_STANDING) == (outcome, ())


def test_met_stands_without_a_protocol_entry() -> None:
    assert admission_capped(AdmissionOutcome.MET, NO_STANDING) == (AdmissionOutcome.MET, ())
