from __future__ import annotations

import json
from collections.abc import Callable
from datetime import datetime

import pytest

from calibration.application.block import BlockOutcome
from calibration.application.layout import (
    raw_dir,
    raw_meta_path,
    raw_request_path,
    raw_response_path,
)
from calibration.application.safety_log import EVENT_HEAVY_WORK, EVENT_WINDOW, SafetyLog
from calibration.domain.constants import (
    DEFAULT_RUN_WINDOW,
    DEVIATIONS_FILE,
    HEAVY_CHECK_EVERY,
    HEAVY_WAIT_SECONDS,
    RETRY_DELAY_SECONDS,
)
from calibration.domain.deviations import DeviationKind
from calibration.domain.models import model_by_id
from calibration.domain.ports import TransportFailure
from calibration.domain.records import RawMeta
from calibration.domain.schedule import Pass
from calibration.domain.window import WINDOW_CLOSED_TEXT, RunWindow
from support.application_fakes import (
    BUSY_REPORT,
    CLEAR_REPORT,
    EVENING,
    MORNING,
    ManualClock,
    MemoryRunStore,
    ScriptedHeavyWork,
    seeded_rig,
)
from support.run_directory_seed import (
    INVALID_REPLY,
    ITEM_REPLY,
    REQUEST_MEMBER,
    ServerBehaviour,
    item_body,
    posted_request_ids,
    seed_responses,
    too_large_reply,
)

RUN_WINDOW = RunWindow.parse(DEFAULT_RUN_WINDOW)
NEXT_EVENING = datetime(2026, 10, 8, 22, 0, 0)
PRE_DAWN = datetime(2026, 10, 8, 7, 0, 0)
WINDOW_CLOSED_LINE = f"- {DeviationKind.PAUSE.value} | scope: none | {WINDOW_CLOSED_TEXT}"
HEAVY_PAUSE_LINE = f"- {DeviationKind.PAUSE.value} | scope: none | another heavy local job is running"


def deviation_lines(store: MemoryRunStore) -> list[str]:
    return store.text_of(DEVIATIONS_FILE).splitlines()


def heavy_events(store: MemoryRunStore) -> list[bool]:
    return [bool(event["clear"]) for event in SafetyLog(store).events() if event["event"] == EVENT_HEAVY_WORK]


def answered(store: MemoryRunStore, which: Pass, ids: list[str], model_id: str = "M3") -> list[str]:
    return [request_id for request_id in ids if store.exists(raw_response_path(model_id, which, request_id))]


def close_after_item_posts(clock: ManualClock, count: int) -> Callable[[bytes], None]:
    seen = 0

    def after_post(body: bytes) -> None:
        nonlocal seen
        if REQUEST_MEMBER in json.loads(body):
            seen += 1
            if seen == count:
                clock.current = MORNING

    return after_post


def test_a_model_the_lock_blocks_is_blocked_without_starting_a_server() -> None:
    rig, _, _ = seeded_rig(blocked=["M3"])

    outcome = rig.run()

    assert outcome is BlockOutcome.BLOCKED
    assert rig.servers.requested == []
    assert rig.transport.posts == []
    assert rig.heavy.calls == 0


@pytest.mark.parametrize("scope", ["M3", "all", "M1, M3"])
def test_a_blocked_entry_in_scope_of_the_model_blocks_it(scope: str) -> None:
    rig, _, _ = seeded_rig()
    rig.store.write_text(DEVIATIONS_FILE, f"# Deviations\n\n- blocked | scope: {scope} | memory-pressure: 2 stops\n")

    assert rig.run() is BlockOutcome.BLOCKED
    assert rig.servers.requested == []


def test_a_blocked_entry_for_another_model_does_not_block_this_one() -> None:
    rig, _, _ = seeded_rig()
    rig.store.write_text(DEVIATIONS_FILE, "# Deviations\n\n- blocked | scope: M2 | memory-fit\n")

    assert rig.run() is BlockOutcome.COMPLETE


def test_a_finished_model_is_complete_without_a_window_a_check_or_a_server() -> None:
    rig, first, second = seeded_rig(pass1=3, pass2=2, window=RUN_WINDOW, clock=ManualClock(MORNING))
    seed_responses(rig.store, "M3", Pass.ONE, first)
    seed_responses(rig.store, "M3", Pass.TWO, second)

    assert rig.run() is BlockOutcome.COMPLETE
    assert rig.servers.requested == []
    assert rig.heavy.calls == 0
    assert not rig.store.exists(DEVIATIONS_FILE)


