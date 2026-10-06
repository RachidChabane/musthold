"""Witnesses of a validity verdict: one concrete call, executed on both trees (PREREG 6.5)."""

from __future__ import annotations

from collections.abc import Mapping
from dataclasses import dataclass
from decimal import Decimal, InvalidOperation
from enum import Enum
from typing import Any

from pilot.domain.differential_text import (
    DECIMAL_TAG,
    OUTPUT_EXCEPTION_KEY,
    OUTPUT_OK_KEY,
)

QUOTE_FUNCTION = "quote"
RECEIPT_FUNCTION = "format_receipt"
TYPE_DATETIME = "datetime"
TYPE_BOOL = "bool"
TYPE_DECIMAL = "Decimal"


class WitnessInvalid(Exception):
    """The witness names no executable call."""


class OutputKind(Enum):
    RETURNS = "RETURNS"
    RAISES = "RAISES"


@dataclass(frozen=True)
class Output:
    kind: OutputKind
    value: str

    def to_mapping(self) -> dict[str, str]:
        return {"kind": self.kind.value, "value": self.value}


@dataclass(frozen=True)
class WitnessCall:
    """The call in driver form: parameter types and JSON-compatible raw arguments."""

    function: str
    types: Mapping[str, str]
    arguments: Mapping[str, Any]


@dataclass(frozen=True)
class Witness:
    function: str
    entry: str | None
    exit: str | None
    lost_ticket: bool | None
    validated: bool | None
    amount: str | None
    expected: Output

    @classmethod
    def from_payload(cls, raw: Mapping[str, Any]) -> Witness:
        expected = raw["expected"]
        return cls(
            function=raw["function"],
            entry=raw.get("entry"),
            exit=raw.get("exit"),
            lost_ticket=raw.get("lost_ticket"),
            validated=raw.get("validated"),
            amount=raw.get("amount"),
            expected=Output(OutputKind(expected["kind"]), expected["value"]),
        )

    def call(self) -> WitnessCall:
        if self.function == QUOTE_FUNCTION:
            return self._quote_call()
        if self.function == RECEIPT_FUNCTION:
            return self._receipt_call()
        raise WitnessInvalid(f"unknown function {self.function!r}")

    def _quote_call(self) -> WitnessCall:
        if self.entry is None or self.exit is None:
            raise WitnessInvalid("quote needs entry and exit")
        types = {"entry": TYPE_DATETIME, "exit": TYPE_DATETIME}
        arguments: dict[str, Any] = {"entry": self.entry, "exit": self.exit}
        for name, value in (("lost_ticket", self.lost_ticket), ("validated", self.validated)):
            if value is not None:
                types[name] = TYPE_BOOL
                arguments[name] = value
        return WitnessCall(QUOTE_FUNCTION, types, arguments)

    def _receipt_call(self) -> WitnessCall:
        if self.amount is None:
            raise WitnessInvalid("format_receipt needs amount")
        return WitnessCall(RECEIPT_FUNCTION, {"amount": TYPE_DECIMAL}, {"amount": self.amount})


def output_from_driver(raw: Mapping[str, Any]) -> Output:
    """Output recorded by the evaluation driver: `{ok: [tag, text]}` or `{exc: name}`."""
    if OUTPUT_EXCEPTION_KEY in raw:
        return Output(OutputKind.RAISES, str(raw[OUTPUT_EXCEPTION_KEY]))
    _tag, text = raw[OUTPUT_OK_KEY]
    return Output(OutputKind.RETURNS, str(text))


def outputs_equal(function: str, left: Output, right: Output) -> bool:
    if left.kind is not right.kind:
        return False
    if left.kind is OutputKind.RAISES or function != QUOTE_FUNCTION:
        return left.value == right.value
    try:
        return Decimal(left.value) == Decimal(right.value)
    except InvalidOperation:
        return False


def is_confirmed(function: str, expected: Output, reference: Output, mutant: Output) -> bool:
    """The reference gives the expected output and the mutant differs or raises."""
    return outputs_equal(function, expected, reference) and not outputs_equal(function, reference, mutant)


__all__ = [
    "DECIMAL_TAG",
    "Output",
    "OutputKind",
    "Witness",
    "WitnessCall",
    "WitnessInvalid",
    "is_confirmed",
    "output_from_driver",
    "outputs_equal",
]
