"""`ModelBlock` driven with the real adapters (transport, servers, run store) against the stub servers."""

from __future__ import annotations

import json
import socket
from collections.abc import Iterator
from dataclasses import dataclass
from pathlib import Path

import pytest

from calibration.adapters.http_transport import UrllibTransport
from calibration.adapters.process_tree import ManagedProcess, ServerCommand, is_alive, port_is_free, spawn_process
from calibration.adapters.server_factory import ExperimentServerFactory
from calibration.application.block import BlockDeps, BlockOutcome, ModelBlock
from calibration.application.deviation_log import DeviationLog
from calibration.application.layout import (
    canary_path,
    item_path,
    raw_dir,
    raw_meta_path,
    raw_request_path,
    raw_response_path,
)
from calibration.application.ports import ModelServer, ServerCard, ServerFactory, ShutdownReport
from calibration.application.safety_log import EVENT_SHUTDOWN, SafetyLog
from calibration.domain.canonical import document_json
from calibration.domain.constants import ENCODING, KEV_COMMIT, TIMING_FILE
from calibration.domain.jsonread import json_object, sequence
from calibration.domain.models import ModelSpec, model_by_id
from calibration.domain.records import RawMeta
from calibration.domain.schedule import Pass, alignment_request_id
from calibration.domain.server_wire import ReplyClass, classify_reply, question_specs
from calibration.domain.window import RunWindow
from support.block_world import (
    ALIGNMENT_NODEID,
    WINDOW_TEXT,
    ClearHeavyProbe,
    FixedHarness,
    FixedProvenance,
    ObservedTransport,
    RecordingSleeper,
    RunWorld,
    SettableClock,
    build_run_world,
    canary_closed_text,
    local_tools,
)
from support.fakebin_install import TESTS_DIR, TESTS_DIR_VARIABLE, install_fakebin
from support.stub_common import StubHttpServer
from support.stub_ollama import CONTEXT_LENGTH, DEFAULT_DIGEST, OLLAMA_VERSION, OllamaBehaviour, StubOllamaServer

pytestmark = pytest.mark.integration

HOST = "127.0.0.1"
START_TIMEOUT = 30.0
STOP_GRACE = 2.0
OLLAMA_STATE_LIMIT = 3000
KEV_STATE_LIMIT = 100_000
BEHAVIOUR_VARIABLE = "FAKE_STUB_BEHAVIOUR"
RUNNER_PID_VARIABLE = "FAKE_OLLAMA_RUNNER_PID_FILE"
ROUTE = "/v1/systemone"
CANARY_REQUESTS = 2
HANG_SECONDS = 2.0
SHORT_TIMEOUT_SECONDS = 0.1
CANARY_CHECKS = [
    "server-valid",
    "relay-accepts",
    "document-keeps-probabilities",
    "confidence-formula",
    "state-size-probe",
]
OLLAMA_OVERSIZE_STATUS = 400
KEV_OVERSIZE_STATUS = 422
EARLIER_ATTEMPT_FILE = "response.attempt1.bin"
M3 = model_by_id("M3")
M4 = model_by_id("M4")
LONG_STATE_ID = alignment_request_id(ALIGNMENT_NODEID.format(0))


def free_port() -> int:
    with socket.socket() as probe:
        probe.bind((HOST, 0))
        return int(probe.getsockname()[1])


class RecordingSpawn:
    def __init__(self) -> None:
        self.processes: list[ManagedProcess] = []

    def __call__(self, command: ServerCommand) -> ManagedProcess:
        process = spawn_process(command)
        self.processes.append(process)
        return process

    def kill_survivors(self) -> None:
        for process in self.processes:
            if process.poll() is None:
                process.kill()


class StubBackedServer:
    """A `ModelServer` over an in-process stub: the transport that talks to it is the real one."""

    def __init__(self, stub: StubHttpServer) -> None:
        self._stub = stub
        self.stopped = False

    @property
    def endpoint_url(self) -> str:
        return f"{self._stub.base_url}{ROUTE}"

    def start(self, model: ModelSpec, run_reference: str | None) -> None:
        self._stub.start()

    def card(self) -> ServerCard:
        return ServerCard(OLLAMA_VERSION, DEFAULT_DIGEST, {}, {}, CONTEXT_LENGTH)

    def prefix_cache_hits(self) -> int | None:
        return None

    def stop(self) -> ShutdownReport:
        self._stub.stop()
        self.stopped = True
        return ShutdownReport(True, (), True, True)


