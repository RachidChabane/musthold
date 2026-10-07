from __future__ import annotations

import pytest

from calibration.application.deviation_log import DeviationLog
from calibration.domain.constants import DEVIATIONS_FILE
from calibration.domain.deviations import (
    DEVIATIONS_TITLE,
    SCOPE_ALL,
    SCOPE_NONE,
    DeviationKind,
    InvalidDeviation,
    standing_of,
)
from support.application_fakes import MemoryRunStore

WINDOW_TEXT = "run window closed"


def log_over(store: MemoryRunStore | None = None) -> tuple[DeviationLog, MemoryRunStore]:
    run_store = store or MemoryRunStore()
    return DeviationLog(run_store), run_store


def test_a_log_without_a_file_has_no_entries() -> None:
    log, store = log_over()

    assert log.entries() == []
    assert not store.exists(DEVIATIONS_FILE)


def test_the_first_entry_creates_the_file_with_its_title() -> None:
    log, store = log_over()

    log.record(DeviationKind.PAUSE, [SCOPE_NONE], WINDOW_TEXT)

    lines = store.read_text(DEVIATIONS_FILE).splitlines()
    assert lines == [DEVIATIONS_TITLE, "", f"- pause | scope: {SCOPE_NONE} | {WINDOW_TEXT}"]


def test_entries_are_appended_one_line_each_and_never_rewritten() -> None:
    log, store = log_over()

    log.record(DeviationKind.PAUSE, [SCOPE_NONE], WINDOW_TEXT)
    first_text = store.read_text(DEVIATIONS_FILE)
    log.record(DeviationKind.BLOCKED, ["M2"], "memory-fit")

    assert store.read_text(DEVIATIONS_FILE).startswith(first_text)
    assert store.read_text(DEVIATIONS_FILE).splitlines()[-1] == "- blocked | scope: M2 | memory-fit"


def test_the_entries_read_back_in_order_with_their_kind_scope_and_text() -> None:
    log, _ = log_over()

    log.record(DeviationKind.PROVENANCE, [SCOPE_NONE], "os build changed")
    log.record(DeviationKind.PROTOCOL, ["M3", "M1"], "digest changed")

    entries = log.entries()
    assert [(e.kind, e.scope, e.text) for e in entries] == [
        (DeviationKind.PROVENANCE, (SCOPE_NONE,), "os build changed"),
        (DeviationKind.PROTOCOL, ("M1", "M3"), "digest changed"),
    ]


def test_the_text_is_flattened_to_one_line() -> None:
    log, store = log_over()

    log.record(DeviationKind.PROTOCOL, [SCOPE_ALL], "first line\nsecond   line")

    assert (
        store.read_text(DEVIATIONS_FILE).splitlines()[-1] == f"- protocol | scope: {SCOPE_ALL} | first line second line"
    )


@pytest.mark.parametrize("kind", [DeviationKind.PAUSE, DeviationKind.PROVENANCE])
def test_a_pause_or_provenance_entry_must_have_no_scope(kind: DeviationKind) -> None:
    log, store = log_over()

    with pytest.raises(InvalidDeviation):
        log.record(kind, ["M3"], "text")

    assert not store.exists(DEVIATIONS_FILE)


def test_an_entry_needs_a_text_and_known_identifiers() -> None:
    log, _ = log_over()

    with pytest.raises(InvalidDeviation):
        log.record(DeviationKind.PROTOCOL, [SCOPE_ALL], "   ")
    with pytest.raises(InvalidDeviation):
        log.record(DeviationKind.PROTOCOL, ["M9"], "text")


@pytest.mark.parametrize("kind", [DeviationKind.BLOCKED, DeviationKind.PROTOCOL])
def test_an_operator_may_log_a_blocked_or_protocol_entry(kind: DeviationKind) -> None:
    log, _ = log_over()

    entry = log.record_operator(kind, ["M4"], "operator text")

    assert log.entries() == [entry]


@pytest.mark.parametrize("kind", [DeviationKind.PAUSE, DeviationKind.PROVENANCE])
def test_an_operator_may_not_log_a_pause_or_provenance_entry(kind: DeviationKind) -> None:
    log, store = log_over()

    with pytest.raises(InvalidDeviation):
        log.record_operator(kind, [SCOPE_NONE], "operator text")

    assert not store.exists(DEVIATIONS_FILE)


def test_a_hand_written_line_that_fits_no_entry_reads_as_a_protocol_entry_for_everything() -> None:
    store = MemoryRunStore()
    store.write_text(DEVIATIONS_FILE, f"{DEVIATIONS_TITLE}\n\nthe operator changed the window by hand\n")
    log, _ = log_over(store)

    entries = log.entries()

    assert [(e.kind, e.scope, e.well_formed) for e in entries] == [(DeviationKind.PROTOCOL, (SCOPE_ALL,), False)]
    assert standing_of(entries, ["M5"]).protocol


def test_an_entry_recorded_after_a_hand_written_line_is_kept_apart() -> None:
    store = MemoryRunStore()
    store.write_text(DEVIATIONS_FILE, f"{DEVIATIONS_TITLE}\n\nfree text\n")
    log, _ = log_over(store)

    log.record(DeviationKind.BLOCKED, ["M6"], "memory-fit")

    assert [e.well_formed for e in log.entries()] == [False, True]


def test_the_standing_of_a_model_follows_the_scope_of_the_entries() -> None:
    log, _ = log_over()
    log.record(DeviationKind.BLOCKED, ["M2"], "memory-fit")
    log.record(DeviationKind.PROTOCOL, ["M3"], "rerun")

    entries = log.entries()

    assert standing_of(entries, ["M2"]).blocked
    assert not standing_of(entries, ["M3"]).blocked
    assert standing_of(entries, ["M3"]).protocol
    assert not standing_of(entries, ["M2"]).protocol
