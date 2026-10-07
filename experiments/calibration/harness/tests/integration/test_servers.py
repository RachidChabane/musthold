"""The real `OllamaServer` and `KevServer` code paths against the fake `ollama` and `uv` executables."""

from __future__ import annotations

import json
import socket
import sys
import time
from collections.abc import Iterator
from pathlib import Path

import pytest

from calibration.adapters.http_transport import UrllibTransport
from calibration.adapters.kev_server import KevServer
from calibration.adapters.ollama_provisioner import ServerOllamaProvisioner
from calibration.adapters.ollama_server import OllamaServer
from calibration.adapters.process_tree import (
    ManagedProcess,
    ProcessTable,
    ServerCommand,
    descendants_of,
    is_alive,
    port_is_free,
    spawn_process,
)
from calibration.adapters.server_factory import ExperimentServerFactory
from calibration.adapters.server_http import ServerError, ServerStartError
from calibration.adapters.subprocess_runner import SubprocessRunner
from calibration.domain.constants import KEV_UNSET_ENVIRONMENT, OLLAMA_ENVIRONMENT
from calibration.domain.models import model_by_id
from calibration.domain.server_wire import ReplyClass, classify_reply, question_specs
from fixtures.wire_data import RECORDED_KEV_RUN, RECORDED_OLLAMA_CHOICE_REQUEST
from support.fakebin_install import TESTS_DIR, TESTS_DIR_VARIABLE, install_fakebin

pytestmark = pytest.mark.integration

HOST = "127.0.0.1"
START_TIMEOUT = 30.0
STOP_GRACE = 2.0
REQUEST_TIMEOUT = 10.0
KEV_COMMIT = "5e42a7a03f28134853dd3ff77461457e921e5ec1"
M3 = model_by_id("M3")
M4 = model_by_id("M4")


class RecordingSpawn:
    def __init__(self) -> None:
        self.processes: list[ManagedProcess] = []

    def __call__(self, command: ServerCommand) -> ManagedProcess:
        process = spawn_process(command)
        self.processes.append(process)
        return process


def free_port() -> int:
    with socket.socket() as probe:
        probe.bind((HOST, 0))
        return int(probe.getsockname()[1])


def wait_until_dead(pid: int) -> bool:
    deadline = time.monotonic() + 5
    while is_alive(pid) and time.monotonic() < deadline:
        time.sleep(0.05)
    return not is_alive(pid)


