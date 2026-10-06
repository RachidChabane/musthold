"""Outcome matrix: per variant, per node id, one test outcome."""

from __future__ import annotations

import re
from collections.abc import Mapping
from dataclasses import dataclass

from pilot.domain.ground_truth_paths import ORACLE_FILE_PATTERN
from pilot.domain.outcomes import TestOutcome

_ORACLE_PATTERN = re.compile(ORACLE_FILE_PATTERN)


def oracle_criterion(nodeid: str) -> int | None:
    """Criterion number of an oracle node id, None for any other test."""
    file_part = nodeid.split("::", 1)[0]
    match = _ORACLE_PATTERN.search(file_part)
    return int(match.group(1)) if match else None


@dataclass(frozen=True)
class VariantOutcomes:
    """All outcomes of one variant run, project tests and oracle tests together."""

    outcomes: Mapping[str, TestOutcome]

    def of(self, nodeid: str) -> TestOutcome:
        return self.outcomes.get(nodeid, TestOutcome.ERROR)

    def project_nodeids(self) -> list[str]:
        return sorted(n for n in self.outcomes if oracle_criterion(n) is None)

    def oracle_passed(self, criterion: int) -> bool:
        nodeids = [n for n in self.outcomes if oracle_criterion(n) == criterion]
        return bool(nodeids) and all(self.outcomes[n].is_pass for n in nodeids)

    def oracle_criteria(self) -> list[int]:
        found = {oracle_criterion(n) for n in self.outcomes}
        return sorted(c for c in found if c is not None)

    def anomalies(self) -> list[str]:
        return sorted(
            f"{nodeid}={outcome.value}"
            for nodeid, outcome in self.outcomes.items()
            if outcome in (TestOutcome.ERROR, TestOutcome.SKIPPED)
        )
