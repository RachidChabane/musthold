"""The suite runs of PREREG 8.4: counts from a JUnit report or a `go test -json` stream, and the outcome of run 2."""

from __future__ import annotations

import json
from dataclasses import dataclass
from typing import Final
from xml.etree import ElementTree

from demos1.domain.ecosystem import Ecosystem
from demos1.domain.jsonvalue import JsonObject
from demos1.domain.outcomes import SetupOutcome

MIN_PASSING: Final = 12
PYTEST_RAN_STATUSES: Final = frozenset({0, 1})
NOT_PASSING_CHILDREN: Final = ("failure", "error", "skipped")
GO_PASS: Final = "pass"
GO_FAIL: Final = "fail"
GO_SKIP: Final = "skip"


@dataclass(frozen=True)
class SuiteCounts:
    passing: int
    failing: int
    erroring: int
    skipped: int
    readable: bool = True
    """False when the report could not be read at all."""

    def to_json(self) -> JsonObject:
        return {
            "passing": self.passing,
            "failing": self.failing,
            "erroring": self.erroring,
            "skipped": self.skipped,
            "readable": self.readable,
        }


UNREADABLE: Final = SuiteCounts(0, 0, 0, 0, readable=False)


def _local(tag: str) -> str:
    return tag.rsplit("}", 1)[-1]


def junit_counts(report: str | None) -> SuiteCounts:
    """A test case passes when it has no `failure`, `error` or `skipped` child."""
    if report is None:
        return UNREADABLE
    try:
        root = ElementTree.fromstring(report)
    except ElementTree.ParseError:
        return UNREADABLE
    passing = failing = erroring = skipped = 0
    for case in root.iter():
        if _local(case.tag) != "testcase":
            continue
        kinds = {_local(element.tag) for element in case}
        if "error" in kinds:
            erroring += 1
        elif "failure" in kinds:
            failing += 1
        elif "skipped" in kinds:
            skipped += 1
        else:
            passing += 1
    return SuiteCounts(passing, failing, erroring, skipped)


def _events(stream: str) -> list[dict[str, object]]:
    events: list[dict[str, object]] = []
    for line in stream.splitlines():
        if not line.strip():
            continue
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue
        if isinstance(event, dict):
            events.append(event)  # pyright: ignore[reportUnknownArgumentType]
    return events


@dataclass(frozen=True)
class GoSuite:
    counts: SuiteCounts
    named_pass: bool
    """Whether a `pass` event names a test."""


def go_counts(stream: str | None) -> GoSuite:
    """A passing Go test is a distinct (Package, Test) pair with a pass event, subtests included.

    Failing and skipped tests count the same way; an erroring package is one whose package-level `fail`
    event comes with no failing test of its own, such as a package that does not build.
    """
    if stream is None:
        return GoSuite(UNREADABLE, named_pass=False)
    outcomes: dict[str, dict[tuple[str, str], str]] = {GO_PASS: {}, GO_FAIL: {}, GO_SKIP: {}}
    failed_packages: set[str] = set()
    for event in _events(stream):
        action, package, test = event.get("Action"), event.get("Package"), event.get("Test")
        if not isinstance(action, str) or not isinstance(package, str):
            continue
        if isinstance(test, str) and test and action in outcomes:
            outcomes[action][(package, test)] = action
        elif action == GO_FAIL and not test:
            failed_packages.add(package)
    packages_with_failing_tests = {package for package, _ in outcomes[GO_FAIL]}
    counts = SuiteCounts(
        passing=len(outcomes[GO_PASS]),
        failing=len(outcomes[GO_FAIL]),
        erroring=len(failed_packages - packages_with_failing_tests),
        skipped=len(outcomes[GO_SKIP]),
    )
    return GoSuite(counts, named_pass=bool(outcomes[GO_PASS]))


@dataclass(frozen=True)
class SecondRun:
    exit_status: int
    timed_out: bool
    counts: SuiteCounts
    named_pass: bool


def second_run_outcome(ecosystem: Ecosystem, run: SecondRun) -> SetupOutcome | None:
    """The outcome of the second run, or None when the attempt passes the suite stage."""
    if run.timed_out:
        return SetupOutcome.SUITE_TOO_SLOW
    if ecosystem is Ecosystem.PYTEST and run.exit_status not in PYTEST_RAN_STATUSES:
        return SetupOutcome.SUITE_DOES_NOT_RUN
    if ecosystem is Ecosystem.GO and not run.named_pass:
        return SetupOutcome.SUITE_DOES_NOT_RUN
    if run.counts.passing < MIN_PASSING:
        return SetupOutcome.TOO_FEW_PASSING
    return None