@pytest.fixture
def fakebin(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> Path:
    monkeypatch.setenv(TESTS_DIR_VARIABLE, str(TESTS_DIR))
    return install_fakebin(tmp_path / "fakebin")


@pytest.fixture
def spawn() -> RecordingSpawn:
    return RecordingSpawn()


@pytest.fixture
def ollama(fakebin: Path, spawn: RecordingSpawn) -> Iterator[OllamaServer]:
    server = OllamaServer(
        UrllibTransport(),
        spawn=spawn,
        binary=str(fakebin / "ollama"),
        port=free_port(),
        start_timeout_seconds=START_TIMEOUT,
        stop_grace_seconds=STOP_GRACE,
    )
    yield server
    server.stop()


@pytest.fixture
def kev(fakebin: Path, spawn: RecordingSpawn, tmp_path: Path) -> Iterator[KevServer]:
    server = KevServer(
        UrllibTransport(),
        kev_directory=tmp_path,
        kev_commit=KEV_COMMIT,
        spawn=spawn,
        uv_binary=str(fakebin / "uv"),
        port=free_port(),
        start_timeout_seconds=START_TIMEOUT,
        stop_grace_seconds=STOP_GRACE,
    )
    yield server
    server.stop()


def post_item(url: str, body: str = RECORDED_OLLAMA_CHOICE_REQUEST) -> tuple[int, bytes]:
    reply = UrllibTransport().post(url, body.encode(), REQUEST_TIMEOUT)
    return reply.status, reply.body


def kev_request(state: str = "the sky is green") -> str:
    document = json.loads(RECORDED_OLLAMA_CHOICE_REQUEST)
    document["model"] = M4.identifier
    document["state"] = state
    return json.dumps(document)


def test_ollama_end_to_end(
    ollama: OllamaServer, spawn: RecordingSpawn, tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    env_file = tmp_path / "env.json"
    runner_file = tmp_path / "runner.pid"
    monkeypatch.setenv("FAKE_OLLAMA_ENV_FILE", str(env_file))
    monkeypatch.setenv("FAKE_OLLAMA_RUNNER_PID_FILE", str(runner_file))
    port = int(ollama.endpoint_url.rsplit(":", 1)[1].split("/")[0])

    ollama.start(M3, None)
    assert ollama.endpoint_url == f"http://{HOST}:{port}/v1/systemone"
    assert ollama.card().loaded_context_tokens is None

    status, body = post_item(ollama.endpoint_url)
    assert status == 200
    specs = question_specs(_closed(RECORDED_OLLAMA_CHOICE_REQUEST))
    assert classify_reply(M3.flavour, specs, status, body) is ReplyClass.VALID

    card = ollama.card()
    assert card.server_version == "0.35.1"
    assert card.model_digest == "33aa" * 16
    assert card.loaded_context_tokens == 2048
    assert set(card.documents) == {"api/version", "api/show", "api/ps", "api/tags"}
    assert dict(card.environment) == {**dict(OLLAMA_ENVIRONMENT), "OLLAMA_HOST": f"{HOST}:{port}"}
    assert json.loads(env_file.read_text()) == dict(card.environment)
    assert ollama.prefix_cache_hits() is None

    runner_pid = int(runner_file.read_text())
    assert is_alive(runner_pid)
    root = spawn.processes[0]
    report = ollama.stop()
    assert report.clean
    assert root.poll() is not None
    assert wait_until_dead(runner_pid)
    assert port_is_free(HOST, port)


def _closed(request: str) -> dict[str, object]:
    questions = json.loads(request)["questions"]
    return {
        "questions": [
            {"id": key, "type": value["type"], "options": [{"name": name} for name in value["criteria"]]}
            for key, value in questions.items()
        ]
    }


def test_ollama_stop_ends_a_runner_that_survives_the_unload_and_ignores_sigterm(
    ollama: OllamaServer, tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    runner_file = tmp_path / "runner.pid"
    monkeypatch.setenv("FAKE_OLLAMA_RUNNER_PID_FILE", str(runner_file))
    monkeypatch.setenv("FAKE_OLLAMA_KEEP_RUNNER", "1")
    monkeypatch.setenv("FAKE_OLLAMA_STUBBORN_RUNNER", "1")
    ollama.start(M3, None)
    post_item(ollama.endpoint_url)
    runner_pid = int(runner_file.read_text())

    assert ollama.stop().clean
    assert wait_until_dead(runner_pid)


def test_ollama_stop_without_a_loaded_model_is_clean(ollama: OllamaServer) -> None:
    ollama.start_empty()
    assert ollama.stop().clean


def test_ollama_stop_after_a_crash_is_clean_and_leaves_the_port_free(
    ollama: OllamaServer, spawn: RecordingSpawn
) -> None:
    ollama.start(M3, None)
    spawn.processes[0].kill()
    spawn.processes[0].wait(10)
    report = ollama.stop()
    assert report.clean


def test_ollama_start_refuses_a_busy_port(fakebin: Path) -> None:
    with socket.socket() as listener:
        listener.bind((HOST, 0))
        listener.listen(1)
        server = OllamaServer(UrllibTransport(), binary=str(fakebin / "ollama"), port=int(listener.getsockname()[1]))
        with pytest.raises(ServerStartError):
            server.start_empty()


def test_ollama_start_fails_fast_when_the_process_exits() -> None:
    server = OllamaServer(
        UrllibTransport(), binary=sys.executable, port=free_port(), start_timeout_seconds=START_TIMEOUT
    )
    started = time.monotonic()
    with pytest.raises(ServerStartError, match="exited"):
        server.start_empty()
    assert time.monotonic() - started < START_TIMEOUT / 2


def test_ollama_pull_show_and_digest(ollama: OllamaServer) -> None:
    ollama.start_empty()
    ollama.pull("nimble:9b")
    shown = ollama.show("nimble:9b")
    assert "num_ctx" in str(shown["parameters"])
    assert len(ollama.digest("nimble:9b")) == 64
    assert ollama.version() == "0.35.1"
    with pytest.raises(ServerError):
        ollama.show("absent:1b")


def test_kev_end_to_end(kev: KevServer, spawn: RecordingSpawn, tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    record = tmp_path / "uv.json"
    monkeypatch.setenv("FAKE_UV_RECORD_FILE", str(record))
    for name in KEV_UNSET_ENVIRONMENT:
        monkeypatch.setenv(name, "inherited")
    monkeypatch.delenv("HF_HUB_OFFLINE", raising=False)
    port = int(kev.endpoint_url.rsplit(":", 1)[1].split("/")[0])

    kev.start(M4, RECORDED_KEV_RUN)

    seen = json.loads(record.read_text())
    assert seen["argv"] == [
        *["run", "--frozen", "--extra", "serve", "python", "-m", "kev.serve", "--run"],
        RECORDED_KEV_RUN,
        "--host",
        HOST,
        "--port",
        str(port),
    ]
    assert Path(seen["cwd"]).resolve() == tmp_path.resolve()
    assert seen["environment"] == {"HF_HUB_OFFLINE": "1"}

    assert kev.prefix_cache_hits() == 0
    status, body = post_item(kev.endpoint_url, kev_request())
    assert status == 200
    assert json.loads(body)["model"] == "kev-0.8b"
    assert kev.prefix_cache_hits() == 0
    post_item(kev.endpoint_url, kev_request())
    assert kev.prefix_cache_hits() == 1

    card = kev.card()
    assert card.server_version == KEV_COMMIT
    assert card.model_digest == "0123456789abcdef0123456789abcdef01234567"
    assert card.loaded_context_tokens == 65536
    assert "v1/models" in card.documents

    root = spawn.processes[0]
    children = descendants_of(ProcessTable(SubprocessRunner()).snapshot(), root.pid)
    assert children
    report = kev.stop()
    assert report.clean
    assert root.poll() is not None
    assert all(wait_until_dead(pid) for pid in children)
    assert port_is_free(HOST, port)


def test_kev_start_needs_a_run_reference_with_a_revision(kev: KevServer) -> None:
    with pytest.raises(ServerError):
        kev.start(M4, None)
    with pytest.raises(ServerError):
        kev.start(M4, "jaredpalmer/kev-0.8b")


def test_kev_start_fails_when_the_server_exits(fakebin: Path, tmp_path: Path) -> None:
    server = KevServer(
        UrllibTransport(),
        kev_directory=tmp_path,
        kev_commit=KEV_COMMIT,
        uv_binary=str(fakebin / "ollama"),
        port=free_port(),
        start_timeout_seconds=START_TIMEOUT,
    )
    with pytest.raises(ServerStartError):
        server.start(M4, RECORDED_KEV_RUN)


def test_the_factory_builds_each_flavour(fakebin: Path, tmp_path: Path) -> None:
    factory = ExperimentServerFactory(
        UrllibTransport(), kev_directory=tmp_path, kev_commit=KEV_COMMIT, ollama_port=11111, kev_port=22222
    )
    assert isinstance(factory.server_for(M3), OllamaServer)
    assert isinstance(factory.server_for(M4), KevServer)
    assert factory.server_for(M3).endpoint_url == f"http://{HOST}:11111/v1/systemone"
    assert factory.server_for(M4).endpoint_url == f"http://{HOST}:22222/v1/systemone"


def test_provisioning_pulls_through_the_real_server_and_leaves_nothing_behind(
    ollama: OllamaServer, spawn: RecordingSpawn
) -> None:
    provision = ServerOllamaProvisioner(lambda: ollama).provision(["nimble:9b", "tev1:4b"])
    assert provision.version == "0.35.1"
    assert provision.models["nimble:9b"].parameters["num_ctx"] == 2048
    assert provision.models["tev1:4b"].details["family"] == "qwen"
    assert spawn.processes[0].poll() is not None


def test_provisioning_no_tags_starts_no_process(ollama: OllamaServer, spawn: RecordingSpawn) -> None:
    ServerOllamaProvisioner(lambda: ollama).provision([])
    assert spawn.processes == []
