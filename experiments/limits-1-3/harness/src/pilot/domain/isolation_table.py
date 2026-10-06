"""The isolation table of the hand-written violations: which ones a test bound to another criterion kills.

Computed from the committed label matrix and the Shallnot bindings, with the same predicate
that decides VALID_x. Ground truth: read only by `isolation-table` and the analysis.
"""

from __future__ import annotations

from collections.abc import Iterable, Mapping
from dataclasses import dataclass
from typing import Any, Final

from pilot.domain.constants import CELL_FAIL
from pilot.domain.execution_validation import other_binding_veto
from pilot.domain.outcomes import KillResult
from pilot.domain.refs import criterion_ref
from pilot.domain.variants import VariantId, VariantKind

ISOLATION_TABLE_FILENAME: Final = "isolation_table.json"

REGRESSION_LETTERS: Final = ("_a", "_b")


@dataclass(frozen=True)
class IsolationTable:
    """Which hand-written violations a test bound to another criterion kills, from committed truth."""

    violations: Mapping[str, int]
    killed_by_other: frozenset[str]
    criteria: tuple[int, ...]

    def surviving(self, number: int) -> tuple[str, ...]:
        return tuple(
            name for name, owner in sorted(self.violations.items()) if owner == number and name not in self.killed_by_other
        )

    @property
    def d8_criteria(self) -> tuple[int, ...]:
        """C_D8: the criteria with at least one violation that no other binding's test kills."""
        return tuple(number for number in self.criteria if self.surviving(number))

    @property
    def regression_items(self) -> tuple[str, ...]:
        return tuple(sorted(name for name in self.violations if name.endswith(REGRESSION_LETTERS)))

    def to_mapping(self) -> dict[str, Any]:
        surviving_items = [name for name in self.regression_items if name not in self.killed_by_other]
        return {
            "c_d8": list(self.d8_criteria),
            "killed_by_other_binding": sorted(self.killed_by_other),
            "surviving_per_criterion": {str(number): len(self.surviving(number)) for number in self.criteria},
            "surviving_regression_items": len(surviving_items),
            "regression_items": len(self.regression_items),
        }


def isolation_table(
    matrix_rows: Mapping[str, Mapping[str, str]], bindings: Mapping[str, str], criteria: Iterable[int]
) -> IsolationTable:
    """`matrix_rows` is nodeid -> variant -> pass or fail (label_matrix.csv); `bindings` is nodeid -> `PARK-n~r`."""
    results: dict[str, dict[str, KillResult]] = {}
    violations: dict[str, int] = {}
    for nodeid, cells in matrix_rows.items():
        for variant, cell in cells.items():
            parsed = VariantId.parse_or_none(variant)
            if parsed is None or parsed.kind is not VariantKind.VIOLATION or parsed.criterion is None:
                continue
            violations[variant] = parsed.criterion
            results.setdefault(variant, {})[nodeid] = KillResult.KILLED if cell == CELL_FAIL else KillResult.SURVIVED
    killed_by_other = frozenset(
        variant
        for variant, number in violations.items()
        if other_binding_veto(results[variant], bindings, criterion_ref(number))
    )
    return IsolationTable(violations, killed_by_other, tuple(sorted(criteria)))
