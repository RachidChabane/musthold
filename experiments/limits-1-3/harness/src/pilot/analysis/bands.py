"""Band rules of the decisional endpoints D1 to D11 (PREREG 10): one pure function each.

Every function returns exactly one of GO, NO-GO or INCONCLUSIVE. An empty or undefined
input is INCONCLUSIVE. NO-GO is tested first so the two bands can never both hold.
"""

from __future__ import annotations

import math
from dataclasses import dataclass
from enum import Enum

from pilot.domain import analysis_constants as k
from pilot.analysis.stats import Interval

PERCENT = 100


class Outcome(Enum):
    GO = k.OUTCOME_GO
    NO_GO = k.OUTCOME_NO_GO
    INCONCLUSIVE = k.OUTCOME_INCONCLUSIVE


@dataclass(frozen=True)
class Estimate:
    point: float
    interval: Interval

    @property
    def is_defined(self) -> bool:
        return not math.isnan(self.point) and self.interval.is_defined


def _undefined(*estimates: Estimate) -> bool:
    return any(not e.is_defined for e in estimates)


def _resolve(go: bool, no_go: bool) -> Outcome:
    if no_go:
        return Outcome.NO_GO
    return Outcome.GO if go else Outcome.INCONCLUSIVE


def full_set_false_pass(false_passes: int, uncovered_total: int) -> Outcome:
    """D1: 0 of the uncovered full sets passing is GO, any is NO-GO."""
    if uncovered_total <= 0:
        return Outcome.INCONCLUSIVE
    return _resolve(false_passes == 0, false_passes >= 1)


def full_set_false_block(false_blocks: int, adequate_total: int) -> Outcome:
    """D2: at most floor(0.15 n_A) is GO, at least ceil(0.5 n_A) is NO-GO."""
    if adequate_total <= 0:
        return Outcome.INCONCLUSIVE
    go_limit = adequate_total * k.D2_GO_FRACTION_PERCENT // PERCENT
    no_go_limit = -(-adequate_total * k.D2_NO_GO_FRACTION_PERCENT // PERCENT)
    return _resolve(false_blocks <= go_limit, false_blocks >= no_go_limit)


def lower_is_better_rate(
    estimate: Estimate, go_point: float, go_upper: float, no_go_lower: float
) -> Outcome:
    """D3, D4, D5: GO when point and upper bound are low, NO-GO when even the lower bound is high."""
    if _undefined(estimate):
        return Outcome.INCONCLUSIVE
    go = estimate.point <= go_point and estimate.interval.hi <= go_upper
    return _resolve(go, estimate.interval.lo > no_go_lower)


def valid_precision(precision: Estimate, base_rate: float) -> Outcome:
    """D6: point, lower bound and margin over the N7 base rate (waived when N7 is high)."""
    if _undefined(precision) or math.isnan(base_rate):
        return Outcome.INCONCLUSIVE
    margin_ok = base_rate >= k.D6_N7_WAIVER or precision.point - base_rate >= k.D6_GO_MARGIN_OVER_N7
    go = precision.point >= k.D6_GO_POINT and precision.interval.lo >= k.D6_GO_LOWER and margin_ok
    return _resolve(go, precision.interval.hi < k.D6_NO_GO_UPPER)


def planted_controls(
    regression_valid: int, regression_total: int, neutral_valid: int, neutral_total: int
) -> Outcome:
    """D7: planted r-items found VALID and planted n1-items kept out."""
    if regression_total <= 0 or neutral_total <= 0:
        return Outcome.INCONCLUSIVE
    go = regression_valid >= k.D7_GO_REGRESSION_MIN and neutral_valid <= k.D7_GO_NEUTRAL_MAX
    no_go = regression_valid <= k.D7_NO_GO_REGRESSION_MAX or neutral_valid >= k.D7_NO_GO_NEUTRAL_MIN
    return _resolve(go, no_go)


def criteria_with_valid_mutants(criteria_count: int, criteria_total: int) -> Outcome:
    """D8: criteria that have at least two VALID arm-T mutants."""
    if criteria_total <= 0:
        return Outcome.INCONCLUSIVE
    return _resolve(criteria_count >= k.D8_GO_CRITERIA, criteria_count <= k.D8_NO_GO_CRITERIA_MAX)


def bad_test_detection(recall: Estimate, precision: Estimate, always_misaligned_precision: float) -> Outcome:
    """D9: recall and precision of 'predicted bad' against the 24 gold bad tests."""
    if _undefined(recall, precision) or math.isnan(always_misaligned_precision):
        return Outcome.INCONCLUSIVE
    go = (
        min(recall.point, precision.point) >= k.D9_GO_POINT
        and min(recall.interval.lo, precision.interval.lo) >= k.D9_GO_LOWER
        and precision.point - always_misaligned_precision >= k.D9_GO_MARGIN_OVER_N2
    )
    no_go = min(recall.interval.hi, precision.interval.hi) < k.D9_NO_GO_UPPER
    return _resolve(go, no_go)


def matrix_balanced_accuracy(
    accuracy: Estimate, null_balanced_accuracy: float, mean_dc: float | None, dc_null_threshold: float
) -> Outcome:
    """D10: balanced accuracy over chance and mean DC above the N3 percentile."""
    if _undefined(accuracy) or math.isnan(null_balanced_accuracy):
        return Outcome.INCONCLUSIVE
    if mean_dc is None or math.isnan(dc_null_threshold):
        return Outcome.INCONCLUSIVE
    dc_above_null = mean_dc > dc_null_threshold
    go = (
        accuracy.point >= k.D10_GO_POINT
        and accuracy.interval.lo >= k.D10_GO_LOWER
        and accuracy.interval.lo - null_balanced_accuracy > 0
        and dc_above_null
    )
    return _resolve(go, accuracy.interval.hi < k.D10_NO_GO_UPPER or not dc_above_null)


def cross_check_kappa(kappa: Estimate) -> Outcome:
    """D11: Cohen's kappa between the judge and the matrix on determinate items."""
    if _undefined(kappa):
        return Outcome.INCONCLUSIVE
    go = kappa.point >= k.D11_GO_POINT and kappa.interval.lo >= k.D11_GO_LOWER
    return _resolve(go, kappa.interval.hi < k.D11_NO_GO_UPPER)
