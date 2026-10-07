from __future__ import annotations

import json
from collections.abc import Mapping
from dataclasses import replace

import pytest

from calibration.application.alignment_items import (
    AlignmentAssembly,
    alignment_problems,
    assemble_alignment_items,
)
from calibration.domain.canonical import JsonValue, sha256_hex
from calibration.domain.items import KindFamily
from calibration.domain.jsonread import RecordError
from calibration.domain.models import identifiers
from calibration.domain.schedule import alignment_request_id
from support.build_scenario import ALIGNMENT_TEMPLATE_DIGEST, DEFAULT_TESTS, AlignmentTest, alignment_build_output

ACCEPTED_FIRST, ACCEPTED_SECOND, MASKED = DEFAULT_TESTS


def _assemble(tests: tuple[AlignmentTest, ...] = DEFAULT_TESTS) -> AlignmentAssembly:
    return assemble_alignment_items(alignment_build_output(tests, ALIGNMENT_TEMPLATE_DIGEST))


def _bindings() -> dict[str, str]:
    return {test.nodeid: test.reference for test in DEFAULT_TESTS}


class TestAssembly:
    def test_each_accepted_test_is_an_alignment_item_under_its_request_id(self) -> None:
        assembly = _assemble()

        assert {item.request_id for item in assembly.items} == {
            alignment_request_id(ACCEPTED_FIRST.nodeid),
            alignment_request_id(ACCEPTED_SECOND.nodeid),
        }
        assert all(item.family is KindFamily.ALIGNMENT for item in assembly.items)

    def test_the_node_id_leaves_out_the_class_part_when_the_classname_is_the_module(self) -> None:
        assembly = _assemble()

        units = {item.unit for item in assembly.items}
        assert "tests/test_fee.py::test_free_hour" in units
        assert "tests/test_fee.py::TestFee::test_charges" in units

    def test_items_are_ordered_by_request_id(self) -> None:
        assembly = _assemble(tuple(reversed(DEFAULT_TESTS)))

        ids = [item.request_id for item in assembly.items]
        assert ids == sorted(ids)

    def test_the_closed_set_is_kept_verbatim_and_digested_as_rendered(self) -> None:
        raw = alignment_build_output(DEFAULT_TESTS, ALIGNMENT_TEMPLATE_DIGEST)
        entry = _first_entry(raw)
        closed_text = str(entry["closed"])

        item = assemble_alignment_items(raw).items[0]

        assert item.closed == json.loads(closed_text)
        assert item.closed_digest == sha256_hex(closed_text)

    def test_a_body_is_kept_for_every_model_identifier(self) -> None:
        item = _assemble().items[0]

        assert tuple(item.bodies) == identifiers()

    def test_the_declared_reference_is_the_first_one_and_the_tokens_are_kept(self) -> None:
        item = next(i for i in _assemble().items if i.nodeid == ACCEPTED_FIRST.nodeid)

        assert item.declared == "PARK-1~1"
        assert item.token_to_ref == {"T1": "PARK-1~1"}
        assert item.label == "label of test_charges"

    def test_a_test_without_declared_reference_is_an_item_with_none_declared(self) -> None:
        raw = dict(alignment_build_output((ACCEPTED_FIRST,), ALIGNMENT_TEMPLATE_DIGEST))
        entry = dict(_first_entry(raw))
        entry["declared"] = []
        raw["items"] = [entry]

        assert assemble_alignment_items(raw).items[0].declared is None

    def test_a_masker_rejection_is_listed_with_its_reason_and_is_not_an_item(self) -> None:
        assembly = _assemble()

        (rejection,) = assembly.rejections
        assert rejection.nodeid == MASKED.nodeid
        assert rejection.request_id == alignment_request_id(MASKED.nodeid)
        assert (rejection.reason, rejection.detail) == ("unmaskable", "the test names its criterion")
        assert MASKED.nodeid not in {item.unit for item in assembly.items}
        assert rejection.to_record()["label"] == "label of test_masked"

    def test_a_rejected_test_still_has_its_declared_references_recorded(self) -> None:
        assert _assemble().declared[MASKED.nodeid] == ("PARK-3~1",)

    def test_the_criteria_and_the_template_digest_are_kept(self) -> None:
        assembly = _assemble()

        assert assembly.criteria == ("PARK-1~1", "PARK-2~1", "PARK-3~1")
        assert assembly.template_digest == ALIGNMENT_TEMPLATE_DIGEST

    def test_a_build_without_items_is_a_record_error(self) -> None:
        with pytest.raises(RecordError, match=r"build\.items"):
            assemble_alignment_items({"criteria": [], "template_digest": "x"})

    def test_an_item_without_a_label_is_a_record_error(self) -> None:
        raw = dict(alignment_build_output((ACCEPTED_FIRST,), ALIGNMENT_TEMPLATE_DIGEST))
        entry = {key: value for key, value in _first_entry(raw).items() if key != "label"}
        raw["items"] = [entry]

        with pytest.raises(RecordError, match=r"item\.label"):
            assemble_alignment_items(raw)


