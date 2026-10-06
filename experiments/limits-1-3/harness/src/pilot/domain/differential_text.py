"""The `{differential}` sentence of the validity prompt (PREREG 4.5)."""

from __future__ import annotations

from collections.abc import Mapping
from typing import Any

from pilot.domain.mutants import DifferentialKind, DifferentialResult

OUTPUT_OK_KEY = "ok"
OUTPUT_EXCEPTION_KEY = "exc"
DECIMAL_TAG = "decimal"
STRING_TAG = "str"


def render_output(output: Any) -> str:
    """A recorded call output (`{ok: [tag, text]}` or `{exc: name}`) as prose."""
    if isinstance(output, Mapping) and OUTPUT_EXCEPTION_KEY in output:
        return f"raised {output[OUTPUT_EXCEPTION_KEY]}"
    if isinstance(output, Mapping) and OUTPUT_OK_KEY in output:
        tag, text = output[OUTPUT_OK_KEY]
        return repr(text) if tag == STRING_TAG else str(text)
    return str(output)


def render_arguments(args: Mapping[str, Any]) -> str:
    return ", ".join(f"{name}={value!r}" for name, value in args.items())


def differential_sentence(result: DifferentialResult | None) -> str:
    if result is None or result.kind is not DifferentialKind.DIFFERS:
        return ""
    return (
        "An automated comparison found this distinguishing call: "
        f"{result.function}({render_arguments(result.args)}) returned "
        f"{render_output(result.original_out)} before the change and "
        f"{render_output(result.mutant_out)} after it."
    )
