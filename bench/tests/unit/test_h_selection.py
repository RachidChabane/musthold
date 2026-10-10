from __future__ import annotations

import json
from pathlib import Path

import pytest

from specbench.domain.canonical import canonical_json
from specbench.domain.h_selection import (
    H_SELECTION_PATH,
    MAX_KEPT_PER_CRITERION,
    CandidateRuns,
    HExecution,
    HRound,
    HSelectionRecord,
    Verdict,
    exposed_criteria,
    rejection,
    require_selected_h,
    sealed_h_files,
    select_h,
    selection_rank,
    shortfall,
    stable_outcome,
    uncollected_files,
)
from specbench.domain.h_tests import HFile, HTest, TestKind
from specbench.domain.languages import Language
from specbench.domain.oracle import Outcome
from specbench.domain.protocol_disclosures import H_SELECTION, RUN_START_DISCLOSURES
from specbench.domain.sealing import SealingError
from support.builders import make_bundle, selection_record_for

P, F, M = Outcome.PASSED, Outcome.FAILED, Outcome.MISSING
GOOD = CandidateRuns((P, P, P), (F, F, F))
NONE: frozenset[str] = frozenset()
SEED = 20261007
TASK = "ACME-PARKFEE-12"


def candidate(
    name: str, number: int = 1, kind: TestKind = TestKind.EXAMPLE, path: str = "a.py", round_: int = 0
) -> HTest:
    return HTest(f"{path}::{name}", number, kind, path, round_)


@pytest.mark.parametrize(
    ("runs", "unbuildable", "judged", "expected"),
    [
        (GOOD, NONE, True, None),
        (GOOD, frozenset({"a.py"}), True, Verdict.REJECTED_FILE),
        (CandidateRuns((P, F, P), (F, F, F)), NONE, True, Verdict.REJECTED_FLAKY),
        (CandidateRuns((P, P, P), (F, P, F)), NONE, False, Verdict.REJECTED_FLAKY),
        (CandidateRuns((F, F, F), (F, F, F)), NONE, True, Verdict.REJECTED_MERGE),
        (CandidateRuns((M, M, M), (M, M, M)), NONE, True, Verdict.REJECTED_MERGE),
        (CandidateRuns((P, P, P), (P, P, P)), NONE, True, Verdict.REJECTED_BASE_PASS),
        (CandidateRuns((P, P, P), (P, P, P)), NONE, False, None),
        (CandidateRuns((P, P, P), (M, M, M)), NONE, True, None),
    ],
)
def test_each_rejection_class_applies_verifications_own_rule(
    runs: CandidateRuns, unbuildable: frozenset[str], judged: bool, expected: Verdict | None
) -> None:
    assert rejection(candidate("t"), runs, unbuildable, judged) is expected


def test_a_test_that_never_ran_is_missing_never_flaky() -> None:
    assert stable_outcome(()) is Outcome.MISSING
    assert stable_outcome((P, P, P)) is P and stable_outcome((P, F, P)) is Outcome.FLAKY


def test_property_tests_rank_first_then_the_seeded_hash() -> None:
    prop = candidate("p", kind=TestKind.PROPERTY)
    examples = [candidate(f"e{k}") for k in range(5)]
    ranked = sorted([*examples, prop], key=lambda t: selection_rank(SEED, TASK, t))
    assert ranked[0] == prop
    assert [t.test_id for t in ranked[1:]] == sorted(
        (t.test_id for t in examples), key=lambda i: selection_rank(SEED, TASK, candidate(i.split("::")[1]))[1]
    )
    assert selection_rank(SEED, TASK, examples[0]) != selection_rank(SEED + 1, TASK, examples[0])


def test_at_most_three_are_kept_per_criterion_and_the_rest_are_surplus() -> None:
    tests = [candidate(f"t{k}") for k in range(5)]
    selection = select_h(tests, HExecution({t.test_id: GOOD for t in tests}), (1,), frozenset({1}), SEED, TASK)
    counts = selection.verdict_counts(1)
    assert counts[Verdict.KEPT] == MAX_KEPT_PER_CRITERION and counts[Verdict.SURPLUS] == 2
    assert selection.deficient == ()


def test_a_criterion_with_fewer_than_two_survivors_is_deficient_and_its_shortfall_names_the_classes() -> None:
    tests = [candidate("a"), candidate("b"), candidate("c", 2), candidate("d", 2)]
    runs = {
        tests[0].test_id: GOOD,
        tests[1].test_id: CandidateRuns((F, F, F), (F, F, F)),
        tests[2].test_id: GOOD,
        tests[3].test_id: GOOD,
    }
    selection = select_h(tests, HExecution(runs), (1, 2), frozenset({1, 2}), SEED, TASK)
    assert selection.deficient == (1,)
    assert shortfall(selection) == "criterion 1 keeps 1 H tests, 2 required (rejected-merge 1)"


