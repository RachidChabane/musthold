"""What a case page reads from a Shallnot report: the verdict, the criterion, the tests that cite it and the findings."""

from __future__ import annotations

from collections.abc import Mapping
from dataclasses import dataclass
from typing import Final, cast

from specbench.domain.jsonread import RecordError, text

PASSED: Final = "passed"
NOT_RECORDED: Final = "not recorded"
REVISION_SEPARATOR: Final = "~"


@dataclass(frozen=True)
class CitingTest:
    name: str
    classname: str
    outcome: str


@dataclass(frozen=True)
class Finding:
    category: str
    module: str
    message: str
    blocking: bool


@dataclass(frozen=True)
class TestRun:
    total: int
    passed: int


@dataclass(frozen=True)
class GateReading:
    verdict: str
    statement: str
    tests: tuple[CitingTest, ...]
    findings: tuple[Finding, ...]
    run: TestRun


def _items(value: object) -> list[Mapping[str, object]]:
    if not isinstance(value, list):
        return []
    return [cast(Mapping[str, object], item) for item in cast(list[object], value) if isinstance(item, Mapping)]


def _string(value: object, default: str = "") -> str:
    return value if isinstance(value, str) else default


def _field(value: object, name: str) -> object:
    """`value[name]` when `value` is an object, else `None`."""
    return cast(Mapping[str, object], value).get(name) if isinstance(value, Mapping) else None


def _test_run(report: Mapping[str, object]) -> TestRun:
    seen: dict[tuple[str, str], str] = {}
    for requirement in _items(report.get("requirements")):
        for test in _items(requirement.get("tests")):
            key = (_string(test.get("classname")), _string(test.get("name")))
            seen[key] = _string(test.get("outcome"), NOT_RECORDED)
    total = _field(_field(_field(report, "summary"), "tests"), "total")
    counted = total if isinstance(total, int) and not isinstance(total, bool) else len(seen)
    return TestRun(counted, sum(1 for outcome in seen.values() if outcome == PASSED))


def read_gate_report(report: Mapping[str, object], criterion: str) -> GateReading:
    """The reading of `report` for one criterion (`ID~REV`); a report that does not declare it is refused."""
    requirement = next((r for r in _items(report.get("requirements")) if r.get("ref") == criterion), None)
    if requirement is None:
        raise RecordError(f"the gate report declares no requirement {criterion}")
    identifier = criterion.split(REVISION_SEPARATOR, 1)[0]
    findings = tuple(
        Finding(
            _string(f.get("category")),
            _string(f.get("module"), "shallnot"),
            _string(f.get("message")),
            f.get("blocking") is True,
        )
        for f in _items(report.get("findings"))
        if f.get("requirement_id") == identifier
    )
    tests = tuple(
        sorted(
            (
                CitingTest(_string(t.get("name")), _string(t.get("classname")), _string(t.get("outcome"), NOT_RECORDED))
                for t in _items(requirement.get("tests"))
            ),
            key=lambda t: (t.classname, t.name),
        )
    )
    return GateReading(
        verdict=_string(report.get("verdict"), NOT_RECORDED),
        statement=text(requirement.get("statement"), f"{criterion}.statement"),
        tests=tests,
        findings=findings,
        run=_test_run(report),
    )
