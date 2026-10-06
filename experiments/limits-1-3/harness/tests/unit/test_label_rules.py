from __future__ import annotations

from pilot.domain.constants import REFERENCE_VARIANT_ID
from pilot.domain.label_rules import ExpectedCounts, evaluate
from pilot.domain.labels import TestLabel
from pilot.domain.outcome_matrix import VariantOutcomes
from pilot.domain.outcomes import TestOutcome

PASS, FAIL = TestOutcome.PASSED, TestOutcome.FAILED
COUNTS = ExpectedCounts(tests=1, violations=4, planted=0, criteria=2)
NODE = "tests/t.py::t"


def make_label(label: str, cites: str = "PARK-1~1") -> TestLabel:
    return TestLabel.from_mapping({"nodeid": NODE, "cites": cites, "label": label, "checks": ""})


def oracle(failing: set[int]) -> dict[str, TestOutcome]:
    return {f"tests/test_oracle_park_{c}.py::o": FAIL if c in failing else PASS for c in (1, 2)}


def runs(test_by_variant: dict[str, TestOutcome]) -> dict[str, VariantOutcomes]:
    """Each violation fails exactly its own oracle; R passes every oracle."""
    table = {REFERENCE_VARIANT_ID: VariantOutcomes({NODE: PASS, **oracle(set())})}
    for variant in ("PARK-1_a", "PARK-1_b", "PARK-2_a", "PARK-2_b"):
        target = int(variant[5])
        table[variant] = VariantOutcomes(
            {NODE: test_by_variant.get(variant, PASS), **oracle({target})}
        )
    table["R"] = VariantOutcomes({NODE: test_by_variant.get("R", PASS), **oracle(set())})
    return table


def rules_failed(label: str, test_by_variant: dict[str, TestOutcome], cites="PARK-1~1") -> set[str]:
    failures = evaluate([make_label(label, cites)], runs(test_by_variant), COUNTS)
    return {f.rule for f in failures}


def test_a_consistent_good_test_raises_no_failure():
    assert rules_failed("GOOD", {"PARK-1_a": FAIL, "PARK-1_b": FAIL}) == set()


def test_good_must_fail_under_every_own_violation():
    assert rules_failed("GOOD", {"PARK-1_a": FAIL}) == {"4"}


def test_good_must_not_fail_under_another_criterion():
    assert rules_failed("GOOD", {"PARK-1_a": FAIL, "PARK-1_b": FAIL, "PARK-2_a": FAIL}) == {"4"}


def test_good_must_pass_under_the_refactor():
    assert rules_failed("GOOD", {"PARK-1_a": FAIL, "PARK-1_b": FAIL, "R": FAIL}) == {"4"}


def test_good_broad_needs_own_kills_and_a_second_criterion_kill():
    ok = {"PARK-1_a": FAIL, "PARK-1_b": FAIL, "PARK-2_b": FAIL}
    assert rules_failed("GOOD_BROAD(PARK-1,PARK-2)", ok) == set()
    no_second = {"PARK-1_a": FAIL, "PARK-1_b": FAIL}
    assert rules_failed("GOOD_BROAD(PARK-1,PARK-2)", no_second) == {"5"}


def test_weak_fails_some_but_not_all_own_violations():
    assert rules_failed("WEAK", {"PARK-1_a": FAIL}) == set()
    assert rules_failed("WEAK", {}) == {"6"}
    assert rules_failed("WEAK", {"PARK-1_a": FAIL, "PARK-1_b": FAIL}) == {"6"}


def test_mis_other_passes_own_and_fails_the_other_criterion():
    assert rules_failed("MIS_OTHER(PARK-1->PARK-2)", {"PARK-2_a": FAIL}) == set()
    assert rules_failed("MIS_OTHER(PARK-1->PARK-2)", {"PARK-1_a": FAIL, "PARK-2_a": FAIL}) == {"7"}
    assert rules_failed("MIS_OTHER(PARK-1->PARK-2)", {}) == {"7"}


