"""The conclusion templates of PREREG section 11, verbatim, for every band."""

from __future__ import annotations

from dataclasses import replace
from typing import Final

import pytest

from calibration.analysis.bands import Band
from calibration.analysis.cell_metrics import CellMetrics
from calibration.analysis.conclusions import conclusion, interval_text
from calibration.analysis.model import Kind
from calibration.analysis.stats.intervals import Estimate
from support.metric_cases import base_metrics

SOURCE: Final = "S1"
MODEL: Final = "M6, Kev 9B"
KIND: Final = "K3"
PREFIX: Final = "S1 (M6, Kev 9B) on K3:"
METHOD: Final = "test"
READY_TAIL: Final = (
    "A threshold on this confidence reads as an error rate on this fixture; this does not establish calibration on "
    "other repositories."
)
ROUTE_TAIL: Final = (
    "The confidence ranks decisions above chance but its values are not error rates; thresholds must be derived, "
    "not read."
)
USELESS_TAIL: Final = "No usable signal on this fixture."


def _metrics() -> CellMetrics:
    return replace(
        base_metrics(Kind.K3),
        n=38,
        clusters=10,
        auroc=Estimate(0.9, 0.8, 1.0, METHOD),
        ece=Estimate(0.04, 0.01, 0.09, METHOD),
        q09=Estimate(0.875, 0.5, 1.0, METHOD),
    )


def test_threshold_ready_names_auroc_ece_q09_and_the_size() -> None:
    expected = (
        f"{PREFIX} AUROC 0.900 [0.800, 1.000], ECE 0.040 [0.010, 0.090], Q0.9 0.875 [0.500, 1.000], "
        f"n = 38 decisions in 10 clusters. {READY_TAIL}"
    )
    assert conclusion(Band.THRESHOLD_READY, SOURCE, MODEL, KIND, _metrics()) == expected


def test_route_only_names_auroc_and_ece() -> None:
    expected = f"{PREFIX} AUROC 0.900 [0.800, 1.000], ECE 0.040 [0.010, 0.090]. {ROUTE_TAIL}"
    assert conclusion(Band.ROUTE_ONLY, SOURCE, MODEL, KIND, _metrics()) == expected


def test_useless_names_auroc_only() -> None:
    expected = f"{PREFIX} AUROC 0.900 [0.800, 1.000]. {USELESS_TAIL}"
    assert conclusion(Band.USELESS, SOURCE, MODEL, KIND, _metrics()) == expected


def test_inconclusive_names_what_it_could_not_place() -> None:
    expected = (
        f"{PREFIX} the data cannot place the result in a band (AUROC 0.900 [0.800, 1.000], ECE 0.040 [0.010, 0.090])."
    )
    assert conclusion(Band.INCONCLUSIVE, SOURCE, MODEL, KIND, _metrics()) == expected


def test_inconclusive_for_data_reads_like_inconclusive() -> None:
    assert conclusion(Band.INCONCLUSIVE_DATA, SOURCE, MODEL, KIND, _metrics()) == conclusion(
        Band.INCONCLUSIVE, SOURCE, MODEL, KIND, _metrics()
    )


def test_blocked_names_its_reason() -> None:
    expected = f"{PREFIX} not measured (memory-fit)."
    assert conclusion(Band.BLOCKED, SOURCE, MODEL, KIND, None, "memory-fit") == expected


@pytest.mark.parametrize("band", [Band.INCONCLUSIVE, Band.INCONCLUSIVE_DATA, Band.USELESS, Band.ROUTE_ONLY])
def test_a_band_without_figures_says_they_are_undefined(band: Band) -> None:
    expected = f"{PREFIX} the data cannot place the result in a band (AUROC undefined, ECE undefined)."
    assert conclusion(band, SOURCE, MODEL, KIND, None) == expected


def test_an_undefined_figure_is_written_undefined() -> None:
    undefined = Estimate(float("nan"), float("nan"), float("nan"), METHOD)
    assert interval_text(undefined) == "undefined [undefined, undefined]"


def test_figures_are_rounded_to_three_decimals() -> None:
    assert interval_text(Estimate(0.12345, 0.1, 0.98765, METHOD)) == "0.123 [0.100, 0.988]"
