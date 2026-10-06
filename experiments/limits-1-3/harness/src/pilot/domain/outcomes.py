"""Per-test outcomes and kill classification (PREREG 6.4 F2)."""

from __future__ import annotations

from collections.abc import Mapping
from dataclasses import dataclass
from enum import Enum

from pilot.domain.constants import CELL_FAIL, CELL_PASS


class TestOutcome(Enum):
    __test__ = False

    PASSED = "passed"
    FAILED = "failed"
    ERROR = "error"
    SKIPPED = "skipped"

    @property
    def is_pass(self) -> bool:
        return self is TestOutcome.PASSED

    @property
    def cell(self) -> str:
        return CELL_PASS if self.is_pass else CELL_FAIL


class KillResult(Enum):
    KILLED = "killed"
    SURVIVED = "survived"
    ERROR = "error"
    FLAKY = "flaky"

    @property
    def counts_as_survived(self) -> bool:
        return self in (KillResult.SURVIVED, KillResult.FLAKY)


def classify_kill(first: TestOutcome, rerun: TestOutcome | None) -> KillResult:
    """Classify one test on one mutant; `rerun` is the second run of a first-run kill."""
    if first is TestOutcome.ERROR:
        return KillResult.ERROR
    if first is not TestOutcome.FAILED:
        return KillResult.SURVIVED
    if rerun is TestOutcome.FAILED:
        return KillResult.KILLED
    return KillResult.FLAKY


@dataclass(frozen=True)
class RunResult:
    """Outcomes of one pytest invocation, keyed by node id."""

    outcomes: Mapping[str, TestOutcome]
    returncode: int
    deadline_exceeded: bool = False

    def outcome_of(self, nodeid: str) -> TestOutcome:
        return self.outcomes.get(nodeid, TestOutcome.ERROR)

    @property
    def all_passed(self) -> bool:
        return bool(self.outcomes) and all(o.is_pass for o in self.outcomes.values())
