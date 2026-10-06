"""Stage traceability: the Shallnot report and the declared links D(t) (PREREG 2.6, 5)."""

from __future__ import annotations

from collections.abc import Mapping
from typing import Any, Protocol

from pilot.application.ports import RunStore
from pilot.domain.pipeline_constants import BINDINGS_FILENAME
from pilot.domain.constants import TRACEABILITY_FILENAME
from pilot.domain.traceability import nodeid_bindings, null_gate_failures, report_from_json


class RawTraceability(Protocol):
    def raw_report(self) -> dict[str, Any]: ...


class TraceabilityFailed(Exception):
    pass


def ensure_traceability(
    store: RunStore, source: RawTraceability, expected_tests: int, expected_criteria: int
) -> dict[str, str]:
    """Runs Shallnot once per run; returns {nodeid: declared ref}."""
    if store.exists(TRACEABILITY_FILENAME) and store.exists(BINDINGS_FILENAME):
        return dict(store.read_json(BINDINGS_FILENAME))
    report = source.raw_report()
    failures = null_gate_failures(report_from_json(report), expected_tests, expected_criteria)
    if failures:
        raise TraceabilityFailed("; ".join(failures))
    bindings = _single_bindings(nodeid_bindings(report))
    store.write_json(TRACEABILITY_FILENAME, report)
    store.write_json(BINDINGS_FILENAME, bindings)
    return bindings


def _single_bindings(bound: Mapping[str, tuple[str, ...]]) -> dict[str, str]:
    ambiguous = sorted(nodeid for nodeid, refs in bound.items() if len(refs) != 1)
    if ambiguous:
        raise TraceabilityFailed(f"tests without exactly one declared link: {ambiguous}")
    return {nodeid: refs[0] for nodeid, refs in sorted(bound.items())}
