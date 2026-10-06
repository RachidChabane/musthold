"""Use case `leak-check`: static in-universe rules over the fixture's test files."""

from __future__ import annotations

from pilot.application.ports import TestSourceReader
from pilot.domain.leak_rules import LeakFinding, check_test_files


def leak_check(
    reader: TestSourceReader, expected_files: tuple[str, ...], expected_test_count: int
) -> list[LeakFinding]:
    return check_test_files(reader.read(), expected_files, expected_test_count, reader.read_conftest())
