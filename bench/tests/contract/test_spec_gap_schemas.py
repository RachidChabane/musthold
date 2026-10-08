from __future__ import annotations

import copy
from typing import Any

import pytest

from specbench.adapters.schemas import SchemaName, load_schema, schema_problems
from specbench.domain.canonical import JsonValue
from specbench.domain.gaps import gaps_record
from specbench.domain.hunks import MAX_HUNKS_PER_CRITERION, CodeHunk
from specbench.domain.jsonread import RecordError
from specbench.domain.languages import Language
from specbench.domain.sealing import BUNDLE_VERSION_PATH, CorpusLock, lock_entry_for
from specbench.domain.spec import (
    Dimension,
    InterfaceEntry,
    SpecCriterion,
    SpecDraft,
    spec_from_record,
    spec_to_record,
)
from specbench.domain.task import CriterionStatus
from support.builders import TASK_ID, make_bundle, task_json

HUNK = CodeHunk("pkg/fee.py", 2, 3)
DRAFT = SpecDraft(
    "Parking fee",
    "The fee of a stay.",
    (InterfaceEntry("pkg.fee", "fee(minutes: int) -> int", ""),),
    (
        SpecCriterion(
            1,
            "WHEN short THE SYSTEM SHALL charge 0",
            ("fee(minutes=7) => 0",),
            CriterionStatus.NEW,
            Dimension.BOUNDARY,
            (HUNK,),
        ),
        SpecCriterion(
            2,
            "WHEN long THE SYSTEM SHALL charge 5",
            ("fee(minutes=60) => 5",),
            CriterionStatus.UNCHANGED,
            Dimension.ERROR,
            (),
        ),
    ),
)


def record() -> dict[str, Any]:
    return copy.deepcopy(spec_to_record(DRAFT))  # type: ignore[return-value]


def with_first_criterion(**changes: JsonValue) -> dict[str, Any]:
    body = record()
    body["criteria"][0].update(changes)
    return body


def without_first_criterion_field(field: str) -> dict[str, Any]:
    body = record()
    del body["criteria"][0][field]
    return body


def test_a_spec_record_conforms_to_the_schema_and_round_trips_through_the_parser() -> None:
    assert schema_problems(SchemaName.SPEC, record()) == []
    assert spec_from_record(record()) == DRAFT


SCHEMA_ONLY_REFUSALS = [
    with_first_criterion(implementingHunks=[{"path": "a.py", "startLine": 1, "endLine": 1, "extra": 1}]),
    with_first_criterion(implementingHunks=[HUNK.to_record()] * (MAX_HUNKS_PER_CRITERION + 1)),
]
SHARED_REFUSALS = [
    without_first_criterion_field("dimension"),
    without_first_criterion_field("implementingHunks"),
    with_first_criterion(dimension="other"),
    with_first_criterion(dimension="Error"),
    with_first_criterion(implementingHunks=[{"path": "a.py", "startLine": 0, "endLine": 1}]),
    with_first_criterion(implementingHunks=[{"path": "a.py", "startLine": 1}]),
    with_first_criterion(implementingHunks=[{"path": "", "startLine": 1, "endLine": 1}]),
    with_first_criterion(implementingHunks=[{"path": "a.py", "start_line": 1, "end_line": 1}]),
    with_first_criterion(implementingHunks=HUNK.to_record()),
]


@pytest.mark.parametrize("broken", SHARED_REFUSALS)
def test_the_schema_and_the_parser_refuse_the_same_malformed_spec_gap_fields(broken: dict[str, Any]) -> None:
    assert schema_problems(SchemaName.SPEC, broken) != []
    with pytest.raises(RecordError):
        spec_from_record(broken)


@pytest.mark.parametrize("broken", SCHEMA_ONLY_REFUSALS)
def test_the_schema_alone_refuses_unknown_hunk_keys_and_too_many_hunks_which_the_validator_also_bounds(
    broken: dict[str, Any],
) -> None:
    assert schema_problems(SchemaName.SPEC, broken) != []


def test_the_schema_accepts_the_most_hunks_a_criterion_may_cite() -> None:
    body = with_first_criterion(implementingHunks=[HUNK.to_record()] * MAX_HUNKS_PER_CRITERION)
    assert schema_problems(SchemaName.SPEC, body) == []


def test_the_schema_names_the_dimensions_the_hunk_fields_and_the_hunk_limit_of_the_domain() -> None:
    criterion = load_schema(SchemaName.SPEC)["properties"]["criteria"]["items"]["properties"]
    assert criterion["dimension"]["enum"] == [d.value for d in Dimension]
    hunks = criterion["implementingHunks"]
    assert hunks["maxItems"] == MAX_HUNKS_PER_CRITERION
    assert sorted(hunks["items"]["properties"]) == sorted(HUNK.to_record())
    assert sorted(hunks["items"]["required"]) == sorted(HUNK.to_record())


@pytest.mark.parametrize("kind", ["wrong_dimension", "wrong_hunks"])
def test_the_review_schema_accepts_the_hunk_and_dimension_findings(kind: str) -> None:
    review: dict[str, JsonValue] = {"findings": [{"criterion": 1, "kind": kind, "comment": "Cite the return."}]}
    assert schema_problems(SchemaName.REVIEW, review) == []
    assert (
        schema_problems(SchemaName.REVIEW, {"findings": [{"criterion": 1, "kind": "wrong_path", "comment": "x"}]}) != []
    )


def test_the_gap_record_conforms_and_the_schema_refuses_a_draw_of_another_size() -> None:
    drawn = gaps_record(20261007, TASK_ID, (1, 3))
    assert schema_problems(SchemaName.GAPS, drawn) == []
    for broken in (
        {**drawn, "criteria": [f"{TASK_ID}-1"]},
        {**drawn, "criteria": [f"{TASK_ID}-1", f"{TASK_ID}-1"]},
        {**drawn, "criteria": [f"{TASK_ID}-1", f"{TASK_ID}-2", f"{TASK_ID}-3"]},
        {**drawn, "dimension": "error"},
        {"criteria": drawn["criteria"]},
    ):
        assert schema_problems(SchemaName.GAPS, broken) != []


def test_the_corpus_lock_records_the_version_record_among_the_bundle_digests() -> None:
    entry = lock_entry_for(TASK_ID, Language.PYTHON, task_json(), make_bundle())
    assert BUNDLE_VERSION_PATH in entry.files
    assert schema_problems(SchemaName.CORPUS_LOCK, CorpusLock((entry,)).to_record()) == []
