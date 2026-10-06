"""The label matrix: tests x variants, pass or fail (PREREG 3.1)."""

from __future__ import annotations

from collections.abc import Iterable, Mapping, Sequence
from dataclasses import dataclass

from pilot.domain.outcome_matrix import VariantOutcomes
from pilot.domain.variants import VariantId, VariantKind


@dataclass(frozen=True)
class LabelMatrix:
    variant_ids: tuple[str, ...]
    rows: Mapping[str, tuple[str, ...]]

    @classmethod
    def from_runs(
        cls, nodeids: Iterable[str], runs: Mapping[str, VariantOutcomes]
    ) -> LabelMatrix:
        columns = column_order(runs)
        rows = {
            nodeid: tuple(runs[column].of(nodeid).cell for column in columns)
            for nodeid in sorted(nodeids)
        }
        return cls(tuple(columns), rows)

    def cell(self, nodeid: str, variant_id: str) -> str | None:
        row = self.rows.get(nodeid)
        if row is None or variant_id not in self.variant_ids:
            return None
        return row[self.variant_ids.index(variant_id)]

    def differences(self, other: LabelMatrix) -> list[str]:
        """Human-readable differences from `other`; empty when the matrices are identical."""
        found = _set_differences("variant", self.variant_ids, other.variant_ids)
        found += _set_differences("test", tuple(self.rows), tuple(other.rows))
        shared_variants = [v for v in self.variant_ids if v in other.variant_ids]
        for nodeid in sorted(set(self.rows) & set(other.rows)):
            for variant in shared_variants:
                ours, theirs = self.cell(nodeid, variant), other.cell(nodeid, variant)
                if ours != theirs:
                    found.append(f"{nodeid} x {variant}: {ours} != {theirs}")
        return found


def column_order(runs: Mapping[str, VariantOutcomes]) -> list[str]:
    """Violations by criterion and letter, then the refactor; planted items and the reference excluded."""
    parsed = [v for v in map(VariantId.parse_or_none, runs) if v is not None]
    wanted = [v for v in parsed if v.kind in (VariantKind.VIOLATION, VariantKind.REFACTOR)]
    return [v.value for v in sorted(wanted, key=lambda v: v.sort_key)]


def _set_differences(kind: str, ours: Sequence[str], theirs: Sequence[str]) -> list[str]:
    return [f"{kind} only in first: {name}" for name in sorted(set(ours) - set(theirs))] + [
        f"{kind} only in second: {name}" for name in sorted(set(theirs) - set(ours))
    ]
