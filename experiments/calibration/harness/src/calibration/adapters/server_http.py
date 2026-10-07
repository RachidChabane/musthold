"""JSON over the `HttpTransport` for the experiment's own servers: a non-200 reply or a body that is no JSON is an error."""

from __future__ import annotations

import json
from collections.abc import Mapping
from typing import Final

from calibration.domain.canonical import JsonValue
from calibration.domain.constants import HTTP_OK
from calibration.domain.ports import HttpTransport, TransportFailure

API_TIMEOUT_SECONDS: Final = 30.0
ENCODING: Final = "utf-8"


class ServerError(RuntimeError):
    """A server that did not answer as the experiment requires."""


class ServerStartError(ServerError):
    """A server that did not come up, or came up in a state a block cannot start from."""


class JsonClient:
    def __init__(self, transport: HttpTransport, base_url: str, timeout_seconds: float = API_TIMEOUT_SECONDS) -> None:
        self._transport = transport
        self._base_url = base_url
        self._timeout_seconds = timeout_seconds

    @property
    def base_url(self) -> str:
        return self._base_url

    def status_of(self, path: str) -> int | None:
        """The HTTP status of a GET, None when no reply came."""
        try:
            return self._transport.get(self._base_url + path, self._timeout_seconds).status
        except TransportFailure:
            return None

    def get(self, path: str) -> JsonValue:
        try:
            reply = self._transport.get(self._base_url + path, self._timeout_seconds)
        except TransportFailure as failure:
            raise ServerError(f"GET {path}: {failure.reason}") from failure
        return self._decode("GET", path, reply.status, reply.body)

    def post(self, path: str, payload: JsonValue, timeout_seconds: float | None = None) -> JsonValue:
        body = json.dumps(payload).encode(ENCODING)
        timeout = self._timeout_seconds if timeout_seconds is None else timeout_seconds
        try:
            reply = self._transport.post(self._base_url + path, body, timeout)
        except TransportFailure as failure:
            raise ServerError(f"POST {path}: {failure.reason}") from failure
        return self._decode("POST", path, reply.status, reply.body)

    @staticmethod
    def _decode(verb: str, path: str, status: int, body: bytes) -> JsonValue:
        if status != HTTP_OK:
            raise ServerError(f"{verb} {path}: HTTP status {status}")
        try:
            document: JsonValue = json.loads(body)
        except ValueError as error:
            raise ServerError(f"{verb} {path}: the body is not JSON") from error
        return document


def object_of(document: JsonValue, where: str) -> Mapping[str, JsonValue]:
    """The document as a JSON object; a server error when it is anything else."""
    if not isinstance(document, Mapping):
        raise ServerError(f"{where} is not a JSON object")
    return document
