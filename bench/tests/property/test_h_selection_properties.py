from __future__ import annotations

from dataclasses import dataclass
from random import Random

from hypothesis import given
from hypothesis import strategies as st

from specbench.domain.h_selection import (
    MAX_KEPT_PER_CRITERION,
    MIN_KEPT_PER_CRITERION,
    CandidateRuns,
    HExecution,
    HSelection,
    Verdict,
    rejection,
    select_h,
)
from specbench.domain.h_tests import HTest, TestKind
from specbench.domain.oracle import Outcome
from specbench.domain.verification import VERIFICATION_RUNS

RUNNER_OUTCOMES = (Outcome.PASSED, Outcome.FAILED, Outcome.ERROR, Outcome.MISSING)
CRITERIA = (1, 2, 3)
TASK = "ACME-PARKFEE-12"
runs_of = st.lists(st.sampled_from(RUNNER_OUTCOMES), min_size=VERIFICATION_RUNS, max_size=VERIFICATION_RUNS).map(tuple)


@dataclass(frozen=True)
class Pool:
    tests: tuple[HTest, ...]
    execution: HExecution
    judged: frozenset[int]
    seed: int


@st.composite
def pools(draw: st.DrawFn) -> Pool:
    count = draw(st.integers(min_value=0, max_value=14))
    tests: list[HTest] = []
    runs: dict[str, CandidateRuns] = {}
    for index in range(count):
        number = draw(st.sampled_from(CRITERIA))
        path = f"tests/spec_h/test_spec_h_c{number}_{draw(st.sampled_from('ab'))}.py"
        test = HTest(f"{path}::test_c{number}_{index}", number, draw(st.sampled_from(TestKind)), path, 0)
        tests.append(test)
        runs[test.test_id] = CandidateRuns(draw(runs_of), draw(runs_of))
    paths = sorted({t.path for t in tests})
    unbuildable = frozenset(draw(st.lists(st.sampled_from(paths), unique=True)) if paths else [])
    judged = frozenset(draw(st.lists(st.sampled_from(CRITERIA), unique=True)))
    return Pool(tuple(tests), HExecution(runs, unbuildable), judged, draw(st.integers(min_value=0, max_value=2**31)))


def selected(pool: Pool, tests: tuple[HTest, ...] | list[HTest] | None = None) -> HSelection:
    return select_h(tests or pool.tests, pool.execution, CRITERIA, pool.judged, pool.seed, TASK)


@given(pools(), st.randoms(use_true_random=False))
def test_selection_is_deterministic_and_invariant_under_permutation(pool: Pool, random: Random) -> None:
    shuffled = list(pool.tests)
    random.shuffle(shuffled)
    assert selected(pool).verdicts == selected(pool).verdicts == selected(pool, shuffled).verdicts


@given(pools())
def test_kept_tests_pass_at_merge_in_every_run_and_meet_every_rule_of_verification(pool: Pool) -> None:
    selection = selected(pool)
    for test in pool.tests:
        if selection.verdicts[test.test_id] is Verdict.KEPT:
            runs = pool.execution.runs[test.test_id]
            assert runs.merge == (Outcome.PASSED,) * VERIFICATION_RUNS
            assert test.path not in pool.execution.unbuildable
            assert rejection(test, runs, pool.execution.unbuildable, test.number in pool.judged) is None
            if test.number in pool.judged:
                assert runs.at_base.is_failure


@given(pools())
def test_each_criterion_keeps_at_most_three_survivors_property_tests_first(pool: Pool) -> None:
    selection = selected(pool)
    for number in CRITERIA:
        own = [t for t in pool.tests if t.number == number]
        survivors = [t for t in own if selection.verdicts[t.test_id] in (Verdict.KEPT, Verdict.SURPLUS)]
        kept = [t for t in own if selection.verdicts[t.test_id] is Verdict.KEPT]
        assert len(kept) == min(MAX_KEPT_PER_CRITERION, len(survivors))
        surplus_properties = [
            t for t in survivors if t.kind is TestKind.PROPERTY and selection.verdicts[t.test_id] is Verdict.SURPLUS
        ]
        if surplus_properties:
            assert all(t.kind is TestKind.PROPERTY for t in kept)
        assert (number in selection.deficient) == (len(kept) < MIN_KEPT_PER_CRITERION)
