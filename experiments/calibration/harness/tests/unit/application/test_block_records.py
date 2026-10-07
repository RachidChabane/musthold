from __future__ import annotations

import json
from datetime import datetime

import pytest

from calibration.application.block import BlockOutcome
from calibration.application.layout import canary_path, raw_meta_path, server_card_path
from calibration.application.safety_log import EVENT_SHUTDOWN, SafetyLog
from calibration.domain.canonical import document_json
from calibration.domain.constants import DEVIATIONS_FILE, MANIFEST_FILE, TIMING_FILE
from calibration.domain.models import model_by_id
from calibration.domain.records import RawMeta
from calibration.domain.schedule import Pass
from support.application_fakes import (
    UNCLEAN_SHUTDOWN,
    FakeModelServer,
    ManualClock,
    MemoryRunStore,
    seeded_rig,
)
from support.run_directory_seed import (
    INVALID_REPLY,
    ServerBehaviour,
    canary_reply,
    seed_responses,
)

PROTOCOL_M3 = "- protocol | scope: M3 | "


def deviations(store: MemoryRunStore) -> list[str]:
    return [line for line in store.text_of(DEVIATIONS_FILE).splitlines() if line.startswith("- ")]


def meta_of(store: MemoryRunStore, model_id: str, request_id: str) -> RawMeta:
    return RawMeta.from_record(json.loads(store.read_text(raw_meta_path(model_id, Pass.ONE, request_id))))


def timing(store: MemoryRunStore) -> dict[str, dict[str, object]]:
    return json.loads(store.read_text(TIMING_FILE))


def test_a_failing_canary_stops_the_block_before_any_item() -> None:
    behaviour = ServerBehaviour(model_by_id("M3").flavour, canary=INVALID_REPLY)
    rig, first, _ = seeded_rig(behaviour=behaviour)

    outcome = rig.run()

    assert outcome is BlockOutcome.CANARY_FAILED
    assert all(not rig.store.exists(raw_meta_path("M3", Pass.ONE, request_id)) for request_id in first)
    assert json.loads(rig.store.read_text(canary_path("M3")))["passed"] is False
    assert rig.events == ["start:M3", "stop:M3"]


def test_the_canary_is_recorded_with_its_request_response_and_checks() -> None:
    rig, _, _ = seeded_rig()

    rig.run(canary_only=True)

    record = json.loads(rig.store.read_text(canary_path("M3")))
    assert record["passed"] is True
    assert record["model"] == "M3"
    assert len(record["checks"]) == 5


def test_a_passed_canary_is_not_run_again() -> None:
    rig, _, _ = seeded_rig()
    rig.run(canary_only=True)
    posts = len(rig.transport.posts)

    assert rig.run(canary_only=True) is BlockOutcome.COMPLETE
    assert len(rig.transport.posts) == posts


def test_a_failed_canary_is_not_rerun_without_a_protocol_entry() -> None:
    rig, _, _ = seeded_rig(behaviour=ServerBehaviour(model_by_id("M3").flavour, canary=INVALID_REPLY))
    rig.run()
    posts = len(rig.transport.posts)

    assert rig.run() is BlockOutcome.CANARY_FAILED
    assert len(rig.transport.posts) == posts


def rig_with_failed_canary_then_a_fix(entry: str) -> tuple[BlockOutcome, int]:
    flavour = model_by_id("M3").flavour
    failing = ServerBehaviour(flavour, canary=INVALID_REPLY)
    rig, _, _ = seeded_rig(behaviour=failing)
    rig.run()
    rig.store.append_text(DEVIATIONS_FILE, entry)
    posts = len(rig.transport.posts)
    failing.canary = canary_reply(flavour)
    return rig.run(), len(rig.transport.posts) - posts


def test_a_protocol_entry_naming_the_model_allows_the_canary_to_run_again() -> None:
    outcome, new_posts = rig_with_failed_canary_then_a_fix("- protocol | scope: M3 | fixed the relay\n")

    assert outcome is BlockOutcome.COMPLETE
    assert new_posts > 0


def test_a_protocol_entry_for_another_model_does_not_allow_the_rerun() -> None:
    outcome, new_posts = rig_with_failed_canary_then_a_fix("- protocol | scope: M2 | fixed the relay\n")

    assert outcome is BlockOutcome.CANARY_FAILED
    assert new_posts == 0


def test_a_blocked_model_with_a_failed_canary_stays_blocked_even_with_a_protocol_entry() -> None:
    rig, _, _ = seeded_rig(blocked=["M3"])
    rig.store.append_text(DEVIATIONS_FILE, "- protocol | scope: M3 | rerun\n")

    assert rig.run() is BlockOutcome.BLOCKED


