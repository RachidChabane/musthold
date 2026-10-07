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
)
from support.build_fakes import InMemoryRunStore


def _log() -> tuple[DeviationLog, InMemoryRunStore]:
    store = InMemoryRunStore()
    return DeviationLog(store), store


def test_a_run_without_a_log_has_no_entries_and_no_file() -> None:
    log, store = _log()

    assert log.entries() == []
    assert not store.exists(DEVIATIONS_FILE)


def test_the_first_entry_creates_the_file_with_its_title() -> None:
    log, store = _log()

    log.record(DeviationKind.BLOCKED, ["M6"], "memory-fit")

    lines = store.read_text(DEVIATIONS_FILE).splitlines()
    assert lines[0] == DEVIATIONS_TITLE
    assert lines[2] == "- blocked | scope: M6 | memory-fit"


def test_entries_are_appended_and_the_earlier_ones_are_never_rewritten() -> None:
    log, store = _log()
    log.record(DeviationKind.BLOCKED, ["M6"], "first")
    after_first = store.read_text(DEVIATIONS_FILE)

    log.record(DeviationKind.PROTOCOL, [SCOPE_ALL], "second")

    assert store.read_text(DEVIATIONS_FILE).startswith(after_first)
    assert [entry.text for entry in log.entries()] == ["first", "second"]


def test_the_title_is_written_once() -> None:
    log, store = _log()
    for text in ("a", "b", "c"):
        log.record(DeviationKind.PROTOCOL, [SCOPE_ALL], text)

    assert store.read_text(DEVIATIONS_FILE).count(DEVIATIONS_TITLE) == 1


def test_a_scope_is_canonical_in_identifier_order_without_duplicates() -> None:
    log, _ = _log()

    entry = log.record(DeviationKind.BLOCKED, ["M3", "K4", "M3"], "text")

    assert entry.scope == ("K4", "M3")


def test_a_text_is_flattened_to_one_line() -> None:
    log, store = _log()

    log.record(DeviationKind.PROTOCOL, [SCOPE_ALL], "two\nlines   here")

    assert store.read_text(DEVIATIONS_FILE).splitlines()[-1].endswith("two lines here")


@pytest.mark.parametrize("kind", [DeviationKind.PROVENANCE, DeviationKind.PAUSE])
def test_a_pipeline_kind_touches_no_output_so_its_scope_is_none(kind: DeviationKind) -> None:
    log, _ = _log()

    entry = log.record(kind, [SCOPE_NONE], "changed")

    assert entry.scope == (SCOPE_NONE,)
    with pytest.raises(InvalidDeviation):
        log.record(kind, ["M1"], "changed")


def test_an_unknown_scope_identifier_is_refused_and_nothing_is_written() -> None:
    log, store = _log()

    with pytest.raises(InvalidDeviation):
        log.record(DeviationKind.BLOCKED, ["M9"], "text")

    assert not store.exists(DEVIATIONS_FILE)


def test_an_empty_text_is_refused() -> None:
    log, _ = _log()

    with pytest.raises(InvalidDeviation):
        log.record(DeviationKind.BLOCKED, ["M1"], "   ")


@pytest.mark.parametrize("kind", [DeviationKind.BLOCKED, DeviationKind.PROTOCOL])
def test_an_operator_may_log_blocked_and_protocol(kind: DeviationKind) -> None:
    log, _ = _log()

    assert log.record_operator(kind, ["M2"], "by hand").kind is kind


@pytest.mark.parametrize("kind", [DeviationKind.PROVENANCE, DeviationKind.PAUSE])
def test_an_operator_may_not_log_a_pipeline_kind(kind: DeviationKind) -> None:
    log, store = _log()

    with pytest.raises(InvalidDeviation):
        log.record_operator(kind, [SCOPE_NONE], "by hand")

    assert not store.exists(DEVIATIONS_FILE)


def test_a_foreign_line_reads_back_as_a_protocol_entry_for_everything() -> None:
    store = InMemoryRunStore({DEVIATIONS_FILE: f"{DEVIATIONS_TITLE}\n\nsomething hand written\n"})

    (entry,) = DeviationLog(store).entries()

    assert (entry.kind, entry.scope, entry.well_formed) == (DeviationKind.PROTOCOL, (SCOPE_ALL,), False)


def test_an_entry_recorded_after_a_hand_written_line_keeps_it() -> None:
    store = InMemoryRunStore({DEVIATIONS_FILE: f"{DEVIATIONS_TITLE}\n\nsomething hand written\n"})
    log = DeviationLog(store)

    log.record(DeviationKind.BLOCKED, ["K4"], "arm records missing")

    assert [entry.well_formed for entry in log.entries()] == [False, True]
