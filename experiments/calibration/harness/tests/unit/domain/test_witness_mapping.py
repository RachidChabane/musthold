from __future__ import annotations

from collections.abc import Mapping

import pytest

from calibration.domain.witness_mapping import (
    KIND_RAISES,
    KIND_RETURNS,
    TYPE_BOOLEAN,
    TYPE_DATETIME,
    TYPE_DECIMAL,
    TYPE_INTEGER,
    TYPE_NULL,
    TYPE_STRING,
    function_signatures,
    map_differential,
)

OPERATION = "compute_fee"
SIGNATURES = {OPERATION: {"entry": "datetime", "rate": "Decimal", "member": "bool", "count": "int", "plain": ""}}
SOURCE = """
from datetime import datetime
from decimal import Decimal

def compute_fee(entry: datetime, rate: Decimal, *, member: bool, count: int = 0, plain=None):
    return rate

async def refund(amount: Decimal) -> None: ...

class Holder:
    def method(self, hidden: int) -> None: ...
"""


def _record(
    arguments: Mapping[str, object] | None = None,
    original: object = None,
    mutant: object = None,
    function: object = OPERATION,
) -> dict[str, object]:
    return {
        "function": function,
        "args": arguments if arguments is not None else {"rate": "1.50"},
        "original_out": original if original is not None else {"ok": ["str", "a"]},
        "mutant_out": mutant if mutant is not None else {"ok": ["str", "b"]},
    }


def _mapped(output: object) -> Mapping[str, object]:
    result = map_differential(_record(original=output), SIGNATURES)
    assert result.witness is not None, result.unmapped_reason
    reference = result.witness["reference"]
    assert isinstance(reference, dict)
    return reference


class TestSignatures:
    def test_module_level_functions_give_their_annotations(self) -> None:
        signatures = function_signatures({"fee.py": SOURCE})
        assert signatures[OPERATION] == {
            "entry": "datetime",
            "rate": "Decimal",
            "member": "bool",
            "count": "int",
            "plain": "",
        }

    def test_async_functions_are_read_and_methods_are_not(self) -> None:
        signatures = function_signatures({"fee.py": SOURCE})
        assert signatures["refund"] == {"amount": "Decimal"}
        assert "method" not in signatures

    def test_several_sources_are_merged(self) -> None:
        signatures = function_signatures({"a.py": "def f(x: bool): ...", "b.py": "def g(y: Decimal): ..."})
        assert set(signatures) == {"f", "g"}


class TestArguments:
    def test_an_argument_by_annotation_maps_to_its_literal(self) -> None:
        arguments = {"entry": "2026-10-07T08:00:00", "rate": "1.50", "member": True}
        result = map_differential(_record(arguments), SIGNATURES)
        assert result.unmapped_reason is None
        assert result.witness is not None
        assert result.witness["call"] == {
            "operation": OPERATION,
            "args": {
                "entry": {"type": TYPE_DATETIME, "value": "2026-10-07T08:00:00"},
                "rate": {"type": TYPE_DECIMAL, "value": "1.50"},
                "member": {"type": TYPE_BOOLEAN, "value": "true"},
            },
        }

    def test_a_false_boolean_maps_to_false(self) -> None:
        result = map_differential(_record({"member": False}), SIGNATURES)
        assert result.witness is not None
        call = result.witness["call"]
        assert call == {"operation": OPERATION, "args": {"member": {"type": TYPE_BOOLEAN, "value": "false"}}}

    @pytest.mark.parametrize(
        "arguments",
        [{"count": 3}, {"plain": "x"}, {"unknown": "x"}, {"member": "true"}, {"rate": 1.5}, {"entry": 5}],
    )
    def test_an_argument_no_row_maps_leaves_the_witness_unmapped(self, arguments: Mapping[str, object]) -> None:
        result = map_differential(_record(arguments), SIGNATURES)
        assert result.witness is None
        assert result.unmapped_reason is not None


class TestOutputs:
    def test_decimal_is_written_in_positional_notation(self) -> None:
        assert _mapped({"ok": ["decimal", "1E+2"]}) == {"kind": KIND_RETURNS, "type": TYPE_DECIMAL, "value": "100"}

    def test_a_decimal_keeps_its_fraction(self) -> None:
        assert _mapped({"ok": ["decimal", "2.50"]})["value"] == "2.50"

    def test_a_small_decimal_is_not_scientific(self) -> None:
        assert _mapped({"ok": ["decimal", "1E-7"]})["value"] == "0.0000001"

    def test_str(self) -> None:
        assert _mapped({"ok": ["str", "hello"]}) == {"kind": KIND_RETURNS, "type": TYPE_STRING, "value": "hello"}

    def test_repr_none_is_null(self) -> None:
        assert _mapped({"ok": ["repr", "None"]}) == {"kind": KIND_RETURNS, "type": TYPE_NULL, "value": ""}

    @pytest.mark.parametrize(("text", "word"), [("True", "true"), ("False", "false")])
    def test_repr_booleans(self, text: str, word: str) -> None:
        assert _mapped({"ok": ["repr", text]}) == {"kind": KIND_RETURNS, "type": TYPE_BOOLEAN, "value": word}

    @pytest.mark.parametrize("text", ["0", "42", "-7"])
    def test_repr_integers(self, text: str) -> None:
        assert _mapped({"ok": ["repr", text]}) == {"kind": KIND_RETURNS, "type": TYPE_INTEGER, "value": text}

    def test_an_exception_is_a_raise(self) -> None:
        assert _mapped({"exc": "ValueError"}) == {"kind": KIND_RAISES, "value": "ValueError"}

    def test_the_function_is_the_operation_and_both_sides_are_mapped(self) -> None:
        result = map_differential(_record(original={"ok": ["str", "a"]}, mutant={"exc": "KeyError"}), SIGNATURES)
        assert result.witness is not None
        assert result.witness["reference"] == {"kind": KIND_RETURNS, "type": TYPE_STRING, "value": "a"}
        assert result.witness["mutant"] == {"kind": KIND_RAISES, "value": "KeyError"}

    @pytest.mark.parametrize(
        "output",
        [
            {"ok": ["repr", "[1, 2]"]},
            {"ok": ["repr", "1.5"]},
            {"ok": ["repr", "'x'"]},
            {"ok": ["bytes", "x"]},
            {"ok": ["decimal", "not a number"]},
            {"ok": ["str"]},
            {"ok": ["str", 1]},
            {"ok": "str"},
            {},
            "ok",
        ],
    )
    def test_an_output_no_row_maps_leaves_the_witness_unmapped(self, output: object) -> None:
        for record in (_record(original=output), _record(mutant=output)):
            result = map_differential(record, SIGNATURES)
            assert result.witness is None
            assert result.unmapped_reason is not None


class TestRecordShape:
    @pytest.mark.parametrize("function", ["unknown_function", None, 3])
    def test_an_unknown_function_leaves_the_witness_unmapped(self, function: object) -> None:
        assert map_differential(_record(function=function), SIGNATURES).witness is None

    def test_a_record_without_arguments_leaves_the_witness_unmapped(self) -> None:
        record = _record()
        record["args"] = "not an object"
        assert map_differential(record, SIGNATURES).witness is None