def test_a_server_that_differs_from_the_lock_is_a_protocol_entry_for_the_model() -> None:
    rig, _, _ = seeded_rig(pass1=1)
    rig.servers.install(FakeModelServer("M3", model_by_id("M3").flavour, rig.events, server_version="0.99.0"))

    rig.run()

    assert deviations(rig.store) == [f"{PROTOCOL_M3}M3: the server is 0.99.0, the lock pins 0.35.1"]


def test_a_model_digest_that_differs_from_the_lock_is_a_protocol_entry() -> None:
    rig, _, _ = seeded_rig(pass1=1)
    rig.servers.install(FakeModelServer("M3", model_by_id("M3").flavour, rig.events, model_digest="sha256:other"))

    rig.run()

    assert deviations(rig.store) == [f"{PROTOCOL_M3}M3: the model digest is sha256:other, the lock pins digest-M3"]


def test_a_digest_that_differs_only_by_its_prefix_and_case_is_no_deviation() -> None:
    rig, _, _ = seeded_rig(pass1=1)
    rig.servers.install(FakeModelServer("M3", model_by_id("M3").flavour, rig.events, model_digest="sha256:DIGEST-M3"))

    rig.run()

    assert not rig.store.exists(DEVIATIONS_FILE)


def test_a_kev_server_is_compared_with_the_pinned_commit() -> None:
    rig, _, _ = seeded_rig(model_id="M4", pass1=1)
    rig.servers.install(FakeModelServer("M4", model_by_id("M4").flavour, rig.events, server_version="deadbeef"))

    rig.run("M4")

    assert deviations(rig.store)[0].startswith("- protocol | scope: M4 | M4: the server is deadbeef")


def test_a_matching_block_logs_no_deviation_at_all() -> None:
    rig, _, _ = seeded_rig(pass1=2)

    rig.run()

    assert not rig.store.exists(DEVIATIONS_FILE)


def seed_card(store: MemoryRunStore, model_id: str, start: str, provenance: dict[str, str]) -> None:
    store.write_text(server_card_path(model_id, start), document_json({"provenance": provenance}))


def test_a_provenance_value_that_changed_since_the_previous_block_is_a_provenance_entry() -> None:
    rig, _, _ = seeded_rig(pass1=1)
    seed_card(rig.store, "M2", "2026-10-06T21:00:00", {"os_build": "25A1", "uv": "0.9"})
    rig.provenance.values = {"os_build": "25A2", "uv": "0.9"}

    rig.run()

    assert deviations(rig.store) == ["- provenance | scope: none | os_build changed from 25A1 to 25A2"]


def test_the_previous_block_is_the_most_recent_one_of_any_model() -> None:
    rig, _, _ = seeded_rig(pass1=1)
    seed_card(rig.store, "M2", "2026-10-05T21:00:00", {"os_build": "old"})
    seed_card(rig.store, "M1", "2026-10-06T21:00:00", {"os_build": "25A2"})
    rig.provenance.values = {"os_build": "25A2"}

    rig.run()

    assert not rig.store.exists(DEVIATIONS_FILE)


def test_the_first_block_has_no_previous_provenance_to_compare() -> None:
    rig, _, _ = seeded_rig(pass1=1)
    rig.provenance.values = {"os_build": "25A2"}

    rig.run()

    assert not rig.store.exists(DEVIATIONS_FILE)


def test_a_harness_commit_that_differs_from_the_manifest_is_a_protocol_entry_for_everything() -> None:
    rig, _, _ = seeded_rig(pass1=1)
    rig.store.write_text(MANIFEST_FILE, document_json({"harness_commit": "old", "builder_digest": "builders-1"}))

    rig.run()

    assert deviations(rig.store) == ["- protocol | scope: all | harness_commit is harness-1, the manifest records old"]


def test_a_builder_digest_that_differs_from_the_manifest_is_a_protocol_entry() -> None:
    rig, _, _ = seeded_rig(pass1=1)
    rig.store.write_text(MANIFEST_FILE, document_json({"harness_commit": "harness-1", "builder_digest": "other"}))

    rig.run()

    assert deviations(rig.store) == [
        "- protocol | scope: all | builder_digest is builders-1, the manifest records other"
    ]


def test_a_manifest_without_the_harness_keys_records_no_deviation() -> None:
    rig, _, _ = seeded_rig(pass1=1)
    rig.store.write_text(MANIFEST_FILE, document_json({"run_id": "run-01"}))

    rig.run()

    assert not rig.store.exists(DEVIATIONS_FILE)


def test_the_server_card_of_the_block_is_written_under_the_block_start() -> None:
    clock = ManualClock(datetime(2026, 10, 7, 22, 15, 0))
    rig, _, _ = seeded_rig(pass1=1, clock=clock)
    rig.provenance.values = {"os_build": "25A2"}

    rig.run()

    card = json.loads(rig.store.read_text(server_card_path("M3", clock.current.isoformat())))
    assert card["model"] == "M3"
    assert card["block_start"] == clock.current.isoformat()
    assert card["provenance"] == {"os_build": "25A2"}
    assert card["harness"] == rig.harness.values
    assert card["card"]["server_version"] == "0.35.1"


