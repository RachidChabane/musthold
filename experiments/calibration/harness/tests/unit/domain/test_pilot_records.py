from __future__ import annotations

import json

import pytest

from calibration.domain.jsonread import RecordError
from calibration.domain.pilot_records import (
    ARM_P,
    ARM_T,
    DIFF_DIFFERS,
    MEASURED_ARMS,
    STATUS_OK,
    ProcessingRow,
    criterion_number,
    criterion_ref,
    parse_bindings,
    parse_differential,
    parse_jsonl,
    parse_mutant_document,
    parse_processing,
)


def _jsonl(*rows: object) -> str:
    return "\n".join(json.dumps(row) for row in rows) + "\n"


class TestCriterionReferences:
    def test_a_reference_wraps_the_number(self) -> None:
        assert criterion_ref(7) == "PARK-7~1"

    @pytest.mark.parametrize("number", [1, 7, 10, 123])
    def test_the_number_is_recovered(self, number: int) -> None:
        assert criterion_number(criterion_ref(number)) == number

    def test_the_arms_are_t_and_p(self) -> None:
        assert MEASURED_ARMS == (ARM_T, ARM_P)


class TestJsonLines:
    def test_blank_lines_are_skipped(self) -> None:
        assert parse_jsonl('{"a":1}\n\n   \n{"a":2}\n', "f") == [{"a": 1}, {"a": 2}]

    def test_a_line_that_is_not_json_names_its_file_and_line(self) -> None:
        with pytest.raises(RecordError, match=r"f\.jsonl:2"):
            parse_jsonl('{"a":1}\nnot json\n', "f.jsonl")

    def test_a_line_that_is_not_an_object_is_refused(self) -> None:
        with pytest.raises(RecordError):
            parse_jsonl("[1]\n", "f")


class TestBindings:
    def test_a_string_value_is_the_reference(self) -> None:
        assert parse_bindings('{"t.py::a": "PARK-1~1"}') == {"t.py::a": "PARK-1~1"}

    def test_a_list_value_keeps_its_first_reference(self) -> None:
        assert parse_bindings('{"t.py::a": ["PARK-2~1", "PARK-3~1"]}') == {"t.py::a": "PARK-2~1"}

    def test_a_reference_that_is_not_text_is_refused(self) -> None:
        with pytest.raises(RecordError):
            parse_bindings('{"t.py::a": 3}')

    def test_bindings_must_be_an_object(self) -> None:
        with pytest.raises(RecordError):
            parse_bindings("[]")


class TestProcessing:
    def test_rows_are_typed(self) -> None:
        content = _jsonl(
            {"mutant_id": "m1", "stillborn": False, "status": STATUS_OK, "duplicate_of": None},
            {"mutant_id": "m2", "stillborn": True, "status": STATUS_OK, "duplicate_of": "m1"},
        )
        assert parse_processing(content) == [
            ProcessingRow("m1", False, STATUS_OK, None),
            ProcessingRow("m2", True, STATUS_OK, "m1"),
        ]

    def test_only_a_non_stillborn_ok_row_is_evaluated(self) -> None:
        assert ProcessingRow("m", False, STATUS_OK, None).evaluated
        assert not ProcessingRow("m", True, STATUS_OK, None).evaluated
        assert not ProcessingRow("m", False, "ERROR", None).evaluated

    def test_a_row_without_a_boolean_stillborn_is_refused(self) -> None:
        with pytest.raises(RecordError):
            parse_processing(_jsonl({"mutant_id": "m", "stillborn": "no", "status": STATUS_OK}))


class TestDifferential:
    def test_rows_are_keyed_by_mutant(self) -> None:
        row = {"mutant_id": "m1", "status": DIFF_DIFFERS, "function": "f"}
        assert parse_differential(_jsonl(row)) == {"m1": row}

    def test_a_row_without_a_mutant_is_refused(self) -> None:
        with pytest.raises(RecordError):
            parse_differential(_jsonl({"status": DIFF_DIFFERS}))


class TestMutantDocument:
    def _document(self, edits: object) -> str:
        return json.dumps({"mutant_id": "m1", "arm": ARM_T, "normalized_edits": edits})

    def test_the_arm_and_the_edits_are_read(self) -> None:
        edit = {"file": "src/a.py", "qualname": "f", "new_source": "def f(): ..."}
        document = parse_mutant_document(self._document([{**edit, "ignored": 1}]))
        assert (document.mutant_id, document.arm, document.normalized_edits) == ("m1", ARM_T, (edit,))

    def test_a_document_without_edits_is_refused(self) -> None:
        with pytest.raises(RecordError):
            parse_mutant_document(json.dumps({"mutant_id": "m1", "arm": ARM_P}))

    def test_an_edit_that_is_not_an_object_is_refused(self) -> None:
        with pytest.raises(RecordError):
            parse_mutant_document(self._document(["edit"]))

    def test_an_edit_missing_a_field_is_refused(self) -> None:
        with pytest.raises(RecordError):
            parse_mutant_document(self._document([{"file": "a.py", "qualname": "f"}]))
