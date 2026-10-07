"""The common part of the stub servers: a threaded HTTP server on an ephemeral port, a request log and the closed questions.

Only the standard library is used, so that a fake executable can import it.
"""

from __future__ import annotations

import hashlib
import json
import threading
from collections.abc import Mapping
from dataclasses import dataclass, field
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from typing import Any

LOOPBACK = "127.0.0.1"
JSON_TYPE = "application/json"
HASH_WORD_BYTES = 4
SERVER_THREAD_JOIN_SECONDS = 10.0
SHUTDOWN_POLL_SECONDS = 0.02


@dataclass(frozen=True)
class RecordedRequest:
    method: str
    path: str
    body: bytes
    headers: Mapping[str, str]

    def json(self) -> Any:
        return json.loads(self.body)


@dataclass(frozen=True)
class StubReply:
    status: int
    body: bytes
    delay_seconds: float = 0.0
    content_type: str = JSON_TYPE


def json_reply(status: int, document: object, delay_seconds: float = 0.0) -> StubReply:
    return StubReply(status, json.dumps(document, ensure_ascii=False).encode("utf-8"), delay_seconds)


class StubHttpServer:
    """Serves `respond` on `host:port` (0 picks a free port) from a thread; every request is logged."""

    def __init__(self, host: str = LOOPBACK, port: int = 0) -> None:
        self.requests: list[RecordedRequest] = []
        self._lock = threading.Lock()
        self._release = threading.Event()
        stub = self

        class Handler(BaseHTTPRequestHandler):
            protocol_version = "HTTP/1.1"

            def _serve(self) -> None:
                length = int(self.headers.get("Content-Length") or 0)
                body = self.rfile.read(length) if length else b""
                request = RecordedRequest(
                    self.command, self.path, body, {key.lower(): value for key, value in self.headers.items()}
                )
                with stub._lock:
                    stub.requests.append(request)
                reply = stub.respond(request)
                if reply.delay_seconds:
                    stub._release.wait(reply.delay_seconds)
                try:
                    self.send_response(reply.status)
                    self.send_header("Content-Type", reply.content_type)
                    self.send_header("Content-Length", str(len(reply.body)))
                    self.end_headers()
                    self.wfile.write(reply.body)
                except OSError:
                    return

            do_GET = _serve  # noqa: N815
            do_POST = _serve  # noqa: N815

            def log_message(self, format: str, *args: object) -> None:
                return

        self._server = ThreadingHTTPServer((host, port), Handler)
        self._server.daemon_threads = True
        self._thread = threading.Thread(target=self._serve, daemon=True)

    @property
    def port(self) -> int:
        return int(self._server.server_address[1])

    @property
    def base_url(self) -> str:
        return f"http://{LOOPBACK}:{self.port}"

    def respond(self, request: RecordedRequest) -> StubReply:
        raise NotImplementedError

    def requests_to(self, path: str) -> list[RecordedRequest]:
        with self._lock:
            return [request for request in self.requests if request.path == path]

    def start(self) -> StubHttpServer:
        self._thread.start()
        return self

    def _serve(self) -> None:
        self._server.serve_forever(poll_interval=SHUTDOWN_POLL_SECONDS)

    def serve_forever_blocking(self) -> None:
        self._serve()

    def stop(self) -> None:
        self._release.set()
        self._server.shutdown()
        self._server.server_close()
        if self._thread.is_alive():
            self._thread.join(SERVER_THREAD_JOIN_SECONDS)

    def __enter__(self) -> StubHttpServer:
        return self.start()

    def __exit__(self, *_: object) -> None:
        self.stop()


@dataclass(frozen=True)
class StubQuestion:
    question_id: str
    question_type: str
    options: tuple[str, ...]
    legend: Mapping[str, str] = field(default_factory=dict[str, str])


def _text(value: object) -> str:
    return value if isinstance(value, str) else json.dumps(value, sort_keys=True)


def parse_questions(request_body: bytes) -> list[StubQuestion]:
    """The closed questions of a `/v1/systemone` request: options by name (choice), by level (score), false/true (noul)."""
    questions = json.loads(request_body)["questions"]
    parsed: list[StubQuestion] = []
    for question_id, question in questions.items():
        kind = question["type"]
        if kind == "choice":
            parsed.append(StubQuestion(question_id, kind, tuple(question["criteria"])))
        elif kind == "score":
            levels = [_text(level) for level in question["criteria"]]
            legend = {str(index): level for index, level in enumerate(levels)}
            parsed.append(StubQuestion(question_id, kind, tuple(str(index) for index in range(len(levels))), legend))
        else:
            parsed.append(StubQuestion(question_id, kind, ("false", "true")))
    return parsed


def state_text(request_body: bytes) -> str:
    return _text(json.loads(request_body).get("state"))


def derived_probabilities(state: str, question_id: str, count: int) -> list[float]:
    """A deterministic distribution over `count` options, a function of sha256(state + question id)."""
    weights = [
        int.from_bytes(
            hashlib.sha256(f"{state}\x00{question_id}\x00{index}".encode()).digest()[:HASH_WORD_BYTES], "big"
        )
        + 1
        for index in range(count)
    ]
    total = sum(weights)
    return [weight / total for weight in weights]
