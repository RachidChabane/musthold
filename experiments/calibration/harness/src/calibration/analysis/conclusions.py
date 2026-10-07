"""The conclusion templates of §11, one per band."""

from __future__ import annotations

import math

from calibration.analysis.bands import Band
from calibration.analysis.cell_metrics import CellMetrics
from calibration.analysis.stats.intervals import Estimate

DECIMALS = 3

THRESHOLD_READY_TAIL = (
    "A threshold on this confidence reads as an error rate on this fixture; this does not establish calibration on "
    "other repositories."
)
ROUTE_ONLY_TAIL = (
    "The confidence ranks decisions above chance but its values are not error rates; thresholds must be derived, "
    "not read."
)
USELESS_TAIL = "No usable signal on this fixture."


def _number(value: float) -> str:
    return "undefined" if math.isnan(value) else f"{value:.{DECIMALS}f}"


def interval_text(estimate: Estimate) -> str:
    return f"{_number(estimate.point)} [{_number(estimate.lo)}, {_number(estimate.hi)}]"


def conclusion(band: Band, source: str, model: str, kind: str, metrics: CellMetrics | None, reason: str = "") -> str:
    """The sentence of the band, filled from the cell's figures; a band without figures names the reason."""
    prefix = f"{source} ({model}) on {kind}:"
    if band is Band.BLOCKED:
        return f"{prefix} not measured ({reason})."
    if metrics is None:
        return f"{prefix} the data cannot place the result in a band (AUROC undefined, ECE undefined)."
    auroc, ece = interval_text(metrics.auroc), interval_text(metrics.ece)
    if band is Band.THRESHOLD_READY:
        clusters = f"{metrics.n} decisions in {metrics.clusters} clusters"
        return (
            f"{prefix} AUROC {auroc}, ECE {ece}, Q0.9 {interval_text(metrics.q09)}, n = {clusters}. "
            f"{THRESHOLD_READY_TAIL}"
        )
    if band is Band.ROUTE_ONLY:
        return f"{prefix} AUROC {auroc}, ECE {ece}. {ROUTE_ONLY_TAIL}"
    if band is Band.USELESS:
        return f"{prefix} AUROC {auroc}. {USELESS_TAIL}"
    return f"{prefix} the data cannot place the result in a band (AUROC {auroc}, ECE {ece})."