def test_every_raw_record_carries_the_server_version_and_model_digest_of_its_block() -> None:
    rig, first, _ = seeded_rig(pass1=1)

    rig.run()

    meta = meta_of(rig.store, "M3", first[0])
    assert (meta.server_version, meta.model_digest) == ("0.35.1", "digest-M3")


def test_a_kev_prefix_cache_hit_is_read_from_the_counter_between_requests() -> None:
    rig, first, _ = seeded_rig(model_id="M4", pass1=3)
    rig.servers.install(FakeModelServer("M4", model_by_id("M4").flavour, rig.events, cache_hits=[5, 5, 6, 6]))

    rig.run("M4")

    assert [meta_of(rig.store, "M4", request_id).prefix_cache_hit for request_id in first] == [False, True, False]


def test_a_server_without_a_counter_records_no_prefix_cache_outcome() -> None:
    rig, first, _ = seeded_rig(pass1=2)

    rig.run()

    assert [meta_of(rig.store, "M3", request_id).prefix_cache_hit for request_id in first] == [None, None]


def test_a_clean_shutdown_is_in_the_safety_log_and_logs_no_deviation() -> None:
    rig, _, _ = seeded_rig(pass1=1)

    rig.run()

    shutdown = next(e for e in SafetyLog(rig.store).events() if e["event"] == EVENT_SHUTDOWN)
    assert (shutdown["unloaded"], shutdown["server_stopped"], shutdown["port_free"]) == (True, True, True)
    assert not rig.store.exists(DEVIATIONS_FILE)


def test_a_shutdown_that_is_not_clean_is_a_protocol_entry_for_the_model() -> None:
    rig, _, _ = seeded_rig(pass1=1)
    rig.servers.install(FakeModelServer("M3", model_by_id("M3").flavour, rig.events, shutdown=UNCLEAN_SHUTDOWN))

    rig.run()

    assert deviations(rig.store) == [f"{PROTOCOL_M3}the end-of-block shutdown check failed"]
    shutdown = next(e for e in SafetyLog(rig.store).events() if e["event"] == EVENT_SHUTDOWN)
    assert (shutdown["models_after_unload"], shutdown["port_free"]) == (["loaded"], False)


def test_a_server_that_fails_to_start_is_still_stopped_and_timed() -> None:
    rig, _, _ = seeded_rig(pass1=1)
    server = rig.servers.install(
        FakeModelServer("M3", model_by_id("M3").flavour, rig.events, start_failure=RuntimeError("no start"))
    )

    with pytest.raises(RuntimeError, match="no start"):
        rig.run()

    assert server.stops == 1
    assert rig.store.exists(TIMING_FILE)


def test_timing_records_the_block_start_end_request_count_median_and_p95() -> None:
    seconds = [0.1, 0.2, 0.3, 0.4]
    clock = ManualClock()
    served = iter(seconds)

    def tick(body: bytes) -> None:
        if b'"request"' in body:
            clock.elapsed += next(served)

    rig, _, _ = seeded_rig(pass1=4, clock=clock, after_post=tick)

    rig.run()

    (key, entry) = next(iter(timing(rig.store).items()))
    assert key == f"M3/{clock.current.isoformat()}"
    assert entry["requests"] == 4
    assert entry["started"] == entry["finished"] == clock.current.isoformat()
    assert entry["median_latency_ms"] == pytest.approx(200.0)
    assert entry["p95_latency_ms"] == pytest.approx(400.0)


def test_timing_counts_item_requests_and_not_the_canary() -> None:
    rig, first, _ = seeded_rig(pass1=2)

    rig.run()

    assert next(iter(timing(rig.store).values()))["requests"] == len(first)


def test_a_block_that_sent_nothing_has_no_latencies() -> None:
    rig, first, _ = seeded_rig(pass1=1)
    rig.run(canary_only=True)

    entry = next(iter(timing(rig.store).values()))

    assert (entry["requests"], entry["median_latency_ms"], entry["p95_latency_ms"]) == (0, None, None)
    assert first


def test_each_block_adds_its_own_entry_to_the_timing_of_the_run() -> None:
    clock = ManualClock()
    rig, first, _ = seeded_rig(pass1=2, clock=clock)
    seed_responses(rig.store, "M3", Pass.ONE, first[:1])
    rig.run(canary_only=True)
    clock.current = datetime(2026, 10, 8, 22, 0, 0)

    rig.run()

    assert len(timing(rig.store)) == 2
