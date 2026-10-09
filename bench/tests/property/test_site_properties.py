"""Properties of the site: the build does not depend on the order its inputs were read in, and the readings are total."""

from __future__ import annotations

import dataclasses
import random
from typing import cast

from hypothesis import given, settings
from hypothesis import strategies as st

from specbench.site.build import build_site
from specbench.site.figures import Comparison, FigureKind, Reading, format_interval, format_value, reading
from support.site import fixture_inputs, make_figure

INPUTS = fixture_inputs()
BASELINE = build_site(INPUTS)


def shuffled(value: object, rng: random.Random) -> object:
    """The same JSON value with the keys of every object in another order."""
    if isinstance(value, dict):
        items = list(cast(dict[str, object], value).items())
        rng.shuffle(items)
        return {key: shuffled(item, rng) for key, item in items}
    if isinstance(value, list):
        return [shuffled(item, rng) for item in cast(list[object], value)]
    return value


@settings(max_examples=12, deadline=None)
@given(st.integers(min_value=0, max_value=2**32 - 1))
def test_the_build_is_the_same_whatever_the_order_of_keys_and_cases(seed: int) -> None:
    rng = random.Random(seed)
    cases = list(INPUTS.cases)
    rng.shuffle(cases)
    reordered = dataclasses.replace(INPUTS, results=shuffled(INPUTS.results, rng), cases=tuple(cases))
    assert build_site(reordered) == BASELINE


finite = st.floats(min_value=-1e6, max_value=1e6, allow_nan=False, allow_infinity=False)


@given(finite, finite, st.sampled_from(list(FigureKind)))
def test_every_finite_interval_has_a_rendering(lo: float, hi: float, kind: FigureKind) -> None:
    low, high = min(lo, hi), max(lo, hi)
    text = format_interval(kind, low, high)
    assert text.startswith("[") and text.endswith("]")
    assert "nan" not in text.lower()
    assert format_value(kind, low)


@given(finite, finite, st.integers(min_value=1, max_value=500))
def test_no_detectable_difference_exactly_when_the_interval_holds_zero(lo: float, hi: float, n: int) -> None:
    low, high = min(lo, hi), max(lo, hi)
    fig = make_figure((low + high) / 2, low, high, n=n, comparison=Comparison.REDUCTION)
    holds_zero = low <= 0.0 <= high
    assert (reading(fig) is Reading.NO_DETECTABLE_DIFFERENCE) == holds_zero


@given(st.one_of(st.none(), finite), st.one_of(st.none(), finite), st.one_of(st.none(), finite))
def test_a_missing_bound_or_estimate_is_never_read_as_a_result(
    value: float | None, lo: float | None, hi: float | None
) -> None:
    fig = make_figure(value, lo, hi, comparison=Comparison.REDUCTION)
    if value is None or lo is None or hi is None:
        assert reading(fig) is Reading.UNDEFINED