def test_a_model_runs_pass_one_then_pass_two_in_the_schedule_order() -> None:
    rig, first, second = seeded_rig(pass1=4, pass2=2)

    outcome = rig.run()

    assert outcome is BlockOutcome.COMPLETE
    assert posted_request_ids(rig.transport.posts) == [*first, *second]
    assert second == [first[1], first[0]]
    assert answered(rig.store, Pass.ONE, first) == first
    assert answered(rig.store, Pass.TWO, second) == second


def test_the_request_bytes_are_the_archived_body_of_the_models_identifier() -> None:
    rig, first, _ = seeded_rig(pass1=1)
    model = model_by_id("M3")

    rig.run()

    sent = item_body(model.identifier, first[0]).encode()
    assert rig.store.read_bytes(raw_request_path("M3", Pass.ONE, first[0])) == sent
    assert sent in rig.transport.posts
    assert rig.store.read_bytes(raw_response_path("M3", Pass.ONE, first[0])) == ITEM_REPLY.body


def test_the_block_posts_to_the_servers_own_endpoint() -> None:
    rig, _, _ = seeded_rig(pass1=1)

    rig.run()

    assert set(rig.transport.urls) == {rig.servers.servers["M3"].endpoint_url}


def test_the_server_is_started_for_the_model_and_stopped_at_the_end() -> None:
    rig, _, _ = seeded_rig(pass1=2)

    rig.run()

    assert rig.events == ["start:M3", "stop:M3"]
    assert rig.servers.servers["M3"].started_with == [("tev1:0.8b", None)]


def test_a_kev_block_starts_its_server_with_the_pinned_run_reference() -> None:
    rig, _, _ = seeded_rig(model_id="M4", pass1=1)

    rig.run("M4")

    (identifier, reference) = rig.servers.servers["M4"].started_with[0]
    assert identifier == "kev-0.8b"
    assert reference is not None
    assert reference.startswith("jaredpalmer/kev-0.8b@")


def test_a_request_whose_response_file_exists_is_not_resent() -> None:
    rig, first, _ = seeded_rig(pass1=3)
    seed_responses(rig.store, "M3", Pass.ONE, first[:1])

    rig.run()

    assert posted_request_ids(rig.transport.posts) == first[1:]
    assert answered(rig.store, Pass.ONE, first) == first


def test_a_request_interrupted_before_its_response_was_written_is_sent_again() -> None:
    rig, first, _ = seeded_rig(pass1=2)
    rig.store.write_bytes(raw_request_path("M3", Pass.ONE, first[0]), b"torn")
    rig.store.write_text(raw_meta_path("M3", Pass.ONE, first[0]), "{}")

    rig.run()

    assert posted_request_ids(rig.transport.posts) == first
    assert rig.store.read_bytes(raw_response_path("M3", Pass.ONE, first[0])) == ITEM_REPLY.body


def test_a_second_run_of_a_finished_block_sends_nothing() -> None:
    rig, _, _ = seeded_rig(pass1=2, pass2=1)
    rig.run()
    posts_after_first = len(rig.transport.posts)

    assert rig.run() is BlockOutcome.COMPLETE
    assert len(rig.transport.posts) == posts_after_first


def test_a_request_is_written_as_request_then_meta_then_response() -> None:
    rig, first, _ = seeded_rig(pass1=1)
    rig.run()

    folder = raw_dir("M3", Pass.ONE, first[0])
    written = [path for path in rig.store.writes if path.startswith(folder)]

    assert written == [
        raw_request_path("M3", Pass.ONE, first[0]),
        raw_meta_path("M3", Pass.ONE, first[0]),
        raw_response_path("M3", Pass.ONE, first[0]),
    ]


def test_the_first_attempt_bytes_are_kept_between_request_and_meta_after_a_retry() -> None:
    first_id = seeded_rig(pass1=1)[1][0]
    behaviour = ServerBehaviour(model_by_id("M3").flavour, queued={first_id: [INVALID_REPLY]})
    rig, first, _ = seeded_rig(pass1=1, behaviour=behaviour)

    rig.run()

    folder = raw_dir("M3", Pass.ONE, first[0])
    written = [path for path in rig.store.writes if path.startswith(folder)]
    assert written == [
        raw_request_path("M3", Pass.ONE, first[0]),
        f"{folder}/response.attempt1.bin",
        raw_meta_path("M3", Pass.ONE, first[0]),
        raw_response_path("M3", Pass.ONE, first[0]),
    ]
    assert rig.store.read_bytes(f"{folder}/response.attempt1.bin") == INVALID_REPLY.body
    assert rig.store.read_bytes(raw_response_path("M3", Pass.ONE, first[0])) == ITEM_REPLY.body
    assert rig.sleeper.sleeps == [RETRY_DELAY_SECONDS]


