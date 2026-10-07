from __future__ import annotations

import json
from collections.abc import Mapping
from pathlib import Path

from calibration.application.build_ports import UnitPair
from calibration.application.screen_items import (
    WITNESS_MAPPED,
    WITNESS_NONE,
    WITNESS_UNMAPPED,
    ScreenCandidate,
    assemble_screen_items,
    population_ids,
    prepare_candidate,
    screen_problems,
)
from calibration.domain.canonical import JsonValue, canonical_json, sha256_hex
from calibration.domain.items import KindFamily
from calibration.domain.models import identifiers
from calibration.domain.pilot_records import MutantDocument, ProcessingRow
from calibration.domain.schedule import mutant_request_id
from calibration.domain.witness_mapping import function_signatures
from support.build_fakes import FakeBuilders, FakePilotDomain
from support.build_scenario import FIXTURE_SOURCES, SCREEN_TEMPLATE_DIGEST, differs

PROJECT = Path("/project")
SIGNATURES = function_signatures(FIXTURE_SOURCES)
EDIT = {"file": "src/parkfee/fees.py", "qualname": "compute_fee", "new_source": "def compute_fee(): return 1"}


def _document(mutant_id: str, arm: str = "T") -> MutantDocument:
    return MutantDocument(mutant_id, arm, (EDIT,))


def _row(mutant_id: str, *, stillborn: bool = False, status: str = "OK") -> ProcessingRow:
    return ProcessingRow(mutant_id, stillborn, status, None)


def _candidate(mutant_id: str, status: str = WITNESS_NONE) -> ScreenCandidate:
    return ScreenCandidate(mutant_id, "T", (), None, status, None)


def _build_output(mutant_ids: list[str]) -> Mapping[str, JsonValue]:
    mutants: list[Mapping[str, JsonValue]] = [{"mutant_id": mutant_id} for mutant_id in mutant_ids]
    return FakeBuilders({}, {}, SCREEN_TEMPLATE_DIGEST).screen_build(PROJECT, PROJECT, identifiers(), mutants)


def _prepare(differential: Mapping[str, object] | None) -> ScreenCandidate:
    return prepare_candidate(_document("m1"), differential, FIXTURE_SOURCES, SIGNATURES, FakePilotDomain())


class TestPopulation:
    def test_only_evaluated_mutants_of_the_measured_arms_with_a_document_are_in_it(self) -> None:
        rows = [
            _row("ok-t"),
            _row("ok-p"),
            _row("stillborn", stillborn=True),
            _row("duplicate", status="DUPLICATE"),
            _row("other-arm"),
            _row("no-document"),
        ]
        documents = {
            "ok-t": _document("ok-t", "T"),
            "ok-p": _document("ok-p", "P"),
            "stillborn": _document("stillborn"),
            "duplicate": _document("duplicate"),
            "other-arm": _document("other-arm", "X"),
        }

        assert population_ids(rows, documents) == ["ok-p", "ok-t"]

    def test_the_population_is_sorted_by_mutant_id(self) -> None:
        rows = [_row("c"), _row("a"), _row("b")]
        documents = {name: _document(name) for name in "abc"}

        assert population_ids(rows, documents) == ["a", "b", "c"]


