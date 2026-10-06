"""Band rules, caps and the composite verdict (PREREG 10)."""

from __future__ import annotations

import math

import pytest
from hypothesis import given, strategies as st

from pilot.analysis import bands
from pilot.analysis.bands import Estimate, Outcome
from pilot.analysis.composite import apply_preconditions, composite_verdict
from pilot.analysis.preconditions import Precondition, Status
from pilot.analysis.results import EndpointResult, search_n_needed
from pilot.analysis.stats import Interval

unit = st.floats(0.0, 1.0)
counts = st.integers(0, 40)


def estimate(point, lo, hi):
    low, high = sorted((lo, hi))
    return Estimate(min(max(point, low), high), Interval(low, high))


intervals = st.tuples(unit, unit, unit).map(lambda t: estimate(*t))


def shrink(e: Estimate, fraction: float) -> Estimate:
    return Estimate(e.point, Interval(e.interval.lo + (e.point - e.interval.lo) * fraction,
                                      e.interval.hi - (e.interval.hi - e.point) * fraction))


# ---------------------------------------------------------------- worked band examples


def test_d1_zero_of_the_uncovered_full_sets_passing_is_go_and_any_pass_is_no_go():
    assert bands.full_set_false_pass(0, 3) is Outcome.GO
    assert bands.full_set_false_pass(1, 3) is Outcome.NO_GO
    assert bands.full_set_false_pass(0, 0) is Outcome.INCONCLUSIVE


@pytest.mark.parametrize(
    ("blocks", "adequate", "outcome"),
    [(0, 7, Outcome.GO), (1, 7, Outcome.GO), (2, 7, Outcome.INCONCLUSIVE), (3, 7, Outcome.INCONCLUSIVE),
     (4, 7, Outcome.NO_GO), (0, 0, Outcome.INCONCLUSIVE), (1, 3, Outcome.INCONCLUSIVE), (2, 3, Outcome.NO_GO),
     (1, 10, Outcome.GO), (2, 10, Outcome.INCONCLUSIVE), (5, 10, Outcome.NO_GO)],
)
def test_d2_uses_floor_015_and_ceil_05_of_the_adequate_full_sets(blocks, adequate, outcome):
    assert bands.full_set_false_block(blocks, adequate) is outcome


def test_d3_needs_both_a_low_point_and_a_low_upper_bound_to_go():
    go = bands.lower_is_better_rate(Estimate(0.04, Interval(0.0, 0.2)), 0.05, 0.20, 0.20)
    wide = bands.lower_is_better_rate(Estimate(0.04, Interval(0.0, 0.21)), 0.05, 0.20, 0.20)
    nogo = bands.lower_is_better_rate(Estimate(0.4, Interval(0.2001, 0.6)), 0.05, 0.20, 0.20)
    assert (go, wide, nogo) == (Outcome.GO, Outcome.INCONCLUSIVE, Outcome.NO_GO)


def test_d6_margin_over_n7_is_waived_when_n7_is_at_least_090():
    precision = Estimate(0.85, Interval(0.72, 0.95))
    assert bands.valid_precision(precision, 0.80) is Outcome.INCONCLUSIVE
    assert bands.valid_precision(precision, 0.74) is Outcome.GO
    assert bands.valid_precision(precision, 0.90) is Outcome.GO
    assert bands.valid_precision(Estimate(0.5, Interval(0.3, 0.69)), 0.2) is Outcome.NO_GO


@pytest.mark.parametrize(
    ("r", "n1", "outcome"),
    [(16, 1, Outcome.GO), (16, 2, Outcome.INCONCLUSIVE), (15, 0, Outcome.INCONCLUSIVE), (10, 0, Outcome.NO_GO),
     (20, 3, Outcome.NO_GO), (11, 2, Outcome.INCONCLUSIVE)],
)
def test_d7_planted_controls(r, n1, outcome):
    assert bands.planted_controls(r, 20, n1, 10) is outcome


@pytest.mark.parametrize(("count", "outcome"), [(10, Outcome.GO), (9, Outcome.INCONCLUSIVE), (8, Outcome.INCONCLUSIVE), (7, Outcome.NO_GO)])
def test_d8_criteria_with_two_valid_mutants(count, outcome):
    assert bands.criteria_with_valid_mutants(count, 10) is outcome


def test_d9_requires_recall_and_precision_and_a_margin_over_the_always_misaligned_judge():
    good = Estimate(0.9, Interval(0.7, 0.97))
    assert bands.bad_test_detection(good, good, 0.632) is Outcome.GO
    assert bands.bad_test_detection(good, good, 0.85) is Outcome.INCONCLUSIVE
    weak = Estimate(0.5, Interval(0.3, 0.69))
    assert bands.bad_test_detection(good, weak, 0.632) is Outcome.NO_GO


