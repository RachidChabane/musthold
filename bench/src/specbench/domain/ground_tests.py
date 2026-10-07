"""The human parts of the oracle, from suite runs: U (fail to pass) and R (pass to pass)."""

from __future__ import annotations

from collections.abc import Mapping
from dataclasses import dataclass
from typing import Final

from specbench.domain.changed_tests import without_parameters
from specbench.domain.oracle import Outcome

MIN_SUITE_PASS_PERCENT: Final = 80


@dataclass(frozen=True)
class GroundTests:
    """U: touched tests that pass at merge and fail at base with the merge's test files. R: the rest that pass at both."""

    u: tuple[str, ...]
    r: tuple[str, ...]
    merge_passed: int
    merge_total: int

    @property
    def merge_pass_percent(self) -> int:
        return 100 * self.merge_passed // self.merge_total if self.merge_total else 0


def partition_ground_tests(
    touched: frozenset[str], merge: Mapping[str, Outcome], base_with_tests: Mapping[str, Outcome]
) -> GroundTests:
    passing = sorted(test for test, outcome in merge.items() if outcome is Outcome.PASSED)
    fail_to_pass = [
        test
        for test in passing
        if without_parameters(test) in touched and base_with_tests.get(test, Outcome.MISSING).is_failure
    ]
    chosen = set(fail_to_pass)
    pass_to_pass = [test for test in passing if test not in chosen and base_with_tests.get(test) is Outcome.PASSED]
    return GroundTests(tuple(fail_to_pass), tuple(pass_to_pass), len(passing), len(merge))
