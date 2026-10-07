from __future__ import annotations

import math
from fractions import Fraction

import numpy as np
import pytest

from calibration.analysis.stats.admission import (
    AdmissionOutcome,
    admission_test,
    bootstrap_error_upper,
    calibrated_value,
    isotonic_fit,
    majority_rule_error_at_coverage,
    split_clusters,
)
from stats_arrays import flags, floats, mask


def _reference_isotonic(confidences: list[float], correct: list[int]) -> list[float]:
    distinct = sorted(set(confidences))
    totals = [sum(c for x, c in zip(confidences, correct, strict=True) if x == d) for d in distinct]
    weights = [sum(1 for x in confidences if x == d) for d in distinct]
    count = len(distinct)

    def average(first: int, last: int) -> Fraction:
        return Fraction(sum(totals[first : last + 1]), sum(weights[first : last + 1]))

    return [float(max(min(average(s, t) for t in range(i, count)) for s in range(i + 1))) for i in range(count)]


def test_pav_pools_adjacent_violators() -> None:
    fit = isotonic_fit(floats([0.1, 0.2, 0.3, 0.4]), flags([1, 0, 1, 1]))
    assert fit.confidences.tolist() == [0.1, 0.2, 0.3, 0.4]
    assert fit.values.tolist() == [0.5, 0.5, 1.0, 1.0]


def test_pav_matches_a_brute_force_reference() -> None:
    generator = np.random.default_rng(5)
    for _ in range(25):
        confidences = [float(v) for v in generator.integers(0, 8, size=20) / 8]
        correct = [int(v) for v in generator.integers(0, 2, size=20)]
        fit = isotonic_fit(floats(confidences), flags(correct))
        assert fit.values.tolist() == _reference_isotonic(confidences, correct)


def test_pav_of_nothing_is_empty() -> None:
    fit = isotonic_fit(floats([]), flags([]))
    assert fit.values.size == 0
    assert math.isnan(calibrated_value(fit, 0.5))


def test_calibrated_value_uses_the_largest_fit_confidence_at_or_below() -> None:
    fit = isotonic_fit(floats([0.2, 0.5, 0.8]), flags([0, 1, 1]))
    assert calibrated_value(fit, 0.5) == 1.0
    assert calibrated_value(fit, 0.49) == 0.0
    assert calibrated_value(fit, 0.99) == 1.0
    assert calibrated_value(fit, 0.01) == 0.0


def test_split_takes_the_first_ceil_half_of_the_hash_order() -> None:
    ids = ["alpha", "beta", "gamma", "delta", "epsilon"]
    split = split_clusters(ids)
    assert len(split.fit_half) == 3
    assert len(split.held_out) == 2
    assert sorted(split.fit_half + split.held_out) == sorted(ids)
    assert split_clusters(reversed(ids)) == split
    assert split_clusters(ids + ids) == split


def _dataset(clusters: int, per_cluster: int) -> list[str]:
    return [f"cluster-{c:02d}" for c in range(clusters) for _ in range(per_cluster)]


def test_a_confident_and_accurate_source_meets_the_test() -> None:
    ids = _dataset(40, 10)
    result = admission_test(ids, floats([0.99] * 400), flags([1] * 400), item_set_could_supply=True)
    assert result.outcome is AdmissionOutcome.MET
    assert (result.fit_decisions, result.fit_clusters) == (200, 20)
    assert (result.held_out_decisions, result.held_out_clusters) == (200, 20)
    assert (result.selected_decisions, result.selected_errors, result.coverage) == (200, 0, 1.0)


def test_a_source_that_is_wrong_when_confident_is_not_met() -> None:
    ids = _dataset(40, 10)
    correct = flags([i % 2 for i in range(400)])
    result = admission_test(ids, floats([0.99] * 400), correct, item_set_could_supply=True)
    assert result.outcome is AdmissionOutcome.NOT_MET
    assert result.selected_decisions == 0
    assert result.coverage == 0.0
    assert math.isnan(result.error_point)


def test_too_little_coverage_is_not_met() -> None:
    ids = _dataset(40, 10)
    confident = np.arange(400) % 5 == 0
    confidence = floats([0.99 if flag else 0.5 for flag in confident])
    correct = flags([1 if flag else 0 for flag in confident])
    result = admission_test(ids, confidence, correct, item_set_could_supply=True)
    assert result.outcome is AdmissionOutcome.NOT_MET
    assert result.coverage == pytest.approx(0.2)


