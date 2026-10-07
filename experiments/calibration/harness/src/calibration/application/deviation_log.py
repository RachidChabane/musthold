"""The deviation log of a run: `DEVIATIONS.md`, appended one line at a time (§14.2)."""

from __future__ import annotations

from collections.abc import Iterable

from calibration.application.ports import RunStore
from calibration.domain.constants import DEVIATIONS_FILE
from calibration.domain.deviations import (
    DEVIATIONS_TITLE,
    OPERATOR_KINDS,
    DeviationEntry,
    DeviationKind,
    InvalidDeviation,
    parse_deviations,
)


class DeviationLog:
    """Entries are appended, never rewritten; the file starts with its title."""

    def __init__(self, store: RunStore) -> None:
        self._store = store

    def entries(self) -> list[DeviationEntry]:
        if not self._store.exists(DEVIATIONS_FILE):
            return []
        return parse_deviations(self._store.read_text(DEVIATIONS_FILE))

    def record(self, kind: DeviationKind, scope: Iterable[str], text: str) -> DeviationEntry:
        entry = DeviationEntry.create(kind, scope, text)
        if not self._store.exists(DEVIATIONS_FILE):
            self._store.write_text(DEVIATIONS_FILE, DEVIATIONS_TITLE + "\n\n")
        self._store.append_text(DEVIATIONS_FILE, entry.render() + "\n")
        return entry

    def record_operator(self, kind: DeviationKind, scope: Iterable[str], text: str) -> DeviationEntry:
        """The entries `log-deviation` writes: only `blocked` and `protocol` (§14.2)."""
        if kind not in OPERATOR_KINDS:
            raise InvalidDeviation(f"an operator logs only {[k.value for k in OPERATOR_KINDS]}, not {kind.value}")
        return self.record(kind, scope, text)
