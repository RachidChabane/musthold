from __future__ import annotations

from pathlib import Path

import pytest

from calibration.application.builder_check import (
    PROJECT_DIR,
    RECORDED_REPORT,
    RECORDED_SHALLNOT,
    BuilderCheck,
    recorded_report_path,
    run_builder_check,
)
from calibration.domain.canonical import JsonValue
from calibration.domain.jsonread import RecordError
from support.build_fakes import FakeAlignmentDriver, FakeBuilders

EXPORT_DIR = Path("/cache/product-abc")
EXPORTED_TEXT = "systemone export text"
HARNESS_COMMIT = "d" * 40


def _report(exported: int = 3, rebuilt: int = 3, matched: int = 3) -> dict[str, JsonValue]:
    return {
        "exported": exported,
        "rebuilt": rebuilt,
        "matched": matched,
        "unmatched_exported": [],
        "unmatched_rebuilt": [],
    }


def _check(report: dict[str, JsonValue]) -> tuple[BuilderCheck, FakeAlignmentDriver, FakeBuilders]:
    driver = FakeAlignmentDriver(EXPORTED_TEXT)
    builders = FakeBuilders({}, report, "unused")
    return run_builder_check(driver, builders, EXPORT_DIR), driver, builders


def test_the_driver_runs_the_export_with_the_recorded_shallnot_and_report() -> None:
    _, driver, _ = _check(_report())

    assert driver.calls == [(EXPORT_DIR, EXPORT_DIR / RECORDED_SHALLNOT, EXPORT_DIR / RECORDED_REPORT)]


def test_the_builder_compares_the_exported_text_on_the_products_own_test_project() -> None:
    _, _, builders = _check(_report())

    assert builders.compare_calls == [(EXPORT_DIR / PROJECT_DIR, EXPORT_DIR / RECORDED_SHALLNOT, EXPORTED_TEXT)]


def test_the_recorded_report_path_is_below_the_export() -> None:
    assert recorded_report_path(EXPORT_DIR) == EXPORT_DIR / RECORDED_REPORT


def test_a_full_match_passes() -> None:
    check, _, _ = _check(_report())

    assert check.passed
    assert (check.exported, check.rebuilt, check.matched) == (3, 3, 3)


@pytest.mark.parametrize(
    ("exported", "rebuilt", "matched"),
    [(0, 0, 0), (3, 3, 2), (2, 3, 3)],
    ids=["nothing exported", "unmatched export", "matched beyond the export"],
)
def test_anything_short_of_a_byte_for_byte_rebuild_of_every_export_fails(
    exported: int, rebuilt: int, matched: int
) -> None:
    check, _, _ = _check(_report(exported, rebuilt, matched))

    assert not check.passed


def test_a_listed_unmatched_ticket_fails_even_when_the_counts_agree() -> None:
    report = {**_report(), "unmatched_exported": ["ticket-1"]}

    check, _, _ = _check(report)

    assert not check.passed
    assert check.unmatched_exported == ("ticket-1",)


def test_rebuilt_sets_beyond_the_exported_ones_do_not_fail_the_check() -> None:
    report = {**_report(rebuilt=4), "unmatched_rebuilt": ["label-1"]}

    check, _, _ = _check(report)

    assert check.passed
    assert check.unmatched_rebuilt == ("label-1",)


def test_the_record_names_the_harness_commit_and_the_outcome() -> None:
    check, _, _ = _check(_report())

    assert check.to_record(HARNESS_COMMIT) == {
        "harness_commit": HARNESS_COMMIT,
        "exported": 3,
        "rebuilt": 3,
        "matched": 3,
        "unmatched_exported": [],
        "unmatched_rebuilt": [],
        "passed": True,
    }


def test_a_report_without_counts_is_a_record_error() -> None:
    with pytest.raises(RecordError, match=r"compare\.exported"):
        _check({"rebuilt": 1})