def test_d10_mean_dc_not_above_the_null_is_no_go_even_with_high_accuracy():
    accuracy = Estimate(0.9, Interval(0.75, 0.97))
    assert bands.matrix_balanced_accuracy(accuracy, 0.5, 0.4, 0.3) is Outcome.GO
    assert bands.matrix_balanced_accuracy(accuracy, 0.5, 0.2, 0.3) is Outcome.NO_GO
    assert bands.matrix_balanced_accuracy(accuracy, 0.8, 0.4, 0.3) is Outcome.INCONCLUSIVE
    assert bands.matrix_balanced_accuracy(accuracy, 0.5, None, 0.3) is Outcome.INCONCLUSIVE


def test_d11_kappa_bands():
    assert bands.cross_check_kappa(Estimate(0.7, Interval(0.4, 0.9))) is Outcome.GO
    assert bands.cross_check_kappa(Estimate(0.7, Interval(0.2, 0.9))) is Outcome.INCONCLUSIVE
    assert bands.cross_check_kappa(Estimate(0.2, Interval(-0.1, 0.39))) is Outcome.NO_GO
    assert bands.cross_check_kappa(Estimate(math.nan, Interval(math.nan, math.nan))) is Outcome.INCONCLUSIVE


# ---------------------------------------------------------------- properties


@given(intervals)
def test_rate_bands_return_exactly_one_outcome_and_are_monotone_in_the_interval(e):
    for args in ((0.05, 0.20, 0.20), (0.10, 0.25, 0.25), (0.30, 0.45, 0.50)):
        outcome = bands.lower_is_better_rate(e, *args)
        assert outcome in tuple(Outcome)
        narrower = bands.lower_is_better_rate(shrink(e, 0.5), *args)
        if outcome is not Outcome.INCONCLUSIVE:
            assert narrower is outcome


@given(intervals, unit)
def test_valid_precision_is_one_outcome_and_monotone(e, base):
    outcome = bands.valid_precision(e, base)
    if outcome is not Outcome.INCONCLUSIVE:
        assert bands.valid_precision(shrink(e, 0.5), base) is outcome


@given(intervals, intervals, unit)
def test_judge_and_kappa_bands_are_monotone(recall, precision, n2):
    outcome = bands.bad_test_detection(recall, precision, n2)
    if outcome is not Outcome.INCONCLUSIVE:
        assert bands.bad_test_detection(shrink(recall, 0.5), shrink(precision, 0.5), n2) is outcome
    kappa = bands.cross_check_kappa(recall)
    if kappa is not Outcome.INCONCLUSIVE:
        assert bands.cross_check_kappa(shrink(recall, 0.5)) is kappa


@given(counts, counts)
def test_count_bands_never_hold_both_and_empty_denominators_are_inconclusive(events, total):
    assert bands.full_set_false_block(events, 0) is Outcome.INCONCLUSIVE
    assert bands.full_set_false_pass(events, 0) is Outcome.INCONCLUSIVE
    assert bands.criteria_with_valid_mutants(events, 0) is Outcome.INCONCLUSIVE
    assert bands.full_set_false_block(min(events, total), total) in tuple(Outcome)
    assert bands.planted_controls(events, 0, 0, 10) is Outcome.INCONCLUSIVE


@given(unit, unit)
def test_undefined_estimates_are_inconclusive(base, accuracy):
    nan = Estimate(math.nan, Interval(math.nan, math.nan))
    assert bands.lower_is_better_rate(nan, 0.05, 0.2, 0.2) is Outcome.INCONCLUSIVE
    assert bands.valid_precision(nan, base) is Outcome.INCONCLUSIVE
    assert bands.matrix_balanced_accuracy(nan, accuracy, 0.1, 0.0) is Outcome.INCONCLUSIVE


# ---------------------------------------------------------------- composite


def endpoint(eid, outcome, attribution=None, outcome_at=None, n=10):
    return EndpointResult(
        id=eid, limit=1, measure="m", n=n, point=0.5, lo=0.4, hi=0.6, band_go="g", band_nogo="n",
        outcome=outcome, uncapped_outcome=outcome, attribution=attribution, outcome_at=outcome_at,
    )


def precondition(pid, status, affects, forces=False):
    return Precondition(pid, "d", status, "x", tuple(affects), forces)