def test_a_request_that_fails_twice_is_recorded_missing_and_is_not_resent_later() -> None:
    first_id = seeded_rig(pass1=2)[1][0]
    failure = TransportFailure("connection reset")
    behaviour = ServerBehaviour(model_by_id("M3").flavour, queued={first_id: [failure, failure]})
    rig, first, _ = seeded_rig(pass1=2, behaviour=behaviour)

    rig.run()
    meta = RawMeta.from_record(json.loads(rig.store.read_text(raw_meta_path("M3", Pass.ONE, first[0]))))

    assert (meta.attempt, meta.http_status, meta.transport_error) == (2, None, "connection reset")
    assert rig.store.read_bytes(raw_response_path("M3", Pass.ONE, first[0])) == b""
    assert rig.run() is BlockOutcome.COMPLETE
    assert posted_request_ids(rig.transport.posts).count(first[0]) == 2


def test_a_state_too_large_reply_is_recorded_after_one_attempt() -> None:
    first_id = seeded_rig(pass1=1)[1][0]
    refusal = too_large_reply(model_by_id("M3").flavour)
    behaviour = ServerBehaviour(model_by_id("M3").flavour, queued={first_id: [refusal]})
    rig, first, _ = seeded_rig(pass1=1, behaviour=behaviour)

    rig.run()
    meta = RawMeta.from_record(json.loads(rig.store.read_text(raw_meta_path("M3", Pass.ONE, first[0]))))

    assert (meta.attempt, meta.http_status) == (1, refusal.status)
    assert rig.sleeper.sleeps == []
    assert not rig.store.exists(f"{raw_dir('M3', Pass.ONE, first[0])}/response.attempt1.bin")


def test_a_block_outside_the_window_is_a_pause_and_starts_no_server() -> None:
    rig, _, _ = seeded_rig(window=RUN_WINDOW, clock=ManualClock(MORNING))

    outcome = rig.run()

    assert outcome is BlockOutcome.WINDOW_CLOSED
    assert rig.servers.requested == []
    assert deviation_lines(rig.store)[-1] == WINDOW_CLOSED_LINE
    assert rig.heavy.calls == 0


def test_a_closed_window_is_logged_in_the_safety_log() -> None:
    rig, _, _ = seeded_rig(window=RUN_WINDOW, clock=ManualClock(MORNING))

    rig.run()

    windows = [event for event in SafetyLog(rig.store).events() if event["event"] == EVENT_WINDOW]
    assert [event["open"] for event in windows] == [False]


def test_a_block_with_no_window_runs_at_any_hour() -> None:
    rig, _, _ = seeded_rig(window=None, clock=ManualClock(MORNING))

    assert rig.run() is BlockOutcome.COMPLETE


def test_the_window_closing_mid_pass_finishes_the_request_in_flight_then_pauses() -> None:
    clock = ManualClock(EVENING)
    rig, first, _ = seeded_rig(pass1=4, window=RUN_WINDOW, clock=clock, after_post=close_after_item_posts(clock, 2))

    outcome = rig.run()

    assert outcome is BlockOutcome.WINDOW_CLOSED
    assert answered(rig.store, Pass.ONE, first) == first[:2]
    assert deviation_lines(rig.store)[-1] == WINDOW_CLOSED_LINE
    assert rig.events == ["start:M3", "stop:M3"]
    assert rig.store.exists("timing.json")


def test_a_later_session_resumes_the_block_after_the_window_closed() -> None:
    clock = ManualClock(EVENING)
    rig, first, second = seeded_rig(
        pass1=4, pass2=1, window=RUN_WINDOW, clock=clock, after_post=close_after_item_posts(clock, 2)
    )
    rig.run()
    clock.current = NEXT_EVENING

    outcome = rig.run()

    assert outcome is BlockOutcome.COMPLETE
    assert posted_request_ids(rig.transport.posts) == [*first, *second]
    assert rig.events == ["start:M3", "stop:M3", "start:M3", "stop:M3"]


def test_the_window_closing_between_the_passes_pauses_before_pass_two() -> None:
    clock = ManualClock(EVENING)
    rig, first, second = seeded_rig(
        pass1=2, pass2=2, window=RUN_WINDOW, clock=clock, after_post=close_after_item_posts(clock, 2)
    )

    outcome = rig.run()

    assert outcome is BlockOutcome.WINDOW_CLOSED
    assert answered(rig.store, Pass.ONE, first) == first
    assert answered(rig.store, Pass.TWO, second) == []


