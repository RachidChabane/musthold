"""The honesty rules of a figure: interval and sample always shown, null as null, zero-crossing as no difference."""

from __future__ import annotations

import pytest

from specbench.site.components import figure
from specbench.site.figures import (
    MIN_REPOSITORIES_FOR_NOMINAL_COVERAGE,
    Caution,
    Comparison,
    FigureKind,
    Reading,
    cautions,
    format_interval,
    format_value,
    reading,
    reading_text,
    too_few_repositories,
)
from support.site import make_figure

MINUS = "\u2212"
SPACE = "\u202f"


@pytest.mark.parametrize(
    ("kind", "value", "shown"),
    [
        (FigureKind.RATE, 0.6, f"60{SPACE}%"),
        (FigureKind.RATE, -0.5, f"{MINUS}50{SPACE}%"),
        (FigureKind.RATE, -0.004, f"0{SPACE}%"),
        (FigureKind.RATE, 1.0, f"100{SPACE}%"),
        (FigureKind.CALLS, 19.25, "19.2"),
        (FigureKind.CALLS, -3.0, f"{MINUS}3.0"),
        (FigureKind.MINUTES, 1500.0, f"25.0{SPACE}min"),
    ],
)
def test_values_are_shown_in_the_unit_of_their_kind(kind: FigureKind, value: float, shown: str) -> None:
    assert format_value(kind, value) == shown


def test_an_interval_is_its_two_bounds_in_brackets() -> None:
    assert format_interval(FigureKind.RATE, 0.166667, 1.0) == f"[17{SPACE}%, 100{SPACE}%]"


def test_a_null_estimate_reads_as_undefined_and_is_never_shown_as_a_number() -> None:
    fig = make_figure(None, None, None)
    assert reading(fig) is Reading.UNDEFINED
    html = figure(fig)
    assert "not estimable" in html
    assert 'data-estimate="null"' in html
    assert "%" not in html


def test_an_estimate_without_an_interval_is_not_estimable() -> None:
    fig = make_figure(0.5, None, None)
    assert reading(fig) is Reading.UNDEFINED
    assert "no interval" in figure(fig)


def test_a_figure_over_no_unit_names_what_is_missing() -> None:
    fig = make_figure(None, None, None, n=0)
    assert reading_text(fig, "tests only") == "not estimable: no trunk"


@pytest.mark.parametrize(
    ("lo", "hi", "expected"),
    [
        (-0.5, 1.0, Reading.NO_DETECTABLE_DIFFERENCE),
        (0.0, 0.76, Reading.NO_DETECTABLE_DIFFERENCE),
        (-0.2, 0.0, Reading.NO_DETECTABLE_DIFFERENCE),
        (0.1, 0.9, Reading.ABOVE_ZERO),
        (-0.9, -0.1, Reading.BELOW_ZERO),
    ],
)
def test_a_reduction_whose_interval_holds_zero_reads_no_detectable_difference(
    lo: float, hi: float, expected: Reading
) -> None:
    fig = make_figure(0.3, lo, hi, comparison=Comparison.REDUCTION)
    assert reading(fig) is expected


def test_reduction_readings_name_the_direction_of_defects() -> None:
    fewer = make_figure(0.5, 0.2, 0.8, comparison=Comparison.REDUCTION)
    more = make_figure(-0.5, -0.8, -0.2, comparison=Comparison.REDUCTION)
    none = make_figure(0.6, -0.5, 1.0, comparison=Comparison.REDUCTION)
    assert reading_text(fewer, "tests only") == "fewer defects than tests only"
    assert reading_text(more, "tests only") == "more defects than tests only"
    assert reading_text(none, "tests only") == "no detectable difference from tests only"


def test_the_reference_arm_reads_as_the_reference_never_as_a_result() -> None:
    fig = make_figure(0.0, 0.0, 0.0, comparison=Comparison.REDUCTION, reference=True)
    assert reading(fig) is Reading.REFERENCE
    assert cautions(fig) == ()
    html = figure(fig, "tests only (T)")
    assert "reference: tests only (T)" in html
    assert "no detectable difference" not in html


def test_a_single_valued_interval_warns_that_the_sample_produced_it() -> None:
    fig = make_figure(1.0, 1.0, 1.0, n=1)
    assert cautions(fig) == (Caution.DEGENERATE_INTERVAL,)
    assert "single-valued interval" in figure(fig)


def test_every_shown_figure_carries_its_interval_and_sample_size() -> None:
    html = figure(make_figure(0.6, 0.17, 1.0, n=5))
    assert f"[17{SPACE}%, 100{SPACE}%]" in html
    assert f"n{SPACE}={SPACE}5 trunks" in html
    assert f"n{SPACE}={SPACE}1 trunk<" in figure(make_figure(0.6, 0.17, 1.0, n=1))


def test_few_repositories_are_flagged_below_the_coverage_threshold() -> None:
    assert too_few_repositories(MIN_REPOSITORIES_FOR_NOMINAL_COVERAGE - 1)
    assert not too_few_repositories(MIN_REPOSITORIES_FOR_NOMINAL_COVERAGE)
