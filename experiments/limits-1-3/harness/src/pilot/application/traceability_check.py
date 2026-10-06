"""Use case `traceability-check`: the null-gate G0 precondition (PREREG 2.6)."""

from __future__ import annotations

from pilot.application.ports import TraceabilityPort
from pilot.domain.traceability import null_gate_failures


def traceability_check(
    port: TraceabilityPort, expected_tests: int, expected_criteria: int
) -> list[str]:
    return null_gate_failures(port.check(), expected_tests, expected_criteria)
