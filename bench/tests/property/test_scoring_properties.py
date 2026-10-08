from __future__ import annotations

from hypothesis import given
from hypothesis import strategies as st

from specbench.domain.oracle import Oracle, OracleTest, Outcome, Part
from specbench.domain.scoring import SCORING_RUNS, settle, settled_outcome, truth_of

TESTS = tuple(OracleTest(f"t{index}::{part.value}", part, "T-1") for index, part in enumerate(Part))
ORACLE = Oracle(TESTS, (), ())
TEST_IDS = [t.test_id for t in TESTS]
REPORTED = st.sampled_from([Outcome.PASSED, Outcome.FAILED, Outcome.ERROR, Outcome.MISSING])
RUNS = st.lists(st.fixed_dictionaries(dict.fromkeys(TEST_IDS, REPORTED)), min_size=SCORING_RUNS, max_size=SCORING_RUNS)


@given(RUNS, st.randoms(use_true_random=False))
def test_the_flaky_rule_does_not_depend_on_the_order_of_the_runs(
    runs: list[dict[str, Outcome]], random: object
) -> None:
    shuffled = list(runs)
    random.shuffle(shuffled)  # type: ignore[attr-defined]
    assert settle(shuffled, TEST_IDS) == settle(runs, TEST_IDS)


@given(
    RUNS, st.sampled_from(TEST_IDS), st.integers(0, SCORING_RUNS - 1), st.sampled_from([Outcome.FAILED, Outcome.ERROR])
)
def test_adding_a_failure_to_a_defect_keeps_it_a_defect(
    runs: list[dict[str, Outcome]], test_id: str, run_index: int, failure: Outcome
) -> None:
    before = truth_of(ORACLE, settle(runs, TEST_IDS))
    worse = [dict(run) for run in runs]
    worse[run_index][test_id] = failure
    after = truth_of(ORACLE, settle(worse, TEST_IDS))
    assert after.defective >= before.defective
    assert after.defective_full >= before.defective_full
    assert after.o_failed >= before.o_failed


@given(st.lists(REPORTED, min_size=SCORING_RUNS, max_size=SCORING_RUNS))
def test_a_test_is_settled_as_failing_exactly_when_two_runs_fail(runs: list[Outcome]) -> None:
    failures = sum(outcome.is_failure for outcome in runs)
    settled = settled_outcome(runs)
    assert settled.is_failure == (failures >= 2)
    assert (settled is Outcome.FLAKY) == (failures == 1)
    assert (settled is Outcome.PASSED) == (failures == 0)