def test_vacuous_passes_everything():
    assert rules_failed("VACUOUS", {}) == set()
    assert rules_failed("VACUOUS", {"PARK-2_b": FAIL}) == {"8"}
    assert rules_failed("VACUOUS", {"R": FAIL}) == {"8"}


def test_offspec_passes_violations_and_fails_the_refactor():
    assert rules_failed("OFFSPEC", {"R": FAIL}) == set()
    assert rules_failed("OFFSPEC", {}) == {"9"}
    assert rules_failed("OFFSPEC", {"R": FAIL, "PARK-1_a": FAIL}) == {"9"}


def test_oracle_matrix_must_be_diagonal():
    table = runs({})
    table["PARK-1_a"] = VariantOutcomes({NODE: PASS, **oracle(set())})
    failures = evaluate([make_label("VACUOUS")], table, COUNTS)
    assert [f.rule for f in failures] == ["2"]
    assert "oracle 1 passes on PARK-1_a" in failures[0].message


def test_reference_failing_a_test_or_an_oracle_is_reported():
    table = runs({})
    table[REFERENCE_VARIANT_ID] = VariantOutcomes({NODE: FAIL, **oracle({2})})
    messages = [str(f) for f in evaluate([make_label("VACUOUS")], table, COUNTS)]
    assert "[1] reference fails tests/t.py::t" in messages
    assert "[1] reference fails oracle 2" in messages


def test_refactor_failing_an_oracle_is_reported():
    table = runs({})
    table["R"] = VariantOutcomes({NODE: PASS, **oracle({1})})
    assert {f.rule for f in evaluate([make_label("VACUOUS")], table, COUNTS)} == {"1"}


def test_planted_item_failing_an_oracle_is_reported():
    table = runs({})
    table["PARK-1_n1"] = VariantOutcomes({NODE: PASS, **oracle({1})})
    failures = evaluate([make_label("VACUOUS")], table, ExpectedCounts(1, 4, 1, 2))
    assert [f.rule for f in failures] == ["3"]


def test_counts_and_label_coverage_are_checked():
    failures = evaluate([make_label("VACUOUS")], runs({}), ExpectedCounts(2, 4, 0, 2))
    assert [f.rule for f in failures] == ["structure"]
    assert "labelled tests: 1, expected 2" in failures[0].message


def test_collected_test_without_label_is_reported():
    table = runs({})
    table[REFERENCE_VARIANT_ID] = VariantOutcomes({NODE: PASS, "tests/t.py::extra": PASS, **oracle(set())})
    messages = [f.message for f in evaluate([make_label("VACUOUS")], table, COUNTS)]
    assert messages == ["collected test without label: tests/t.py::extra"]


def test_error_outcomes_are_anomalies():
    failures = evaluate([make_label("VACUOUS")], runs({"PARK-2_a": TestOutcome.ERROR}), COUNTS)
    assert {f.rule for f in failures} >= {"anomaly", "8"}


def test_a_variant_with_no_outcomes_is_reported_rather_than_read_as_failing():
    table = runs({})
    table["PARK-2_b"] = VariantOutcomes({})
    failures = [f for f in evaluate([make_label("OFFSPEC")], table, COUNTS) if f.rule == "missing"]
    assert [f.message for f in failures] == ["PARK-2_b: no outcome for 3 item(s): tests/t.py::t, oracle 1, oracle 2"]


def test_a_planted_item_without_oracle_outcomes_cannot_pass_rule_3_silently():
    table = runs({})
    table["PARK-1_n1"] = VariantOutcomes({NODE: PASS})
    failures = evaluate([make_label("VACUOUS")], table, ExpectedCounts(1, 4, 1, 2))
    assert any(f.rule == "missing" and "PARK-1_n1" in f.message for f in failures)
