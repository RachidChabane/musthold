"""Blind-spot estimand: escape rule, common set, contrasts, bootstrap, reading, location split."""

from __future__ import annotations

import math

import numpy as np
import pytest
from hypothesis import given, settings, strategies as st

from pilot.analysis.blind_spot import (
    CELL_ORDER,
    CellCount,
    Escape,
    escape_outcome,
    common_set,
    contrasts,
    estimate,
    location_split,
    reading,
)
from pilot.analysis.settings import AnalysisSettings
from pilot.analysis.stats import Interval
from pilot.domain.model_slate import MutantSource, Writer

S, O, OP = MutantSource.SONNET, MutantSource.OPUS, MutantSource.OPERATOR
WS, WO = Writer.SONNET, Writer.OPUS
FAST = AnalysisSettings(resamples=500, permutations=1, seed=20261006)


def grid(criteria, count=CellCount(2, 2, 0)):
    return {(w, s, c): count for w, s in CELL_ORDER for c in criteria}


def test_escape_needs_every_test_survived_or_flaky():
    assert escape_outcome(["survived", "flaky"]) is Escape.ESCAPED
    assert escape_outcome(["survived", "killed"]) is Escape.CAUGHT
    assert escape_outcome(["error", "killed"]) is Escape.CAUGHT
    assert escape_outcome(["error", "survived"]) is Escape.EXCLUDED
    assert escape_outcome(["survived", None]) is Escape.EXCLUDED


def test_rates_count_only_evaluable_pairs():
    assert CellCount(1, 3, 5).rate == 0.25 and CellCount(0, 0, 4).evaluable == 0 and math.isnan(CellCount(0, 0, 4).rate)


def test_common_set_excludes_empty_suites_empty_universes_and_cells_without_an_evaluable_pair():
    cells = grid([1, 2, 3, 4])
    for s in (S, O, OP):
        cells[(WS, s, 1)] = None
        cells[(WO, s, 1)] = None
    cells[(WS, OP, 2)] = CellCount(0, 0, 0)
    cells[(WO, OP, 2)] = CellCount(0, 0, 0)
    cells[(WS, S, 3)] = CellCount(0, 0, 3)
    c_star, exclusions = common_set(cells, [1, 2, 3, 4])
    assert c_star == (4,)
    reasons = {e.criterion: e.reasons for e in exclusions}
    assert any("T_S is empty" in r for r in reasons[1]) and any("U(op) is empty" in r for r in reasons[2])
    assert reasons[3] == ("every pair of (S, S) is excluded",)


def e_of(values):
    return dict(zip(CELL_ORDER, values, strict=True))


def test_contrasts_follow_the_definitions():
    e = e_of([0.5, 0.2, 0.1, 0.3, 0.6, 0.2])  # SS SO Sop OS OO Oop
    c = contrasts(e)
    assert c.delta_s == pytest.approx(0.4) and c.delta_o == pytest.approx(0.4)
    assert c.sb_s == pytest.approx(0.4 - (0.3 - 0.2)) and c.sb_o == pytest.approx(0.4 - (0.2 - 0.1))
    assert c.sb == pytest.approx(0.5 * (0.5 + 0.6 - 0.2 - 0.3))


@given(st.lists(st.floats(0, 1), min_size=6, max_size=6))
def test_sb_is_half_the_sum_of_the_two_per_writer_values_and_operator_terms_cancel(values):
    c = contrasts(e_of(values))
    assert c.sb == pytest.approx(0.5 * (c.sb_s + c.sb_o))
    assert c.sb == pytest.approx(0.5 * (values[0] + values[4] - values[1] - values[3]))


def test_a_writer_that_never_catches_anything_has_no_blind_spot_difference():
    result = estimate(grid(range(1, 6), CellCount(4, 0, 0)), range(1, 6), FAST)
    assert result.contrasts.sb == 0 and result.interval.lo == result.interval.hi == 0


def test_the_bootstrap_is_deterministic_for_a_seed_and_clusters_by_criterion():
    cells = {}
    rng = np.random.default_rng(1)
    for c in range(1, 8):
        for cell in CELL_ORDER:
            cells[(*cell, c)] = CellCount(int(rng.integers(0, 6)), int(rng.integers(1, 6)), 0)
    a, b = estimate(cells, range(1, 8), FAST), estimate(cells, range(1, 8), FAST)
    assert a.interval == b.interval and a.reading == b.reading
    assert a.interval.lo <= a.contrasts.sb <= a.interval.hi


def test_an_empty_common_set_is_undetermined():
    result = estimate({}, [1, 2], FAST)
    assert result.c_star == () and result.reading == "undetermined" and result.contrasts is None


@pytest.mark.parametrize(
    ("lo", "hi", "expected"),
    [(0.01, 0.30, "shared blind spot indicated"), (-0.2, 0.05, "no material shared blind spot"),
     (-0.2, 0.30, "undetermined"), (0.02, 0.08, "shared blind spot indicated"), (-0.1, 0.10, "undetermined"),
     (float("nan"), float("nan"), "undetermined")],
)
def test_the_three_way_reading_with_the_first_rule_winning(lo, hi, expected):
    assert reading(Interval(lo, hi)) == expected


def test_the_upper_bound_below_materiality_is_flagged_beside_an_indicated_reading():
    cells = grid(range(1, 5))
    cells[(WS, S, 1)] = CellCount(2, 0, 0)  # make SS slightly higher in every criterion below
    for c in range(1, 5):
        cells[(WS, S, c)] = CellCount(3, 2, 0)
    result = estimate(cells, range(1, 5), FAST)
    assert result.reading == "shared blind spot indicated" and result.below_materiality in (True, False)


def test_location_split_separates_coverage_holes_from_model_specific_blind_spots():
    hosts = {"m1": ("f::a",), "m2": ("f::b",), "m3": ("f::a", "f::c"), "o1": ("f::a",), "o2": ("f::z",)}
    split = location_split(WS, S, {1: ["m1", "m2"], 2: ["m3"]}, {1: ["o1"], 2: ["o2"]}, lambda m: hosts[m], [1, 2])
    assert (split.escaping, split.coverage_hole, split.model_specific) == (3, 1, 2)


def test_a_model_mutant_is_compared_only_with_operator_mutants_of_its_own_criterion():
    hosts = {"m": ("f::a",), "o": ("f::a",)}
    split = location_split(WS, S, {1: ["m"]}, {2: ["o"]}, lambda m: hosts[m], [1, 2])
    assert split.coverage_hole == 0