def test_a_criterion_with_no_candidate_at_all_is_deficient() -> None:
    selection = select_h([candidate("a"), candidate("b")], HExecution({}), (1, 2), frozenset(), SEED, TASK)
    assert selection.deficient == (1, 2)


def test_uncollected_files_are_those_none_of_whose_tests_any_run_reported() -> None:
    a, b, c = candidate("a", path="x.py"), candidate("b", path="x.py"), candidate("c", path="y.py")
    runs = {a.test_id: (M, M, M), b.test_id: (M, M, M), c.test_id: (M, F, M)}
    assert uncollected_files([a, b, c], runs) == frozenset({"x.py"})


def test_sealed_files_hold_only_kept_tests_python_pruned_and_go_whole() -> None:
    source = "def test_c1_a():\n    pass\n\n\ndef test_c1_b():\n    pass\n"
    python = HFile("tests/spec_h/test_spec_h_c1_x.py", source)
    dropped = HFile("tests/spec_h/test_spec_h_c2_x.py", "def test_c2_a():\n    pass\n")
    (sealed,) = sealed_h_files(Language.PYTHON, [python, dropped], frozenset({f"{python.path}::test_c1_b"}))
    assert sealed.content.lstrip() == "def test_c1_b():\n    pass\n"
    go = HFile(
        "fee/h_c1_x_test.go",
        "package fee_test\n\nfunc TestH1_A(t *testing.T) {\n}\n\nfunc TestH1_B(t *testing.T) {\n}\n",
    )
    assert sealed_h_files(Language.GO, [go], frozenset({"fee::TestH1_B"})) == (go,)


def record(rounds: tuple[HRound, ...] = (HRound(0, (1, 2), ("build-" + "0" * 20,)),)) -> HSelectionRecord:
    tests = [candidate("a"), candidate("b"), candidate("c", 2), candidate("d", 2), candidate("e", 2)]
    runs = {t.test_id: GOOD for t in tests} | {tests[4].test_id: CandidateRuns((F, F, F), (F, F, F))}
    execution = HExecution(runs)
    return HSelectionRecord(
        TASK, SEED, rounds, select_h(tests, execution, (1, 2), frozenset({1, 2}), SEED, TASK), execution
    )


def test_the_record_counts_merge_rejects_and_exposes_criteria_with_a_merge_reject_or_a_refill() -> None:
    plain = record()
    assert plain.exposed == frozenset({2}) and plain.divergences == ("a.py::e",)
    refilled = record((HRound(0, (1, 2), ("build-" + "0" * 20,)), HRound(1, (1,), ("build-" + "1" * 20,))))
    assert refilled.exposed == frozenset({1, 2}) and refilled.refilled == frozenset({1})
    criteria = refilled.to_record()["criteria"]
    assert criteria == [
        {"criterion": f"{TASK}-1", "exposed": True, "kept": 2, "merge_rejects": 0, "refilled": True},
        {"criterion": f"{TASK}-2", "exposed": True, "kept": 2, "merge_rejects": 1, "refilled": False},
    ]


def test_sealing_refuses_a_bundle_without_a_selection_record_or_whose_h_differs_from_it() -> None:
    require_selected_h(make_bundle())
    with pytest.raises(SealingError, match="not selected by execution"):
        require_selected_h(make_bundle({}, without=(H_SELECTION_PATH,)))
    other = selection_record_for(["tests/test_h.py::test_other"])
    with pytest.raises(SealingError, match="not the tests selected by execution"):
        require_selected_h(make_bundle({H_SELECTION_PATH: canonical_json(other).encode()}))


def test_the_run_start_disclosure_is_the_line_adr_0012_states() -> None:
    adr = (Path(__file__).resolve().parents[3] / "docs" / "adr" / "0012-selecting-h-by-execution.md").read_text()
    assert H_SELECTION in RUN_START_DISCLOSURES
    assert H_SELECTION.render().removeprefix("- ") in " ".join(adr.split())


def test_the_exposed_criteria_are_read_back_from_the_sealed_record() -> None:
    body = json.loads(canonical_json(record().to_record()))
    bundle = make_bundle({H_SELECTION_PATH: canonical_json(body).encode()})
    assert exposed_criteria(bundle) == frozenset({f"{TASK}-2"})
    assert exposed_criteria(make_bundle({}, without=(H_SELECTION_PATH,))) == frozenset()
