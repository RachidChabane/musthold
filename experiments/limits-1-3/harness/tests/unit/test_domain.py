from __future__ import annotations

import pytest

from pilot.domain.edits import EditError, EditFailure, EditSet
from pilot.domain.label_matrix import LabelMatrix
from pilot.domain.outcome_matrix import VariantOutcomes, oracle_criterion
from pilot.domain.outcomes import KillResult, RunResult, TestOutcome, classify_kill
from pilot.domain.variants import VariantId, VariantKind


def test_edit_set_round_trips_through_mapping():
    raw = {"edits": [{"file": "a.py", "qualname": "f", "new_source": "def f(): ..."}]}
    assert EditSet.from_mapping(raw).to_mapping() == raw


@pytest.mark.parametrize(
    "raw",
    [{}, {"edits": []}, {"edits": "x"}, {"edits": [{"file": "a.py"}]}, {"edits": [1]}],
)
def test_edit_set_rejects_malformed_input(raw):
    with pytest.raises(EditError) as caught:
        EditSet.from_mapping(raw)
    assert caught.value.failure is EditFailure.MALFORMED


def test_edit_set_lists_each_file_once_in_order():
    edit_set = EditSet.from_mapping(
        {"edits": [{"file": b, "qualname": "f", "new_source": "x"} for b in ("b.py", "a.py", "b.py")]}
    )
    assert edit_set.files == ("b.py", "a.py")


@pytest.mark.parametrize(
    ("first", "rerun", "expected"),
    [
        (TestOutcome.PASSED, None, KillResult.SURVIVED),
        (TestOutcome.SKIPPED, None, KillResult.SURVIVED),
        (TestOutcome.ERROR, None, KillResult.ERROR),
        (TestOutcome.FAILED, TestOutcome.FAILED, KillResult.KILLED),
        (TestOutcome.FAILED, TestOutcome.PASSED, KillResult.FLAKY),
        (TestOutcome.FAILED, None, KillResult.FLAKY),
    ],
)
def test_classify_kill(first, rerun, expected):
    assert classify_kill(first, rerun) is expected


def test_flaky_counts_as_survived_and_killed_does_not():
    assert KillResult.FLAKY.counts_as_survived
    assert KillResult.SURVIVED.counts_as_survived
    assert not KillResult.KILLED.counts_as_survived
    assert not KillResult.ERROR.counts_as_survived


def test_run_result_defaults_missing_tests_to_error():
    result = RunResult({"a": TestOutcome.PASSED}, 0)
    assert result.outcome_of("missing") is TestOutcome.ERROR
    assert result.all_passed
    assert not RunResult({}, 0).all_passed


@pytest.mark.parametrize(
    ("value", "kind", "criterion"),
    [
        ("PARK-3_a", VariantKind.VIOLATION, 3),
        ("PARK-10_c", VariantKind.VIOLATION, 10),
        ("PARK-2_n1", VariantKind.PLANTED, 2),
        ("R", VariantKind.REFACTOR, None),
    ],
)
def test_variant_id_parsing(value, kind, criterion):
    parsed = VariantId.parse(value)
    assert (parsed.kind, parsed.criterion) == (kind, criterion)


def test_variant_id_rejects_unknown_names():
    with pytest.raises(ValueError):
        VariantId.parse("PARK-3_z")
    assert VariantId.parse_or_none("nonsense") is None


def test_oracle_criterion_reads_the_file_name_only():
    assert oracle_criterion("tests/test_oracle_park_7.py::test_x") == 7
    assert oracle_criterion("tests/test_tariff.py::test_oracle_park_7") is None


def test_variant_outcomes_group_oracle_results_by_criterion():
    run = VariantOutcomes(
        {
            "tests/test_a.py::t": TestOutcome.PASSED,
            "tests/test_oracle_park_1.py::a": TestOutcome.PASSED,
            "tests/test_oracle_park_1.py::b": TestOutcome.FAILED,
            "tests/test_oracle_park_2.py::a": TestOutcome.PASSED,
        }
    )
    assert run.project_nodeids() == ["tests/test_a.py::t"]
    assert not run.oracle_passed(1)
    assert run.oracle_passed(2)
    assert not run.oracle_passed(3)
    assert run.oracle_criteria() == [1, 2]


def test_variant_outcomes_report_error_and_skipped_as_anomalies():
    run = VariantOutcomes({"a": TestOutcome.ERROR, "b": TestOutcome.SKIPPED, "c": TestOutcome.FAILED})
    assert run.anomalies() == ["a=error", "b=skipped"]


def test_label_matrix_orders_columns_and_excludes_planted_items():
    ok = VariantOutcomes({"t": TestOutcome.PASSED})
    runs = {"R": ok, "PARK-10_a": ok, "PARK-2_b": ok, "PARK-2_n1": ok, "REFERENCE": ok, "PARK-2_a": ok}
    assert LabelMatrix.from_runs(["t"], runs).variant_ids == ("PARK-2_a", "PARK-2_b", "PARK-10_a", "R")


def test_label_matrix_differences_name_every_mismatch():
    ours = LabelMatrix(("v1", "v2"), {"t1": ("pass", "fail"), "t2": ("pass", "pass")})
    theirs = LabelMatrix(("v1", "v3"), {"t1": ("pass", "pass"), "t3": ("fail", "fail")})
    assert ours.differences(theirs) == [
        "variant only in first: v2",
        "variant only in second: v3",
        "test only in first: t2",
        "test only in second: t3",
    ]
    assert ours.differences(ours) == []


def test_label_matrix_cell_mismatch_is_reported():
    ours = LabelMatrix(("v",), {"t": ("pass",)})
    theirs = LabelMatrix(("v",), {"t": ("fail",)})
    assert ours.differences(theirs) == ["t x v: pass != fail"]
