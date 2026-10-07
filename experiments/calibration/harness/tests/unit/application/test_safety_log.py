from __future__ import annotations

import json
from datetime import datetime

from calibration.application.safety_log import (
    EVENT_BLOCK,
    EVENT_FIT,
    EVENT_GUARD,
    EVENT_HEAVY_WORK,
    EVENT_PRESSURE_STOP,
    EVENT_SHUTDOWN,
    EVENT_WINDOW,
    SafetyLog,
)
from calibration.domain.constants import SAFETY_FILE
from support.application_fakes import MemoryRunStore

NOW = datetime(2026, 10, 7, 22, 30, 0)
LATER = datetime(2026, 10, 7, 23, 0, 0)


def test_a_log_without_a_file_has_no_events() -> None:
    assert SafetyLog(MemoryRunStore()).events() == []


def test_an_event_is_one_line_naming_its_event_model_and_time() -> None:
    store = MemoryRunStore()

    SafetyLog(store).append(EVENT_GUARD, "M3", NOW, {"free_percent": 55})

    lines = store.read_text(SAFETY_FILE).splitlines()
    assert [json.loads(line) for line in lines] == [
        {"event": EVENT_GUARD, "model": "M3", "time": NOW.isoformat(), "free_percent": 55}
    ]


def test_the_line_is_canonical_json_ending_with_a_newline() -> None:
    store = MemoryRunStore()

    SafetyLog(store).append(EVENT_WINDOW, None, NOW, {"open": False})

    assert store.read_text(SAFETY_FILE) == (
        f'{{"event":"{EVENT_WINDOW}","model":null,"open":false,"time":"{NOW.isoformat()}"}}\n'
    )


def test_events_are_appended_in_order_and_read_back_whole() -> None:
    store = MemoryRunStore()
    log = SafetyLog(store)

    log.append(EVENT_BLOCK, "M3", NOW, {"phase": "start"})
    log.append(EVENT_HEAVY_WORK, "M3", LATER, {"clear": True, "detail": ""})
    log.append(EVENT_FIT, "M2", LATER, {"decision": "load"})

    assert [event["event"] for event in log.events()] == [EVENT_BLOCK, EVENT_HEAVY_WORK, EVENT_FIT]
    assert [event["model"] for event in log.events()] == ["M3", "M3", "M2"]
    assert log.events()[1]["clear"] is True


def test_an_earlier_event_is_never_rewritten_by_a_later_one() -> None:
    store = MemoryRunStore()
    log = SafetyLog(store)
    log.append(EVENT_SHUTDOWN, "M1", NOW, {"port_free": True})
    first = store.read_text(SAFETY_FILE)

    log.append(EVENT_PRESSURE_STOP, "M1", LATER, {"stops": 1})

    assert store.read_text(SAFETY_FILE).startswith(first)


def test_a_new_log_object_over_the_same_store_reads_what_another_wrote() -> None:
    store = MemoryRunStore()
    SafetyLog(store).append(EVENT_FIT, "M4", NOW, {"deferrals": 2})

    assert SafetyLog(store).events()[0]["deferrals"] == 2


def test_blank_lines_are_ignored_when_reading() -> None:
    store = MemoryRunStore()
    store.write_text(SAFETY_FILE, '\n{"event":"window","model":null,"time":"t"}\n\n')

    assert [event["event"] for event in SafetyLog(store).events()] == [EVENT_WINDOW]
