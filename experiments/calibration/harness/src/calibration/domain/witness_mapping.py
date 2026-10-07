"""The pilot's F3 differential record as a discpower witness (§4.3): the call and what each side did.

A value that no row of the table maps makes the whole witness unmapped; the request then carries no witness and the
mutant is counted as `witness_unmapped`.
"""

from __future__ import annotations

import ast
import re
from collections.abc import Mapping
from dataclasses import dataclass
from decimal import Decimal, InvalidOperation
from typing import Final

from calibration.domain.canonical import JsonValue
from calibration.domain.jsonread import as_list, as_mapping

DATETIME_ANNOTATION: Final = "datetime"
DECIMAL_ANNOTATION: Final = "Decimal"
BOOL_ANNOTATION: Final = "bool"
TYPE_DATETIME: Final = "datetime"
TYPE_DECIMAL: Final = "decimal"
TYPE_BOOLEAN: Final = "boolean"
TYPE_STRING: Final = "string"
TYPE_INTEGER: Final = "integer"
TYPE_NULL: Final = "null"
KIND_RETURNS: Final = "returns"
KIND_RAISES: Final = "raises"
OK_KEY: Final = "ok"
EXC_KEY: Final = "exc"
TAG_DECIMAL: Final = "decimal"
TAG_STRING: Final = "str"
TAG_REPR: Final = "repr"
REPR_NONE: Final = "None"
REPR_TRUE: Final = "True"
REPR_FALSE: Final = "False"
POSITIONAL_FORMAT: Final = "f"
_INTEGER_LITERAL: Final = re.compile(r"^-?\d+$")
PAIR_LENGTH: Final = 2

Signatures = Mapping[str, Mapping[str, str]]


@dataclass(frozen=True)
class WitnessMapping:
    """`witness` is the discpower `power.Witness` JSON, or None with the reason the record maps to none."""

    witness: dict[str, JsonValue] | None
    unmapped_reason: str | None


def function_signatures(sources: Mapping[str, str]) -> dict[str, dict[str, str]]:
    """Parameter name to annotation text of every module-level function, from `ast`."""
    found: dict[str, dict[str, str]] = {}
    for source in sources.values():
        for node in ast.parse(source).body:
            if isinstance(node, ast.FunctionDef | ast.AsyncFunctionDef):
                arguments = node.args
                found[node.name] = {
                    argument.arg: ast.unparse(argument.annotation) if argument.annotation else ""
                    for argument in (*arguments.posonlyargs, *arguments.args, *arguments.kwonlyargs)
                }
    return found


def _literal(annotation: str, value: object) -> dict[str, JsonValue] | None:
    if annotation == DATETIME_ANNOTATION and isinstance(value, str):
        return {"type": TYPE_DATETIME, "value": value}
    if annotation == DECIMAL_ANNOTATION and isinstance(value, str):
        return {"type": TYPE_DECIMAL, "value": value}
    if annotation == BOOL_ANNOTATION and isinstance(value, bool):
        return {"type": TYPE_BOOLEAN, "value": "true" if value else "false"}
    return None


def _ok_pair(recorded: Mapping[str, object]) -> tuple[str, str] | None:
    pair = as_list(recorded.get(OK_KEY))
    if pair is None or len(pair) != PAIR_LENGTH or not all(isinstance(item, str) for item in pair):
        return None
    return str(pair[0]), str(pair[1])


def _observation(output: object) -> dict[str, JsonValue] | None:
    recorded = as_mapping(output)
    if recorded is None:
        return None
    if isinstance(recorded.get(EXC_KEY), str):
        return {"kind": KIND_RAISES, "value": str(recorded[EXC_KEY])}
    pair = _ok_pair(recorded)
    if pair is None:
        return None
    tag, text = pair
    if tag == TAG_STRING:
        return {"kind": KIND_RETURNS, "type": TYPE_STRING, "value": text}
    if tag == TAG_DECIMAL:
        return _decimal_observation(text)
    return _repr_observation(text) if tag == TAG_REPR else None


def _decimal_observation(text: str) -> dict[str, JsonValue] | None:
    try:
        positional = format(Decimal(text), POSITIONAL_FORMAT)
    except InvalidOperation:
        return None
    return {"kind": KIND_RETURNS, "type": TYPE_DECIMAL, "value": positional}


def _repr_observation(text: str) -> dict[str, JsonValue] | None:
    if text == REPR_NONE:
        return {"kind": KIND_RETURNS, "type": TYPE_NULL, "value": ""}
    if text in (REPR_TRUE, REPR_FALSE):
        return {"kind": KIND_RETURNS, "type": TYPE_BOOLEAN, "value": "true" if text == REPR_TRUE else "false"}
    if _INTEGER_LITERAL.match(text):
        return {"kind": KIND_RETURNS, "type": TYPE_INTEGER, "value": text}
    return None


def map_differential(record: Mapping[str, object], signatures: Signatures) -> WitnessMapping:
    """Maps one `differential.jsonl` record whose status is DIFFERS."""
    function = record.get("function")
    arguments = as_mapping(record.get("args"))
    if not isinstance(function, str) or function not in signatures or arguments is None:
        return WitnessMapping(None, "the differential names no known function or no arguments")
    parameters = signatures[function]
    args: dict[str, JsonValue] = {}
    for name, value in arguments.items():
        literal = _literal(parameters.get(name, ""), value)
        if literal is None:
            return WitnessMapping(None, f"argument {name} of {function} has no literal type")
        args[name] = literal
    reference = _observation(record.get("original_out"))
    mutant = _observation(record.get("mutant_out"))
    if reference is None or mutant is None:
        return WitnessMapping(None, "an output of the differential maps to no observation")
    return WitnessMapping(
        {"call": {"operation": function, "args": args}, "reference": reference, "mutant": mutant}, None
    )
