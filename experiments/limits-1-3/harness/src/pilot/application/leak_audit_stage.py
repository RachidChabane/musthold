"""Pre-registration leak audit by a cross-family model (PREREG 3.3)."""

from __future__ import annotations

from collections.abc import Callable, Mapping
from typing import Any

from pilot.application.model_caller import CallSpec, ModelCaller
from pilot.domain.call_type import CallType
from pilot.domain.model_call import MissingResult
from pilot.domain.model_slate import TERRA
from pilot.domain.prompts import leak_audit_item

CALL_ID = "leak_audit"


class LeakAuditMissing(Exception):
    pass


def run_leak_audit(
    caller: ModelCaller,
    template: str,
    schema_text: str,
    files: Mapping[str, str],
    write_hints: Callable[[Any], None],
) -> int:
    """One isolated call over every fixture file; writes the hint payload and returns the hint count."""
    spec = CallSpec(
        call_id=CALL_ID,
        pin=TERRA,
        call_type=CallType.LEAK_AUDIT,
        item=leak_audit_item(template, files),
        schema_text=schema_text,
    )
    outcome = caller.call_all("leak-audit", [spec])[CALL_ID]
    if isinstance(outcome, MissingResult):
        raise LeakAuditMissing(f"{outcome.failures} {outcome.isolation_violations}")
    write_hints(outcome.payload)
    return len(outcome.payload["hints"])