class SingleServerFactory:
    def __init__(self, server: ModelServer) -> None:
        self._server = server

    def server_for(self, model: ModelSpec) -> ModelServer:
        return self._server


@dataclass
class Rig:
    world: RunWorld
    factory: ExperimentServerFactory
    spawn: RecordingSpawn
    clock: SettableClock
    sleeper: RecordingSleeper
    heavy: ClearHeavyProbe
    ollama_port: int
    kev_port: int

    def block(self, transport: ObservedTransport | None = None, servers: ServerFactory | None = None) -> ModelBlock:
        deps = BlockDeps(
            store=self.world.store,
            clock=self.clock,
            sleeper=self.sleeper,
            transport=transport or self.observed(),
            servers=servers or self.factory,
            tools=local_tools(),
            heavy=self.heavy,
            provenance=FixedProvenance(),
            harness=FixedHarness(),
            window=RunWindow.parse(WINDOW_TEXT),
            canary_closed_text=canary_closed_text(),
        )
        return ModelBlock(deps)

    def observed(
        self,
        *,
        timeout_seconds: float | None = None,
        after_posts: int | None = None,
        clock_to_close: SettableClock | None = None,
    ) -> ObservedTransport:
        return ObservedTransport(
            UrllibTransport(), timeout_seconds=timeout_seconds, after_posts=after_posts, clock_to_close=clock_to_close
        )

    def pass_ids(self, which: Pass) -> tuple[str, ...]:
        return self.world.schedule.ids_of(which)

    def request_count(self) -> int:
        return len(self.pass_ids(Pass.ONE)) + len(self.pass_ids(Pass.TWO))


def build_rig(tmp_path: Path, monkeypatch: pytest.MonkeyPatch, world: RunWorld) -> Rig:
    monkeypatch.setenv(TESTS_DIR_VARIABLE, str(TESTS_DIR))
    fakebin = install_fakebin(tmp_path / "fakebin")
    spawn = RecordingSpawn()
    ollama_port, kev_port = free_port(), free_port()
    factory = ExperimentServerFactory(
        UrllibTransport(),
        kev_directory=tmp_path,
        kev_commit=KEV_COMMIT,
        spawn=spawn,
        ollama_port=ollama_port,
        kev_port=kev_port,
        ollama_binary=str(fakebin / "ollama"),
        uv_binary=str(fakebin / "uv"),
        start_timeout_seconds=START_TIMEOUT,
        stop_grace_seconds=STOP_GRACE,
    )
    return Rig(world, factory, spawn, SettableClock(), RecordingSleeper(), ClearHeavyProbe(), ollama_port, kev_port)


