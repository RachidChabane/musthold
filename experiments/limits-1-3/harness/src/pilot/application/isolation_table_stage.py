"""Use case `isolation-table`: C_D8 from the committed label matrix and the Shallnot bindings (ADR 0006)."""

from __future__ import annotations

from collections.abc import Mapping

from pilot.application.traceability_stage import RawTraceability, TraceabilityFailed
from pilot.domain.isolation_table import IsolationTable, isolation_table
from pilot.domain.label_matrix import LabelMatrix
from pilot.domain.traceability import nodeid_bindings, null_gate_failures, report_from_json


def compute_isolation_table(
    traceability: RawTraceability, matrix: LabelMatrix, expected_tests: int, expected_criteria: int
) -> IsolationTable:
    """Runs Shallnot once; the table is refused when traceability does not pass or a test has no single link."""
    report = traceability.raw_report()
    failures = null_gate_failures(report_from_json(report), expected_tests, expected_criteria)
    if failures:
        raise TraceabilityFailed("; ".join(failures))
    bindings = _single_bindings(nodeid_bindings(report))
    rows = {nodeid: dict(zip(matrix.variant_ids, cells, strict=True)) for nodeid, cells in matrix.rows.items()}
    return isolation_table(rows, bindings, range(1, expected_criteria + 1))


def _single_bindings(bound: Mapping[str, tuple[str, ...]]) -> dict[str, str]:
    ambiguous = sorted(nodeid for nodeid, refs in bound.items() if len(refs) != 1)
    if ambiguous:
        raise TraceabilityFailed(f"tests without exactly one declared link: {ambiguous}")
    return {nodeid: refs[0] for nodeid, refs in bound.items()}