class TestProblems:
    def test_a_module_that_binds_what_the_pilot_bound_has_no_problem(self) -> None:
        assert alignment_problems(_assemble(), _bindings(), ALIGNMENT_TEMPLATE_DIGEST) == []

    def test_a_test_bound_in_the_pilot_and_not_by_the_module_is_a_problem(self) -> None:
        bindings = {**_bindings(), "tests/test_fee.py::test_ghost": "PARK-4~1"}

        problems = alignment_problems(_assemble(), bindings, ALIGNMENT_TEMPLATE_DIGEST)

        assert problems == ["tests/test_fee.py::test_ghost is bound in the pilot and not by the module"]

    def test_a_test_bound_by_the_module_and_not_in_the_pilot_is_a_problem(self) -> None:
        bindings = _bindings()
        del bindings[ACCEPTED_SECOND.nodeid]

        problems = alignment_problems(_assemble(), bindings, ALIGNMENT_TEMPLATE_DIGEST)

        assert problems == [f"{ACCEPTED_SECOND.nodeid} is bound by the module and not in the pilot"]

    def test_a_different_reference_is_a_problem(self) -> None:
        bindings = {**_bindings(), ACCEPTED_FIRST.nodeid: "PARK-9~1"}

        (problem,) = alignment_problems(_assemble(), bindings, ALIGNMENT_TEMPLATE_DIGEST)

        assert "PARK-9~1" in problem

    def test_a_test_bound_to_two_references_by_the_module_differs_from_the_pilots_first(self) -> None:
        raw = dict(alignment_build_output((ACCEPTED_FIRST,), ALIGNMENT_TEMPLATE_DIGEST))
        entry = dict(_first_entry(raw))
        entry["declared"] = ["PARK-1~1", "PARK-2~1"]
        raw["items"] = [entry]

        problems = alignment_problems(
            assemble_alignment_items(raw), {ACCEPTED_FIRST.nodeid: "PARK-1~1"}, ALIGNMENT_TEMPLATE_DIGEST
        )

        assert len(problems) == 1

    def test_an_item_with_a_foreign_template_digest_is_a_problem(self) -> None:
        assembly = _assemble()
        altered = replace(assembly.items[0], template_digest="foreign")

        problems = alignment_problems(
            replace(assembly, items=(altered, *assembly.items[1:])), _bindings(), ALIGNMENT_TEMPLATE_DIGEST
        )

        assert problems == [f"{altered.request_id}: template digest foreign is not the module's"]

    def test_a_builder_template_digest_that_differs_is_a_problem(self) -> None:
        assembly = replace(_assemble(), template_digest="foreign")

        problems = alignment_problems(assembly, _bindings(), ALIGNMENT_TEMPLATE_DIGEST)

        assert problems == ["the builder's template digest differs from the module's decide.closed digest"]

    def test_problems_come_in_node_id_order(self) -> None:
        bindings = {"b::x": "PARK-1~1", "a::x": "PARK-1~1"}
        empty = AlignmentAssembly((), (), {}, (), ALIGNMENT_TEMPLATE_DIGEST)

        problems = alignment_problems(empty, bindings, ALIGNMENT_TEMPLATE_DIGEST)

        assert [problem.split(" ")[0] for problem in problems] == ["a::x", "b::x"]


def _first_entry(raw: Mapping[str, JsonValue]) -> dict[str, JsonValue]:
    items = raw["items"]
    assert isinstance(items, list)
    first = items[0]
    assert isinstance(first, dict)
    return first