@pytest.fixture
def rig(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> Iterator[Rig]:
    current = build_rig(tmp_path, monkeypatch, build_run_world(tmp_path / "run"))
    yield current
    current.spawn.kill_survivors()


def limit_states(monkeypatch: pytest.MonkeyPatch, model: ModelSpec) -> None:
    limit = OLLAMA_STATE_LIMIT if model.is_ollama else KEV_STATE_LIMIT
    monkeypatch.setenv(BEHAVIOUR_VARIABLE, json.dumps({"max_state_chars": limit}))


def read_meta(world: RunWorld, model: ModelSpec, which: Pass, request_id: str) -> RawMeta:
    return RawMeta.from_record(json.loads(world.store.read_text(raw_meta_path(model.model_id, which, request_id))))


def item_record(world: RunWorld, request_id: str) -> dict[str, object]:
    path = item_path(request_id)
    return dict(json_object(json.loads(world.store.read_text(path)), path))


def item_body(world: RunWorld, model: ModelSpec, request_id: str) -> bytes:
    bodies = json_object(item_record(world, request_id)["bodies"], "bodies")
    return str(bodies[model.identifier]).encode(ENCODING)


def reply_class_of(world: RunWorld, model: ModelSpec, which: Pass, request_id: str) -> ReplyClass:
    meta = read_meta(world, model, which, request_id)
    response = world.store.read_bytes(raw_response_path(model.model_id, which, request_id))
    specs = question_specs(item_record(world, request_id)["closed"])
    return classify_reply(model.flavour, specs, meta.http_status, response)


def assert_every_raw_file_exists(rig: Rig, model: ModelSpec) -> None:
    for which in (Pass.ONE, Pass.TWO):
        for request_id in rig.pass_ids(which):
            for path in (
                raw_request_path(model.model_id, which, request_id),
                raw_meta_path(model.model_id, which, request_id),
                raw_response_path(model.model_id, which, request_id),
            ):
                assert rig.world.store.exists(path), path
            sent = rig.world.store.read_bytes(raw_request_path(model.model_id, which, request_id))
            assert sent == item_body(rig.world, model, request_id)


def assert_server_gone(rig: Rig, port: int) -> None:
    assert port_is_free(HOST, port)
    assert rig.spawn.processes
    assert all(process.poll() is not None for process in rig.spawn.processes)


def shutdown_events(rig: Rig) -> list[dict[str, object]]:
    return [dict(event) for event in SafetyLog(rig.world.store).events() if event["event"] == EVENT_SHUTDOWN]


def deviation_lines(rig: Rig) -> list[str]:
    return [entry.render() for entry in DeviationLog(rig.world.store).entries()]


def pass_canary(rig: Rig, model: ModelSpec) -> None:
    rig.world.store.write_text(canary_path(model.model_id), document_json({"passed": True}))


def test_a_full_ollama_block_records_the_canary_both_passes_and_leaves_nothing_running(
    rig: Rig, monkeypatch: pytest.MonkeyPatch, tmp_path: Path
) -> None:
    limit_states(monkeypatch, M3)
    runner_file = tmp_path / "runner.pid"
    monkeypatch.setenv(RUNNER_PID_VARIABLE, str(runner_file))
    transport = rig.observed()

    outcome = rig.block(transport).run(M3)

    assert outcome is BlockOutcome.COMPLETE
    canary = json_object(json.loads(rig.world.store.read_text(canary_path(M3.model_id))), "canary")
    assert canary["passed"] is True
    checks = [json_object(check, "check")["name"] for check in sequence(canary["checks"], "checks")]
    assert checks == CANARY_CHECKS
    assert_every_raw_file_exists(rig, M3)
    for which in (Pass.ONE, Pass.TWO):
        for request_id in rig.pass_ids(which):
            assert reply_class_of(rig.world, M3, which, request_id) is ReplyClass.VALID
            assert read_meta(rig.world, M3, which, request_id).attempt == 1
    assert len(transport.posts) == CANARY_REQUESTS + rig.request_count()
    assert shutdown_events(rig)[0]["server_stopped"] is True
    assert deviation_lines(rig) == []
    assert TIMING_FILE in rig.world.store.list_dir(".")
    assert not is_alive(int(runner_file.read_text()))
    assert_server_gone(rig, rig.ollama_port)


def test_a_full_kev_block_uses_the_kev_formulas_and_records_the_prefix_cache_outcome(
    rig: Rig, monkeypatch: pytest.MonkeyPatch
) -> None:
    limit_states(monkeypatch, M4)

    outcome = rig.block().run(M4)

    assert outcome is BlockOutcome.COMPLETE
    canary = json_object(json.loads(rig.world.store.read_text(canary_path(M4.model_id))), "canary")
    assert canary["passed"] is True
    assert json_object(canary["probe"], "probe")["status"] == KEV_OVERSIZE_STATUS
    assert_every_raw_file_exists(rig, M4)
    first = {read_meta(rig.world, M4, Pass.ONE, rid).prefix_cache_hit for rid in rig.pass_ids(Pass.ONE)}
    second = {read_meta(rig.world, M4, Pass.TWO, rid).prefix_cache_hit for rid in rig.pass_ids(Pass.TWO)}
    assert first == {False}
    assert second == {True}
    assert deviation_lines(rig) == []
    assert_server_gone(rig, rig.kev_port)


@pytest.mark.parametrize("model", [M3, M4], ids=["ollama", "kev"])
def test_an_oversized_state_is_a_final_state_too_large_and_is_never_retried(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch, model: ModelSpec
) -> None:
    world = build_run_world(tmp_path / "run", long_state_for=LONG_STATE_ID, long_state_chars=KEV_STATE_LIMIT + 1)
    current = build_rig(tmp_path, monkeypatch, world)
    limit_states(monkeypatch, model)
    transport = current.observed()
    try:
        outcome = current.block(transport).run(model)
    finally:
        current.spawn.kill_survivors()

    assert outcome is BlockOutcome.COMPLETE
    passes = [which for which in (Pass.ONE, Pass.TWO) if LONG_STATE_ID in current.pass_ids(which)]
    assert transport.posts.count(item_body(world, model, LONG_STATE_ID)) == len(passes)
    for which in passes:
        meta = read_meta(world, model, which, LONG_STATE_ID)
        assert meta.attempt == 1
        assert meta.http_status == (OLLAMA_OVERSIZE_STATUS if model.is_ollama else KEV_OVERSIZE_STATUS)
        assert reply_class_of(world, model, which, LONG_STATE_ID) is ReplyClass.STATE_TOO_LARGE
        assert not world.store.exists(f"{raw_dir(model.model_id, which, LONG_STATE_ID)}/{EARLIER_ATTEMPT_FILE}")
    assert current.sleeper.sleeps == []


def test_an_invalid_reply_twice_is_missing_with_the_response_file_written(rig: Rig) -> None:
    pass_canary(rig, M3)
    server = StubBackedServer(StubOllamaServer(behaviour=OllamaBehaviour(invalid_reply=True)))
    transport = rig.observed()

    outcome = rig.block(transport, SingleServerFactory(server)).run(M3)

    assert outcome is BlockOutcome.COMPLETE
    request_id = rig.pass_ids(Pass.ONE)[0]
    meta = read_meta(rig.world, M3, Pass.ONE, request_id)
    assert meta.attempt == 2
    assert meta.http_status == 200
    assert rig.world.store.read_bytes(raw_response_path(M3.model_id, Pass.ONE, request_id)) == b"{not json"
    assert rig.world.store.exists(f"{raw_dir(M3.model_id, Pass.ONE, request_id)}/{EARLIER_ATTEMPT_FILE}")
    assert reply_class_of(rig.world, M3, Pass.ONE, request_id) is ReplyClass.INVALID
    assert len(transport.posts) == 2 * rig.request_count()
    assert len(rig.sleeper.sleeps) == rig.request_count()
    assert server.stopped


def test_a_hung_server_with_a_short_timeout_is_a_recorded_transport_failure(rig: Rig) -> None:
    pass_canary(rig, M3)
    server = StubBackedServer(StubOllamaServer(behaviour=OllamaBehaviour(hang_seconds=HANG_SECONDS)))
    transport = rig.observed(timeout_seconds=SHORT_TIMEOUT_SECONDS)

    outcome = rig.block(transport, SingleServerFactory(server)).run(M3)

    assert outcome is BlockOutcome.COMPLETE
    request_id = rig.pass_ids(Pass.ONE)[0]
    meta = read_meta(rig.world, M3, Pass.ONE, request_id)
    assert meta.http_status is None
    assert meta.transport_error is not None
    assert meta.attempt == 2
    assert rig.world.store.read_bytes(raw_response_path(M3.model_id, Pass.ONE, request_id)) == b""


def test_the_window_closing_after_some_requests_pauses_the_block_and_stops_the_server(rig: Rig) -> None:
    pass_canary(rig, M3)
    recorded = 3
    server = StubBackedServer(StubOllamaServer())
    transport = rig.observed(after_posts=recorded, clock_to_close=rig.clock)

    outcome = rig.block(transport, SingleServerFactory(server)).run(M3)

    assert outcome is BlockOutcome.WINDOW_CLOSED
    first = rig.pass_ids(Pass.ONE)
    recorded_flags = [rig.world.store.exists(raw_response_path(M3.model_id, Pass.ONE, rid)) for rid in first]
    assert recorded_flags == [index < recorded for index in range(len(first))]
    assert "- pause | scope: none | run window closed" in deviation_lines(rig)
    assert server.stopped


def test_resuming_after_a_crash_resends_only_the_request_without_a_response(
    rig: Rig, monkeypatch: pytest.MonkeyPatch
) -> None:
    limit_states(monkeypatch, M3)
    assert rig.block().run(M3) is BlockOutcome.COMPLETE
    lost = rig.pass_ids(Pass.TWO)[-1]
    (rig.world.store.root / raw_response_path(M3.model_id, Pass.TWO, lost)).unlink()
    transport = rig.observed()

    outcome = rig.block(transport).run(M3)

    assert outcome is BlockOutcome.COMPLETE
    assert transport.posts == [item_body(rig.world, M3, lost)]
    assert reply_class_of(rig.world, M3, Pass.TWO, lost) is ReplyClass.VALID
    assert_server_gone(rig, rig.ollama_port)


def test_a_finished_block_does_not_start_a_server_again(rig: Rig, monkeypatch: pytest.MonkeyPatch) -> None:
    limit_states(monkeypatch, M3)
    assert rig.block().run(M3) is BlockOutcome.COMPLETE
    started = len(rig.spawn.processes)

    assert rig.block().run(M3) is BlockOutcome.COMPLETE
    assert len(rig.spawn.processes) == started
