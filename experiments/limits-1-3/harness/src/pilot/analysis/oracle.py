"""Oracle classification of a mutant from the oracle suites that failed on it (PREREG 6.6)."""

from __future__ import annotations

from dataclasses import dataclass
from enum import Enum

from pilot.domain.analysis_constants import DIFF_DIFFERS, DIFF_NO_DIFF


class OracleKind(Enum):
    VALID = "oracle-valid"
    COLLATERAL = "oracle-collateral"
    SILENT_EQUIVALENT_PROBABLE = "silent-equivalent-probable"
    SILENT_DIFFERENT = "silent-different"
    SILENT_UNTESTED = "silent-untested"

    @property
    def is_silent(self) -> bool:
        return self in (
            OracleKind.SILENT_EQUIVALENT_PROBABLE,
            OracleKind.SILENT_DIFFERENT,
            OracleKind.SILENT_UNTESTED,
        )


@dataclass(frozen=True)
class OracleClass:
    kind: OracleKind
    criteria: tuple[int, ...]

    def valid_for(self, criterion: int | None) -> bool:
        return self.kind is OracleKind.VALID and criterion is not None and self.criteria == (criterion,)


def classify_oracle(failed_criteria: tuple[int, ...], differential_status: str | None) -> OracleClass:
    criteria = tuple(sorted(set(failed_criteria)))
    if len(criteria) == 1:
        return OracleClass(OracleKind.VALID, criteria)
    if criteria:
        return OracleClass(OracleKind.COLLATERAL, criteria)
    if differential_status == DIFF_NO_DIFF:
        return OracleClass(OracleKind.SILENT_EQUIVALENT_PROBABLE, ())
    if differential_status == DIFF_DIFFERS:
        return OracleClass(OracleKind.SILENT_DIFFERENT, ())
    return OracleClass(OracleKind.SILENT_UNTESTED, ())
