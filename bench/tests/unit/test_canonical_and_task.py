from __future__ import annotations

import pytest

from specbench.domain.canonical import (
    CanonicalError,
    canonical_json,
    canonical_results_json,
    parse_canonical,
    round_floats,
)
from specbench.domain.jsonread import RecordError
from specbench.domain.languages import Language
from specbench.domain.task import CriterionStatus, parse_task, task_from_record
from support.builders import TASK_ID, make_task, task_json


def test_canonical_json_sorts_keys_and_ends_with_one_newline() -> None:
    assert canonical_json({"b": 1, "a": [True, None, "é"]}) == '{"a":[true,null,"é"],"b":1}\n'


def test_canonical_json_refuses_floats_and_nan() -> None:
    with pytest.raises(CanonicalError):
        canonical_json({"x": 0.5})
    with pytest.raises(CanonicalError):
        canonical_json([float("nan")])


def test_results_json_rounds_in_one_place_and_nulls_non_finite() -> None:
    assert canonical_results_json({"x": 1 / 3, "y": float("inf"), "z": -0.0}) == '{"x":0.333333,"y":null,"z":0.0}\n'
    assert round_floats([0.1234567891]) == [0.123457]


def test_parse_canonical_refuses_non_canonical_text() -> None:
    assert parse_canonical('{"a":1}\n') == {"a": 1}
    with pytest.raises(CanonicalError):
        parse_canonical('{"a": 1}\n')
    with pytest.raises(CanonicalError):
        parse_canonical('{"a":1}')


def test_task_round_trips_through_its_canonical_text() -> None:
    task = make_task()
    assert parse_task(task.to_canonical_json()) == task


def test_task_text_is_the_same_whatever_the_dictionary_order() -> None:
    reordered = make_task(lockfile_digests={"b.lock": "4" * 64, "a.lock": "3" * 64})
    assert '"a.lock":"' in reordered.to_canonical_json()
    assert reordered.to_canonical_json().index("a.lock") < reordered.to_canonical_json().index("b.lock")


@pytest.mark.parametrize(
    "mutation",
    [
        {"schema": "specbench.task/2"},
        {"task_id": "lower-case"},
        {"language": "rust"},
        {"base_sha": "xyz"},
        {"pull_number": 0},
        {"licence": {"spdx_id": "GPL-3.0-only", "path": "LICENSE", "notice_path": None}},
        {"criteria": [{"id": "OTHER-TASK-1", "status": "new"}]},
        {"criteria": [{"id": f"{TASK_ID}-1", "status": "new"}, {"id": f"{TASK_ID}-1", "status": "new"}]},
        {"lockfile_digests": {"uv.lock": "short"}},
    ],
)
def test_task_parser_refuses_schema_violations(mutation: dict[str, object]) -> None:
    record = {**make_task().to_record(), **mutation}
    with pytest.raises(RecordError):
        task_from_record(record)  # type: ignore[arg-type]


def test_criterion_status_flags_new_or_revised() -> None:
    task = make_task()
    assert [c.is_new_or_revised for c in task.criteria] == [True, True]
    assert make_task().criteria[0].status is CriterionStatus.NEW


def test_task_json_helper_is_canonical() -> None:
    assert task_json() == make_task().to_canonical_json()
    assert make_task(language=Language.GO).language is Language.GO


def test_a_selected_task_has_no_criteria_and_is_not_constructed() -> None:
    selected = make_task(criteria=())
    assert parse_task(selected.to_canonical_json()) == selected
    assert not selected.is_constructed
    assert make_task().is_constructed