class TestCandidates:
    def test_a_mutant_without_a_differential_carries_no_witness(self) -> None:
        candidate = _prepare(None)

        assert (candidate.witness, candidate.witness_status, candidate.unmapped_reason) == (None, WITNESS_NONE, None)

    def test_a_differential_without_a_difference_carries_no_witness(self) -> None:
        candidate = _prepare({"status": "NO_DIFF_FOUND"})

        assert candidate.witness_status == WITNESS_NONE

    def test_a_differential_that_maps_gives_a_witness(self) -> None:
        candidate = _prepare(differs(mapped=True))

        assert candidate.witness_status == WITNESS_MAPPED
        assert candidate.witness is not None
        assert candidate.witness["call"] == {
            "operation": "compute_fee",
            "args": {
                "start": {"type": "datetime", "value": "2026-01-01T00:00:00"},
                "end": {"type": "datetime", "value": "2026-01-01T01:00:00"},
            },
        }

    def test_a_differential_that_maps_to_nothing_is_unmapped_with_its_reason(self) -> None:
        candidate = _prepare(differs(mapped=False))

        assert candidate.witness is None
        assert candidate.witness_status == WITNESS_UNMAPPED
        assert candidate.unmapped_reason is not None

    def test_the_units_are_the_pilot_domains_before_and_after_pairs(self) -> None:
        candidate = _prepare(None)

        assert candidate.units == (
            UnitPair(EDIT["file"], EDIT["qualname"], FIXTURE_SOURCES[EDIT["file"]], EDIT["new_source"]),
        )

    def test_the_builder_input_has_the_edits_and_the_witness_when_there_is_one(self) -> None:
        mapped = _prepare(differs(mapped=True)).builder_input()
        bare = _prepare(None).builder_input()

        assert mapped["mutant_id"] == "m1"
        assert mapped["edits"] == [
            {
                "file": EDIT["file"],
                "qualname": EDIT["qualname"],
                "before": FIXTURE_SOURCES[EDIT["file"]],
                "after": EDIT["new_source"],
            }
        ]
        assert isinstance(mapped["witnesses"], list)
        assert len(mapped["witnesses"]) == 1
        assert bare["witnesses"] == []


class TestAssembly:
    def test_each_built_item_is_a_screen_item_under_its_mutant_request_id(self) -> None:
        candidates = [_candidate("m2", WITNESS_MAPPED), _candidate("m1")]

        items = assemble_screen_items(_build_output(["m2", "m1"]), candidates)

        assert [item.request_id for item in items] == [mutant_request_id("m1"), mutant_request_id("m2")]
        assert all(item.family is KindFamily.SCREEN for item in items)

    def test_the_witness_status_is_the_candidates(self) -> None:
        candidates = [_candidate("m1", WITNESS_MAPPED), _candidate("m2", WITNESS_UNMAPPED)]

        items = assemble_screen_items(_build_output(["m1", "m2"]), candidates)

        assert [item.witness_status for item in items] == [WITNESS_MAPPED, WITNESS_UNMAPPED]

    def test_the_closed_set_digest_is_that_of_the_text_the_builder_rendered(self) -> None:
        (item,) = assemble_screen_items(_build_output(["m1"]), [_candidate("m1")])

        closed = {"questions": {"violates": "does mutant m1 violate the criterion"}}
        assert item.closed == closed
        assert item.closed_digest == sha256_hex(canonical_json(closed))
        assert json.loads(canonical_json(item.closed)) == closed

    def test_bodies_questions_and_template_digest_are_kept(self) -> None:
        (item,) = assemble_screen_items(_build_output(["m1"]), [_candidate("m1")])

        assert tuple(item.bodies) == identifiers()
        assert item.question_to_ref == {"violates": "PARK-1~1"}
        assert item.template_digest == SCREEN_TEMPLATE_DIGEST
        assert item.mutant_id == "m1"

    def test_an_item_that_answers_no_candidate_is_kept_without_a_witness_status_for_the_problem_check(self) -> None:
        (item,) = assemble_screen_items(_build_output(["stranger"]), [_candidate("m1")])

        assert item.witness_status is None
        assert screen_problems([item], [_candidate("m1")], SCREEN_TEMPLATE_DIGEST) == [
            "mutant m1 has no request",
            "request for the unknown mutant stranger",
        ]


class TestProblems:
    def test_one_item_per_candidate_with_the_modules_digest_is_no_problem(self) -> None:
        items = assemble_screen_items(_build_output(["m1", "m2"]), [_candidate("m1"), _candidate("m2")])

        assert screen_problems(items, [_candidate("m1"), _candidate("m2")], SCREEN_TEMPLATE_DIGEST) == []

    def test_a_foreign_template_digest_is_a_problem(self) -> None:
        items = assemble_screen_items(_build_output(["m1"]), [_candidate("m1")])

        problems = screen_problems(items, [_candidate("m1")], "another")

        assert problems == [f"{mutant_request_id('m1')}: template digest {SCREEN_TEMPLATE_DIGEST} is not the module's"]

    def test_a_candidate_without_a_request_is_a_problem(self) -> None:
        assert screen_problems([], [_candidate("m1")], SCREEN_TEMPLATE_DIGEST) == ["mutant m1 has no request"]