def test_the_heavy_work_check_runs_once_before_a_short_block() -> None:
    rig, _, _ = seeded_rig(pass1=HEAVY_CHECK_EVERY - 1)

    rig.run()

    assert heavy_events(rig.store) == [True]


def test_the_heavy_work_check_runs_again_every_twenty_five_requests() -> None:
    rig, _, _ = seeded_rig(pass1=2 * HEAVY_CHECK_EVERY + 1)

    outcome = rig.run()

    assert outcome is BlockOutcome.COMPLETE
    assert rig.heavy.calls == 3


def test_a_block_of_exactly_twenty_five_requests_is_not_checked_again() -> None:
    rig, _, _ = seeded_rig(pass1=HEAVY_CHECK_EVERY)

    rig.run()

    assert rig.heavy.calls == 1


def test_resumed_requests_do_not_count_toward_the_twenty_five() -> None:
    rig, first, _ = seeded_rig(pass1=HEAVY_CHECK_EVERY + 10)
    seed_responses(rig.store, "M3", Pass.ONE, first[:10])

    rig.run()

    assert rig.heavy.calls == 1


def test_a_heavy_check_that_fails_during_the_block_stops_it_cleanly_with_a_pause_entry() -> None:
    heavy = ScriptedHeavyWork([CLEAR_REPORT, BUSY_REPORT])
    rig, first, _ = seeded_rig(pass1=HEAVY_CHECK_EVERY + 5, heavy=heavy)

    outcome = rig.run()

    assert outcome is BlockOutcome.HEAVY_WORK
    assert answered(rig.store, Pass.ONE, first) == first[:HEAVY_CHECK_EVERY]
    assert deviation_lines(rig.store)[-1] == HEAVY_PAUSE_LINE
    assert rig.events == ["start:M3", "stop:M3"]
    assert heavy_events(rig.store) == [True, False]


def test_a_failed_heavy_check_is_logged_with_what_it_found() -> None:
    heavy = ScriptedHeavyWork([CLEAR_REPORT, BUSY_REPORT])
    rig, _, _ = seeded_rig(pass1=HEAVY_CHECK_EVERY + 1, heavy=heavy)

    rig.run()

    events = [e for e in SafetyLog(rig.store).events() if e["event"] == EVENT_HEAVY_WORK]
    assert events[-1]["detail"] == BUSY_REPORT.describe()
    assert events[-1]["model"] == "M3"


def test_a_failed_heavy_check_before_the_block_waits_in_five_minute_steps() -> None:
    heavy = ScriptedHeavyWork([BUSY_REPORT, BUSY_REPORT, CLEAR_REPORT])
    rig, _, _ = seeded_rig(pass1=1, heavy=heavy, window=RUN_WINDOW)

    outcome = rig.run()

    assert outcome is BlockOutcome.COMPLETE
    assert rig.sleeper.sleeps == [HEAVY_WAIT_SECONDS, HEAVY_WAIT_SECONDS]
    assert heavy_events(rig.store) == [False, False, True]
    assert not rig.store.exists(DEVIATIONS_FILE)


def test_no_server_starts_while_other_heavy_work_runs() -> None:
    heavy = ScriptedHeavyWork([BUSY_REPORT, CLEAR_REPORT])
    rig, _, _ = seeded_rig(pass1=1, heavy=heavy)
    started_during_wait: list[int] = []
    original_sleep = rig.sleeper.sleep

    def sleep(seconds: float) -> None:
        started_during_wait.append(len(rig.servers.requested))
        original_sleep(seconds)

    rig.sleeper.sleep = sleep  # type: ignore[method-assign]
    rig.run()

    assert started_during_wait == [0]


def test_heavy_work_that_outlasts_the_window_ends_the_wait_with_a_closed_window() -> None:
    heavy = ScriptedHeavyWork([BUSY_REPORT])
    rig, _, _ = seeded_rig(heavy=heavy, window=RUN_WINDOW, clock=ManualClock(PRE_DAWN))

    outcome = rig.run()

    assert outcome is BlockOutcome.WINDOW_CLOSED
    assert rig.sleeper.sleeps == [HEAVY_WAIT_SECONDS] * 6
    assert rig.servers.requested == []
    assert deviation_lines(rig.store)[-1] == WINDOW_CLOSED_LINE


def test_a_canary_only_run_sends_the_canary_and_no_item() -> None:
    rig, first, _ = seeded_rig(pass1=2)

    outcome = rig.run(canary_only=True)

    assert outcome is BlockOutcome.COMPLETE
    assert posted_request_ids(rig.transport.posts) == []
    assert answered(rig.store, Pass.ONE, first) == []
    assert rig.store.exists("canary/M3.json")
