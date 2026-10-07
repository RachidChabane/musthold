"""Reading bands (§11): the first rule that applies gives the band, then the effects of the deviation log."""

from __future__ import annotations

import math
from dataclasses import dataclass
from enum import StrEnum
from typing import Final

from calibration.analysis.cell_metrics import CellMetrics
from calibration.analysis.model import CellData, Kind
from calibration.analysis.stats.admission import AdmissionOutcome
from calibration.domain.constants import (
    FALL_THROUGH_LIMIT,
    FLAG_POINT_LIMIT,
    MISS_UPPER_LIMIT,
    SCREEN_TAU_DEFAULT,
)
from calibration.domain.deviations import Standing

USELESS_AUROC_UPPER: Final = 0.60
READY_AUROC_LOWER: Final = 0.75
READY_ECE_POINT: Final = 0.05
READY_ECE_UPPER: Final = 0.10
ROUTE_AUROC_LOWER: Final = 0.55
SCREEN_TAU_TOLERANCE: Final = 1e-9


class Band(StrEnum):
    BLOCKED = "BLOCKED"
    INCONCLUSIVE_DATA = "INCONCLUSIVE (data)"
    USELESS = "USELESS"
    THRESHOLD_READY = "THRESHOLD_READY"
    ROUTE_ONLY = "ROUTE_ONLY"
    INCONCLUSIVE = "INCONCLUSIVE"


@dataclass(frozen=True)
class BandVerdict:
    band: Band
    reason: str
    capped_by: tuple[str, ...]


def inconclusive_for_data(cell: CellData) -> bool:
    """§8.4: more than 0.10 of the cell's units are fall-throughs (S1) or without any vote (source 2)."""
    return cell.missing_share > FALL_THROUGH_LIMIT


def _useless(cell: CellData, metrics: CellMetrics) -> bool:
    if metrics.auroc.hi < USELESS_AUROC_UPPER:
        return True
    return cell.kind is Kind.K1 and metrics.accuracy.hi < metrics.majority_accuracy


def _screen_ready(metrics: CellMetrics) -> bool:
    """For K4 a miss-rate upper bound of at most 0.10 and a flag-rate point of at most 0.50, both at tau = 0.20 on
    the screen stratum."""
    point = next(
        (p for p in metrics.screen_stratum if math.isclose(p.tau, SCREEN_TAU_DEFAULT, abs_tol=SCREEN_TAU_TOLERANCE)),
        None,
    )
    return point is not None and point.miss.hi <= MISS_UPPER_LIMIT and point.flag.point <= FLAG_POINT_LIMIT


def _ready(cell: CellData, metrics: CellMetrics) -> bool:
    calibrated = (
        metrics.auroc.lo >= READY_AUROC_LOWER
        and metrics.ece.point <= READY_ECE_POINT
        and metrics.ece.hi <= READY_ECE_UPPER
    )
    return calibrated and (cell.kind is not Kind.K4 or _screen_ready(metrics))


def _metric_band(cell: CellData, metrics: CellMetrics, standing: Standing) -> BandVerdict:
    if _useless(cell, metrics):
        return BandVerdict(Band.USELESS, "the AUROC upper bound is below 0.60", ())
    if _ready(cell, metrics):
        if standing.protocol:
            return BandVerdict(Band.INCONCLUSIVE, "THRESHOLD_READY capped by a protocol entry", ("protocol",))
        return BandVerdict(Band.THRESHOLD_READY, "the AUROC, ECE and screen conditions hold", ())
    if metrics.auroc.lo > ROUTE_AUROC_LOWER:
        return BandVerdict(Band.ROUTE_ONLY, "the AUROC lower bound exceeds 0.55", ())
    return BandVerdict(Band.INCONCLUSIVE, "no rule places the cell", ())


def band_of(cell: CellData, metrics: CellMetrics | None, blocked: str | None, standing: Standing) -> BandVerdict:
    """BLOCKED, INCONCLUSIVE (data), USELESS, THRESHOLD_READY, ROUTE_ONLY or INCONCLUSIVE, in that order; a `protocol`
    entry in scope turns THRESHOLD_READY into INCONCLUSIVE."""
    if blocked is not None or standing.blocked:
        return BandVerdict(Band.BLOCKED, blocked or "a blocked entry is in scope", ())
    if inconclusive_for_data(cell):
        return BandVerdict(Band.INCONCLUSIVE_DATA, "fall-throughs or missing votes exceed 0.10 of the units", ())
    if metrics is None:
        return BandVerdict(Band.INCONCLUSIVE, "no decision carries a prediction", ())
    return _metric_band(cell, metrics, standing)


def admission_capped(outcome: AdmissionOutcome, standing: Standing) -> tuple[AdmissionOutcome, tuple[str, ...]]:
    """A `protocol` entry in scope turns MET into INCONCLUSIVE."""
    if outcome is AdmissionOutcome.MET and standing.protocol:
        return AdmissionOutcome.INCONCLUSIVE, ("protocol",)
    return outcome, ()
