from __future__ import annotations

import socket

import pytest
from hypothesis import HealthCheck, given, settings
from hypothesis import strategies as st

from calibration.adapters.http_transport import UrllibTransport
from calibration.domain.ports import TransportFailure
from support.stub_common import RecordedRequest, StubHttpServer, StubReply

SHORT_TIMEOUT = 0.3
TIMEOUT = 5.0


class ScriptedStub(StubHttpServer):
    def __init__(self, reply: StubReply) -> None:
        super().__init__()
        self.reply = reply

    def respond(self, request: RecordedRequest) -> StubReply:
        return self.reply


def free_port() -> int:
    with socket.socket() as probe:
        probe.bind(("127.0.0.1", 0))
        return int(probe.getsockname()[1])


@pytest.mark.parametrize("status", [200, 201, 400, 404, 413, 422, 500, 503])
def test_every_status_is_a_reply(status: int) -> None:
    with ScriptedStub(StubReply(status, b'{"x":1}')) as stub:
        reply = UrllibTransport().get(stub.base_url + "/p", TIMEOUT)
    assert (reply.status, reply.body) == (status, b'{"x":1}')


def test_a_redirect_is_not_followed() -> None:
    with ScriptedStub(StubReply(302, b"moved")) as stub:
        reply = UrllibTransport().get(stub.base_url + "/p", TIMEOUT)
        assert reply.status == 302
        assert [request.path for request in stub.requests] == ["/p"]


def test_post_sends_the_body_unchanged_as_json() -> None:
    body = b'{"b":1,  "a":"\xc3\xa9"}'
    with ScriptedStub(StubReply(200, b"{}")) as stub:
        UrllibTransport().post(stub.base_url + "/q", body, TIMEOUT)
        (request,) = stub.requests
    assert request.method == "POST"
    assert request.body == body
    assert request.headers["content-type"] == "application/json"


@given(body=st.binary(max_size=2000))
@settings(max_examples=25, deadline=None, suppress_health_check=[HealthCheck.function_scoped_fixture])
def test_any_body_bytes_travel_unchanged(body: bytes) -> None:
    with ScriptedStub(StubReply(200, b"{}")) as stub:
        UrllibTransport().post(stub.base_url + "/q", body, TIMEOUT)
        assert stub.requests[0].body == body


def test_get_headers_reach_the_server() -> None:
    with ScriptedStub(StubReply(200, b"{}")) as stub:
        UrllibTransport().get(stub.base_url + "/h", TIMEOUT, {"Accept": "application/vnd.test"})
        assert stub.requests[0].headers["accept"] == "application/vnd.test"


def test_a_refused_connection_is_a_transport_failure() -> None:
    with pytest.raises(TransportFailure) as raised:
        UrllibTransport().get(f"http://127.0.0.1:{free_port()}/", TIMEOUT)
    assert not raised.value.timed_out


def test_a_reply_slower_than_the_timeout_is_a_timeout_failure() -> None:
    with ScriptedStub(StubReply(200, b"{}", delay_seconds=3.0)) as stub, pytest.raises(TransportFailure) as raised:
        UrllibTransport().get(stub.base_url + "/slow", SHORT_TIMEOUT)
    assert raised.value.timed_out


def test_a_reset_connection_is_a_transport_failure() -> None:
    with socket.socket() as listener:
        listener.bind(("127.0.0.1", 0))
        listener.listen(1)
        port = int(listener.getsockname()[1])
        import threading

        def accept_and_close() -> None:
            connection, _ = listener.accept()
            connection.close()

        thread = threading.Thread(target=accept_and_close)
        thread.start()
        with pytest.raises(TransportFailure):
            UrllibTransport().get(f"http://127.0.0.1:{port}/", TIMEOUT)
        thread.join()


def test_a_malformed_reply_is_a_transport_failure() -> None:
    import threading

    with socket.socket() as listener:
        listener.bind(("127.0.0.1", 0))
        listener.listen(1)
        port = int(listener.getsockname()[1])

        def answer_garbage() -> None:
            connection, _ = listener.accept()
            connection.recv(4096)
            connection.sendall(b"this is not http\r\n\r\n")
            connection.close()

        thread = threading.Thread(target=answer_garbage)
        thread.start()
        with pytest.raises(TransportFailure):
            UrllibTransport().get(f"http://127.0.0.1:{port}/", TIMEOUT)
        thread.join()


def test_proxy_environment_is_ignored(monkeypatch: pytest.MonkeyPatch) -> None:
    dead_proxy = f"http://127.0.0.1:{free_port()}"
    for name in ("HTTP_PROXY", "http_proxy", "ALL_PROXY", "all_proxy"):
        monkeypatch.setenv(name, dead_proxy)
    monkeypatch.delenv("NO_PROXY", raising=False)
    monkeypatch.delenv("no_proxy", raising=False)
    with ScriptedStub(StubReply(200, b"{}")) as stub:
        reply = UrllibTransport().get(stub.base_url + "/direct", TIMEOUT)
    assert reply.status == 200
