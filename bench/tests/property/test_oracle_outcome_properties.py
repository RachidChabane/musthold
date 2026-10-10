from __future__ import annotations

from hypothesis import given
from hypothesis import strategies as st

from specbench.domain.oracle import Outcome, folded_outcome, requested_outcomes

RUNNER_OUTCOMES = (Outcome.PASSED, Outcome.FAILED, Outcome.ERROR, Outcome.MISSING)
case_outcomes = st.lists(st.sampled_from(RUNNER_OUTCOMES), max_size=6)
case_labels = st.from_regex(r"[A-Za-z0-9_.-]{1,8}", fullmatch=True)


@given(case_outcomes)
def test_a_folded_test_fails_exactly_when_a_case_fails_or_none_ran(cases: list[Outcome]) -> None:
    folded = folded_outcome(cases)
    assert folded.is_failure == (not cases or any(c.is_failure for c in cases))
    assert folded in (*cases, Outcome.MISSING)


@given(st.dictionaries(case_labels, st.sampled_from(RUNNER_OUTCOMES), max_size=6))
def test_a_parameter_free_id_never_passes_while_one_of_its_cases_fails(cases: dict[str, Outcome]) -> None:
    test_id = "tests/spec_h/test_spec_h_rates.py::test_c2_rate_never_negative"
    reported = {f"{test_id}[{label}]": outcome for label, outcome in cases.items()}
    (outcome,) = requested_outcomes([test_id], reported).values()
    assert outcome is folded_outcome(list(cases.values()))
    assert all(requested_outcomes([case], reported)[case] is reported[case] for case in reported)