def test_any_no_go_makes_the_composite_a_redesign_naming_the_components():
    results = [endpoint("D3", Outcome.NO_GO, {"component": "alignment judge"}), endpoint("D6", Outcome.NO_GO),
               endpoint("D10", Outcome.NO_GO), endpoint("D11", Outcome.NO_GO), endpoint("D9", Outcome.GO)]
    composite = composite_verdict(results, [])
    assert composite.verdict == "REDESIGN"
    assert composite.components == ("alignment judge", "mutant validity", "matrix", "cross-check")
    assert composite.text == "REDESIGN(alignment judge, mutant validity, matrix, cross-check)"


def test_a_gate_no_go_without_a_removing_substitution_falls_back_to_the_gate():
    result = composite_verdict([endpoint("D4", Outcome.NO_GO, {"component": "gate"})], [])
    assert result.components == ("gate",)
    assert composite_verdict([endpoint("D2", Outcome.NO_GO)], []).components == ("gate",)


def test_every_endpoint_go_with_no_failed_precondition_proceeds():
    results = [endpoint(f"D{i}", Outcome.GO) for i in range(1, 12)]
    assert composite_verdict(results, [precondition("P1", Status.PASS, [])]).verdict == "PROCEED_TO_POWERED_STUDY"


def test_all_go_with_a_failed_precondition_is_inconclusive():
    results = [endpoint("D1", Outcome.GO)]
    verdict = composite_verdict(results, [precondition("P6", Status.FAIL, [])])
    assert verdict.verdict == "INCONCLUSIVE"


def test_inconclusive_lists_open_endpoints_with_n_needed():
    decided_at_30 = lambda n: Outcome.GO if n >= 30 else Outcome.INCONCLUSIVE
    results = [endpoint("D3", Outcome.INCONCLUSIVE, outcome_at=decided_at_30), endpoint("D9", Outcome.INCONCLUSIVE),
               endpoint("D1", Outcome.GO)]
    composite = composite_verdict(results, [])
    assert composite.endpoints == ("D3", "D9")
    assert composite.n_needed == {"D3": 30, "D9": "unreachable"}
    assert composite.text == "INCONCLUSIVE(D3, D9; n_needed {D3: 30, D9: unreachable})"


def test_a_capped_endpoint_reports_blocked_instead_of_a_sample_size():
    capped = apply_preconditions([endpoint("D9", Outcome.GO)], [precondition("P3", Status.FAIL, ["D9"])])
    assert composite_verdict(capped, []).n_needed == {"D9": "blocked"}


def test_a_failed_precondition_caps_go_but_keeps_no_go_unless_it_invalidates_the_run():
    results = [endpoint("D9", Outcome.GO), endpoint("D11", Outcome.NO_GO), endpoint("D1", Outcome.GO)]
    capped = {r.id: r for r in apply_preconditions(results, [precondition("P2", Status.FAIL, ["D9", "D11"])])}
    assert capped["D9"].outcome is Outcome.INCONCLUSIVE and capped["D9"].capped_by == ("P2",)
    assert capped["D11"].outcome is Outcome.NO_GO
    assert capped["D1"].capped_by == ()
    forced = {r.id: r for r in apply_preconditions(results, [precondition("P7", Status.FAIL, ["D11"], forces=True)])}
    assert forced["D11"].outcome is Outcome.INCONCLUSIVE
    assert forced["D9"].outcome is Outcome.GO


def test_an_unevaluable_precondition_caps_like_a_failed_one():
    capped = apply_preconditions([endpoint("D9", Outcome.GO)], [precondition("P2", Status.UNEVALUABLE, ["D9"])])
    assert capped[0].outcome is Outcome.INCONCLUSIVE


@given(st.lists(st.sampled_from(list(Outcome)), min_size=1, max_size=11), st.booleans())
def test_composite_is_a_pure_function_of_endpoints_and_preconditions(outcomes, failing):
    results = [endpoint(f"D{i + 1}", o) for i, o in enumerate(outcomes)]
    preconditions = [precondition("P1", Status.FAIL if failing else Status.PASS, ["D1"])]
    first = composite_verdict(results, preconditions)
    assert composite_verdict(results, preconditions) == first
    if Outcome.NO_GO in outcomes:
        assert first.verdict == "REDESIGN"
    elif all(o is Outcome.GO for o in outcomes) and not failing:
        assert first.verdict == "PROCEED_TO_POWERED_STUDY"
    else:
        assert first.verdict == "INCONCLUSIVE"


def test_search_n_needed_returns_the_first_deciding_sample_size():
    assert search_n_needed(10, lambda n: Outcome.GO if n >= 57 else Outcome.INCONCLUSIVE) == 57
    assert search_n_needed(10, lambda n: Outcome.INCONCLUSIVE) == "unreachable"
    assert search_n_needed(10, None) == "unreachable"