def test_a_wilson_bound_above_the_limit_is_not_met_although_the_point_estimate_passes() -> None:
    ids = _dataset(40, 10)
    split = split_clusters(ids)
    fit_clusters = set(split.fit_half)
    confidence: list[float] = []
    correct: list[int] = []
    held_seen = 0
    for cluster in ids:
        if cluster in fit_clusters:
            confident = len(confidence) % 2 == 0
            confidence.append(0.99 if confident else 0.5)
            correct.append(1 if confident else 0)
            continue
        confident = held_seen < 60
        confidence.append(0.99 if confident else 0.5)
        correct.append(0 if held_seen < 3 or not confident else 1)
        held_seen += 1
    result = admission_test(ids, floats(confidence), flags(correct), item_set_could_supply=True)
    assert (result.selected_decisions, result.selected_errors) == (60, 3)
    assert result.error_point == 0.05
    assert result.coverage == 0.3
    assert result.error_upper > 0.10
    assert result.outcome is AdmissionOutcome.NOT_MET
    lenient = admission_test(
        ids, floats(confidence), flags(correct), item_set_could_supply=True, error_upper=lambda rows: 0.0
    )
    assert lenient.outcome is AdmissionOutcome.MET


@pytest.mark.parametrize(
    ("could_supply", "outcome"),
    [(False, AdmissionOutcome.UNREACHABLE), (True, AdmissionOutcome.INCONCLUSIVE)],
)
def test_unmet_preconditions_depend_on_whether_the_item_set_could_supply(
    could_supply: bool, outcome: AdmissionOutcome
) -> None:
    ids = _dataset(10, 5)
    result = admission_test(ids, floats([0.99] * 50), flags([1] * 50), item_set_could_supply=could_supply)
    assert result.outcome is outcome
    assert result.selected_decisions == 0
    assert math.isnan(result.coverage)


def test_fit_half_needs_ten_clusters_as_well_as_two_hundred_decisions() -> None:
    ids = _dataset(16, 40)
    result = admission_test(ids, floats([0.99] * 640), flags([1] * 640), item_set_could_supply=False)
    assert result.fit_decisions == 320
    assert result.fit_clusters == 8
    assert result.outcome is AdmissionOutcome.UNREACHABLE


def test_the_bootstrap_upper_bound_replaces_wilson() -> None:
    ids = _dataset(40, 10)
    confidence = floats([0.99] * 400)
    correct = flags([1] * 400)
    upper = bootstrap_error_upper(ids, correct, resamples=100)
    result = admission_test(ids, confidence, correct, item_set_could_supply=True, error_upper=upper)
    assert result.outcome is AdmissionOutcome.MET
    assert result.error_upper == 0.0


def test_a_caller_supplied_bound_above_the_limit_prevents_the_met_outcome() -> None:
    ids = _dataset(40, 10)
    result = admission_test(
        ids, floats([0.99] * 400), flags([1] * 400), item_set_could_supply=True, error_upper=lambda rows: 0.2
    )
    assert result.outcome is AdmissionOutcome.NOT_MET


def test_an_undefined_bound_is_inconclusive() -> None:
    ids = _dataset(40, 10)
    result = admission_test(
        ids, floats([0.99] * 400), flags([1] * 400), item_set_could_supply=True, error_upper=lambda rows: math.nan
    )
    assert result.outcome is AdmissionOutcome.INCONCLUSIVE


def test_a_row_mask_restricts_decisions_but_keeps_the_halves() -> None:
    ids = _dataset(80, 10)
    predicting = mask([i % 2 == 0 for i in range(800)])
    result = admission_test(
        ids, floats([0.99] * 800), flags([1] * 800), item_set_could_supply=True, row_mask=predicting
    )
    assert result.outcome is AdmissionOutcome.MET
    assert (result.fit_decisions, result.fit_clusters) == (200, 40)
    assert result.held_out_decisions == 200


def test_a_mask_that_starves_the_fit_half_fails_the_preconditions() -> None:
    ids = _dataset(80, 10)
    sparse = mask([i % 4 == 0 for i in range(800)])
    result = admission_test(ids, floats([0.99] * 800), flags([1] * 800), item_set_could_supply=False, row_mask=sparse)
    assert result.outcome is AdmissionOutcome.UNREACHABLE


def test_the_record_serialises_undefined_numbers_as_none() -> None:
    ids = _dataset(10, 5)
    record = admission_test(ids, floats([0.99] * 50), flags([1] * 50), item_set_could_supply=False).to_record()
    assert record["outcome"] == "UNREACHABLE"
    assert record["coverage"] is None
    assert record["fit_decisions"] == 25


def test_majority_rule_error_over_the_admitted_prefix() -> None:
    proportion = majority_rule_error_at_coverage(flags([1, 0, 1, 1, 0]), 0.6)
    assert (proportion.successes, proportion.total) == (1, 3)
    assert majority_rule_error_at_coverage(flags([1, 0]), 0.0).total == 0
    assert majority_rule_error_at_coverage(flags([1, 0]), 1.5).total == 2
