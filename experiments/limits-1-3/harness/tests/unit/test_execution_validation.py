"""VALID_x and the isolation table (ADR 0006): one predicate, the committed truth reproduces the stated facts."""

from __future__ import annotations

import csv
import json
import re
from pathlib import Path

import pytest
import yaml
from hypothesis import given, strategies as st

from pilot.domain.execution_validation import other_binding_veto, valid_x_criteria
from pilot.domain.isolation_table import isolation_table
from pilot.domain.outcomes import KillResult

C3, C5 = "PARK-3~1", "PARK-5~1"
BINDINGS = {"a": C3, "b": C5, "c": C3}
PREREG = Path(__file__).resolve().parents[3] / "prereg"


def test_a_kill_on_a_test_bound_to_another_criterion_vetoes():
    assert other_binding_veto({"a": KillResult.SURVIVED, "b": KillResult.KILLED}, BINDINGS, C3)
    assert not other_binding_veto({"a": KillResult.KILLED, "c": KillResult.KILLED, "b": KillResult.SURVIVED}, BINDINGS, C3)


def test_an_error_on_another_criterions_test_vetoes_and_a_flaky_result_does_not():
    assert other_binding_veto({"b": KillResult.ERROR}, BINDINGS, C3)
    assert not other_binding_veto({"b": KillResult.FLAKY, "a": KillResult.SURVIVED}, BINDINGS, C3)


def test_a_test_without_a_binding_counts_as_another_criterions_test():
    assert other_binding_veto({"unbound": KillResult.KILLED}, BINDINGS, C3)


def test_valid_x_keeps_the_valid_w_criteria_no_other_binding_vetoes():
    results = {"a": KillResult.KILLED, "b": KillResult.KILLED}
    assert valid_x_criteria([C3, C5], True, results, BINDINGS) == frozenset()
    assert valid_x_criteria([C3], True, {"a": KillResult.KILLED}, BINDINGS) == {C3}
    assert valid_x_criteria([C3], False, {"a": KillResult.KILLED}, BINDINGS) == frozenset()


@given(results=st.dictionaries(st.sampled_from("abcde"), st.sampled_from(list(KillResult)), max_size=5), added=st.sampled_from("abcde"))
def test_a_larger_suite_only_shrinks_valid_x(results, added):
    bindings = {"a": C3, "b": C5, "c": C3, "d": C5, "e": "PARK-9~1"}
    smaller = {n: r for n, r in results.items() if n != added}
    assert valid_x_criteria([C3, C5], True, results, bindings) <= valid_x_criteria([C3, C5], True, smaller, bindings)


@pytest.fixture(scope="module")
def committed_truth():
    labels = yaml.safe_load((PREREG / "labels.yaml").read_text())
    bindings = {entry["nodeid"]: entry["cites"] for entry in labels}
    rows = list(csv.reader((PREREG / "label_matrix.csv").read_text().splitlines()))
    matrix = {row[0]: dict(zip(rows[0][1:], row[1:], strict=True)) for row in rows[1:]}
    return isolation_table(matrix, bindings, range(1, 11))


def test_the_committed_truth_reproduces_the_seven_killed_violations(committed_truth):
    assert sorted(committed_truth.killed_by_other) == [
        "PARK-3_a", "PARK-3_b", "PARK-3_c", "PARK-5_b", "PARK-5_c", "PARK-8_a", "PARK-8_c",
    ]


def test_the_committed_truth_reproduces_the_surviving_violations_per_criterion(committed_truth):
    assert {n: len(committed_truth.surviving(n)) for n in range(1, 11)} == {
        1: 3, 2: 3, 3: 0, 4: 3, 5: 1, 6: 3, 7: 3, 8: 1, 9: 3, 10: 3,
    }
    assert committed_truth.surviving(5) == ("PARK-5_a",) and committed_truth.surviving(8) == ("PARK-8_b",)


def test_the_committed_truth_has_sixteen_of_twenty_regression_items_surviving(committed_truth):
    mapping = committed_truth.to_mapping()
    assert (mapping["surviving_regression_items"], mapping["regression_items"]) == (16, 20)


def test_c_d8_is_the_ten_criteria_minus_park_3(committed_truth):
    assert committed_truth.d8_criteria == (1, 2, 4, 5, 6, 7, 8, 9, 10)


def test_the_committed_isolation_table_file_equals_the_computed_one(committed_truth):
    assert json.loads((PREREG / "isolation_table.json").read_text()) == json.loads(json.dumps(committed_truth.to_mapping()))


def test_a_violation_killed_by_its_own_criterions_test_only_survives_isolation():
    matrix = {"t1": {"PARK-1_a": "fail", "PARK-1_b": "fail"}, "t2": {"PARK-1_a": "pass", "PARK-1_b": "fail"}}
    table = isolation_table(matrix, {"t1": "PARK-1~1", "t2": "PARK-2~1"}, [1, 2])
    assert table.killed_by_other == {"PARK-1_b"} and table.d8_criteria == (1,)


def test_a_criterion_number_is_never_confused_with_one_that_shares_its_prefix():
    matrix = {"t1": {"PARK-1_a": "fail"}, "t2": {"PARK-10_a": "fail"}}
    table = isolation_table(matrix, {"t1": "PARK-10~1", "t2": "PARK-10~1"}, [1, 10])
    assert table.killed_by_other == {"PARK-1_a"} and re.fullmatch(r"PARK-\d+_a", "PARK-10_a")
