from __future__ import annotations

from collections.abc import Mapping

import pytest
from hypothesis import given
from hypothesis import strategies as st

from calibration.adapters.ollama_provisioner import ServerOllamaProvisioner, parse_parameters
from calibration.adapters.server_http import ServerError
from calibration.application.ports import ShutdownReport
from calibration.domain.canonical import JsonValue

CLEAN = ShutdownReport(True, (), True, True)
SHOW: Mapping[str, JsonValue] = {
    "parameters": 'num_ctx                        2048\ntemperature                    0\nstop "<|im_end|>"\nstop "<|im_start|>"',
    "details": {"family": "qwen", "parameter_size": "0.8B"},
}


class FakeServer:
    def __init__(self, report: ShutdownReport = CLEAN, fail_on: str | None = None) -> None:
        self.events: list[str] = []
        self._report = report
        self._fail_on = fail_on

    def start_empty(self) -> None:
        self.events.append("start")

    def version(self) -> str:
        return "0.35.1"

    def pull(self, tag: str) -> None:
        self.events.append(f"pull {tag}")
        if tag == self._fail_on:
            raise ServerError("pull failed")

    def show(self, tag: str) -> Mapping[str, JsonValue]:
        return SHOW

    def digest(self, tag: str) -> str:
        return "digest-" + tag

    def stop(self) -> ShutdownReport:
        self.events.append("stop")
        return self._report


def test_tags_are_pulled_and_recorded_between_start_and_stop() -> None:
    server = FakeServer()
    provision = ServerOllamaProvisioner(lambda: server).provision(["nimble:9b", "tev1:4b"])
    assert server.events == ["start", "pull nimble:9b", "pull tev1:4b", "stop"]
    assert provision.version == "0.35.1"
    record = provision.models["tev1:4b"]
    assert record.digest == "digest-tev1:4b"
    assert record.details == {"family": "qwen", "parameter_size": "0.8B"}
    assert record.parameters == {"num_ctx": 2048, "temperature": 0, "stop": ["<|im_end|>", "<|im_start|>"]}


def test_no_tags_does_not_start_a_server() -> None:
    created: list[FakeServer] = []

    def make() -> FakeServer:
        created.append(FakeServer())
        return created[-1]

    provision = ServerOllamaProvisioner(make).provision([])
    assert created == []
    assert provision.models == {}


def test_the_server_is_stopped_when_a_pull_fails() -> None:
    server = FakeServer(fail_on="tev1:4b")
    with pytest.raises(ServerError):
        ServerOllamaProvisioner(lambda: server).provision(["tev1:4b"])
    assert server.events[-1] == "stop"


def test_an_unclean_stop_is_an_error() -> None:
    with pytest.raises(ServerError):
        ServerOllamaProvisioner(lambda: FakeServer(ShutdownReport(True, (), True, False))).provision(["tev1:4b"])


def test_parameters_become_numbers_strings_and_lists() -> None:
    assert parse_parameters("num_ctx 2048\ntop_p 0.9\nname plain\n\n") == {
        "num_ctx": 2048,
        "top_p": 0.9,
        "name": "plain",
    }


@given(st.text())
def test_parameter_parsing_never_raises(text: str) -> None:
    assert isinstance(parse_parameters(text), dict)
