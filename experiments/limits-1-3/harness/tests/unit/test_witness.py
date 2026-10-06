from __future__ import annotations

import pytest

from pilot.domain.witness import (
    Output,
    OutputKind,
    Witness,
    WitnessInvalid,
    is_confirmed,
    output_from_driver,
    outputs_equal,
)
from pilot.domain.validity_payload import parse_validity, validity_problems

QUOTE = Witness("quote", "2026-03-02T09:00", "2026-03-02T09:16", False, None, None, Output(OutputKind.RETURNS, "2.50"))
RECEIPT = Witness("format_receipt", None, None, None, None, "12.5", Output(OutputKind.RETURNS, "EUR 12.50"))


def test_quote_witness_converts_by_type_and_omits_null_flags():
    call = QUOTE.call()
    assert call.function == "quote"
    assert call.types == {"entry": "datetime", "exit": "datetime", "lost_ticket": "bool"}
    assert call.arguments == {"entry": "2026-03-02T09:00", "exit": "2026-03-02T09:16", "lost_ticket": False}


def test_receipt_witness_passes_the_amount_as_a_decimal_string():
    call = RECEIPT.call()
    assert call.types == {"amount": "Decimal"} and call.arguments == {"amount": "12.5"}


@pytest.mark.parametrize(
    "witness",
    [
        Witness("quote", None, "2026-03-02T09:16", None, None, None, Output(OutputKind.RAISES, "ValueError")),
        Witness("format_receipt", None, None, None, None, None, Output(OutputKind.RETURNS, "x")),
        Witness("eval", None, None, None, None, None, Output(OutputKind.RETURNS, "x")),
    ],
)
def test_witnesses_that_name_no_executable_call_are_invalid(witness):
    with pytest.raises(WitnessInvalid):
        witness.call()


def test_quote_values_compare_as_decimals_and_receipts_as_strings():
    assert outputs_equal("quote", Output(OutputKind.RETURNS, "2.50"), Output(OutputKind.RETURNS, "2.5E+0"))
    assert not outputs_equal("quote", Output(OutputKind.RETURNS, "2.50"), Output(OutputKind.RETURNS, "2.51"))
    assert not outputs_equal("format_receipt", Output(OutputKind.RETURNS, "EUR 2.50"), Output(OutputKind.RETURNS, "EUR 2.5"))
    assert not outputs_equal("quote", Output(OutputKind.RETURNS, "x"), Output(OutputKind.RETURNS, "x1"))
    assert outputs_equal("quote", Output(OutputKind.RAISES, "ValueError"), Output(OutputKind.RAISES, "ValueError"))
    assert not outputs_equal("quote", Output(OutputKind.RAISES, "ValueError"), Output(OutputKind.RETURNS, "0"))


def test_driver_output_maps_to_returns_or_raises():
    assert output_from_driver({"ok": ["decimal", "5"]}) == Output(OutputKind.RETURNS, "5")
    assert output_from_driver({"exc": "ValueError"}) == Output(OutputKind.RAISES, "ValueError")


def test_confirmed_needs_the_reference_to_match_and_the_mutant_to_differ_or_raise():
    expected = Output(OutputKind.RETURNS, "2.50")
    assert is_confirmed("quote", expected, Output(OutputKind.RETURNS, "2.50"), Output(OutputKind.RETURNS, "2.00"))
    assert is_confirmed("quote", expected, Output(OutputKind.RETURNS, "2.50"), Output(OutputKind.RAISES, "TypeError"))
    assert not is_confirmed("quote", expected, Output(OutputKind.RETURNS, "2.50"), Output(OutputKind.RETURNS, "2.5"))
    assert not is_confirmed("quote", expected, Output(OutputKind.RETURNS, "9.99"), Output(OutputKind.RETURNS, "2.00"))


def verdict_payload(verdict="VIOLATED", witness=None, confidence=0.9):
    return {
        "equivalent": {"value": False, "confidence": 0.8},
        "requirements": {"PARK-1~1": {"verdict": verdict, "confidence": confidence, "witness": witness}},
    }


WITNESS_PAYLOAD = {
    "function": "quote", "entry": "2026-03-02T09:00", "exit": "2026-03-02T09:16", "lost_ticket": False,
    "validated": False, "amount": None, "expected": {"kind": "RETURNS", "value": "2.50"},
}


def test_validity_payload_parses_witnesses_of_violated_verdicts():
    parsed = parse_validity(verdict_payload(witness=WITNESS_PAYLOAD))
    assert parsed.witnesses()["PARK-1~1"].function == "quote" and not parsed.equivalent


def test_semantic_checks_require_a_witness_for_violated_and_confidences_in_range():
    assert validity_problems(verdict_payload(witness=WITNESS_PAYLOAD)) == []
    assert validity_problems(verdict_payload()) == ["PARK-1~1: VIOLATED without a witness"]
    assert validity_problems(verdict_payload("NOT_VIOLATED", confidence=1.5)) == ["PARK-1~1: confidence outside [0, 1]"]
