"""Traceability report as the harness consumes it (PREREG 2.6 and 5)."""

from __future__ import annotations

from collections.abc import Mapping
from dataclasses import dataclass

from pilot.domain.constants import (
    FORBIDDEN_TRACEABILITY_FINDINGS,
    TRACEABILITY_PASS_VERDICT,
)


@dataclass(frozen=True)
class TraceabilityReport:
    verdict: str
    requirements_in_focus: int
    requirements_covered: int
    tests_total: int
    tests_bound: int
    finding_categories: tuple[str, ...]
    bindings: Mapping[str, tuple[str, ...]]


def null_gate_failures(
    report: TraceabilityReport, expected_tests: int, expected_criteria: int
) -> list[str]:
    """Precondition of the null gate G0 (PREREG 2.6); empty when it holds."""
    failures = []
    if report.tests_bound != expected_tests or report.tests_total != expected_tests:
        failures.append(
            f"tests bound {report.tests_bound}/{report.tests_total}, expected {expected_tests}/{expected_tests}"
        )
    if report.requirements_covered != expected_criteria or report.requirements_in_focus != expected_criteria:
        failures.append(
            f"criteria covered {report.requirements_covered}/{report.requirements_in_focus}, "
            f"expected {expected_criteria}/{expected_criteria}"
        )
    for category in sorted(set(report.finding_categories) & set(FORBIDDEN_TRACEABILITY_FINDINGS)):
        failures.append(f"finding {category}")
    if report.verdict != TRACEABILITY_PASS_VERDICT:
        failures.append(f"verdict {report.verdict}")
    return failures
