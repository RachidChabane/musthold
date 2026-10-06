"""Records of the written suites and of their kill vectors (RUN_RECORDS: `suites/`, `writer_kills.jsonl`)."""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from typing import Any

from pilot.domain.model_slate import Writer
from pilot.domain.test_writing import TestStatus, WrittenTest, suite_module, suite_nodeid, suite_path

STATUS_OK = "ok"
STATUS_MISSING = "missing"


@dataclass(frozen=True)
class SuiteTest:
    name: str
    source: str
    status: TestStatus
    reason: str | None = None

    def to_mapping(self) -> dict[str, Any]:
        return {"name": self.name, "source": self.source, "status": self.status.value, "reason": self.reason}

    @classmethod
    def from_mapping(cls, raw: Mapping[str, Any]) -> SuiteTest:
        return cls(raw["name"], raw["source"], TestStatus(raw["status"]), raw["reason"])


@dataclass(frozen=True)
class SuiteRecord:
    """One writer's suite for one criterion; `reference_checked` is set once the reference filter has run."""

    writer: Writer
    criterion: str
    call_id: str
    status: str
    tests: tuple[SuiteTest, ...]
    reference_checked: bool = False

    @property
    def kept(self) -> tuple[SuiteTest, ...]:
        return tuple(test for test in self.tests if test.status is TestStatus.KEPT)

    @property
    def nodeids(self) -> tuple[str, ...]:
        return tuple(suite_nodeid(self.criterion, test.name) for test in self.kept)

    def module(self) -> str:
        return suite_module(self.criterion, [WrittenTest(test.name, test.source) for test in self.kept])

    def to_mapping(self) -> dict[str, Any]:
        return {
            "writer": self.writer.value,
            "criterion": self.criterion,
            "call_id": self.call_id,
            "status": self.status,
            "reference_checked": self.reference_checked,
            "tests": [test.to_mapping() for test in self.tests],
        }

    @classmethod
    def from_mapping(cls, raw: Mapping[str, Any]) -> SuiteRecord:
        return cls(
            Writer(raw["writer"]), raw["criterion"], raw["call_id"], raw["status"],
            tuple(SuiteTest.from_mapping(test) for test in raw["tests"]), raw["reference_checked"],
        )


def suite_files(records: Sequence[SuiteRecord]) -> dict[str, str]:
    """The test modules of one writer, by project-relative path; a suite with no kept test has no module."""
    return {suite_path(record.criterion): record.module() for record in records if record.kept}
