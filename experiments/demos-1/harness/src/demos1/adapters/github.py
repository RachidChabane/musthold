"""GitHub API access through `gh api`, every response saved; and the replay of saved responses."""

from __future__ import annotations

import json
import subprocess
from collections import defaultdict, deque
from collections.abc import Callable, Mapping
from datetime import datetime
from pathlib import Path, PurePosixPath
from typing import Final, cast

from demos1.application.ports import GitHubApi
from demos1.domain.api import NO_STATUS, RATE_LIMIT_HEADERS, ApiResponse
from demos1.domain.jsonvalue import JsonObject, as_object, encode, integer, optional_text, text
from demos1.domain.queries import ApiRequest
from demos1.domain.window import parse_utc

GH_TIMEOUT_SECONDS: Final = 300
STATUS_LINE_PREFIX: Final = "HTTP/"
RESPONSE_FILE: Final = "{seq:06d}.json"


def parse_included(output: str, fetched_at: datetime) -> ApiResponse:
    """The output of `gh api --include`: a status line, headers, a blank line, then the body."""
    text_output = output.replace("\r\n", "\n")
    if not text_output.startswith(STATUS_LINE_PREFIX):
        return ApiResponse(NO_STATUS, text_output, fetched_at)
    head, _, body = text_output.partition("\n\n")
    lines = head.split("\n")
    parts = lines[0].split()
    status = int(parts[1]) if len(parts) > 1 and parts[1].isdigit() else NO_STATUS
    headers: dict[str, str] = {}
    for line in lines[1:]:
        name, separator, value = line.partition(":")
        if separator:
            headers[name.strip().lower()] = value.strip()
    return ApiResponse(status, body, fetched_at, headers)


class GhCliApi:
    """`gh api --include --method GET`: the only code of the tooling that reaches the GitHub API."""

    def __init__(self, now: Callable[[], datetime]) -> None:
        self._now = now

    def argv(self, request: ApiRequest) -> list[str]:
        argv = ["gh", "api", "--include", "--method", "GET", request.path]
        for name, value in request.params.items():
            argv += ["-f", f"{name}={value}"]
        return argv

    def get(self, request: ApiRequest) -> ApiResponse:
        fetched_at = self._now()
        completed = subprocess.run(
            self.argv(request), capture_output=True, text=True, timeout=GH_TIMEOUT_SECONDS, check=False
        )
        response = parse_included(completed.stdout, fetched_at)
        if response.status == NO_STATUS:
            return ApiResponse(NO_STATUS, completed.stdout + completed.stderr, fetched_at)
        return response


def response_document(seq: int, request: ApiRequest, response: ApiResponse) -> JsonObject:
    """One saved response of PREREG 5: request, UTC fetch time, status, body and the body's SHA-256."""
    return {
        "seq": seq,
        "request": {"path": request.path, "params": dict(request.params)},
        "fetched_at": response.fetched_at.isoformat(),
        "status": response.status,
        "headers": {name: response.headers[name] for name in RATE_LIMIT_HEADERS if name in response.headers},
        "body": response.body,
        "body_sha256": response.body_sha256(),
    }


class RecordingApi:
    """Decorator that saves every response under `selection/responses/`, one file per request."""

    def __init__(self, inner: GitHubApi, directory: PurePosixPath) -> None:
        self._inner = inner
        self._directory = Path(directory)
        self._directory.mkdir(parents=True, exist_ok=True)
        self._seq = len(list(self._directory.iterdir()))

    def get(self, request: ApiRequest) -> ApiResponse:
        response = self._inner.get(request)
        self._seq += 1
        path = self._directory / RESPONSE_FILE.format(seq=self._seq)
        path.write_text(encode(response_document(self._seq, request, response)), encoding="utf-8")
        return response


class MissingResponse(RuntimeError):
    """A replay asked for a request that was never saved."""


class ReplayApi:
    """The saved responses, served again in the order they were saved, one per request."""

    def __init__(self, documents: list[JsonObject]) -> None:
        self._queues: dict[tuple[str, tuple[tuple[str, str], ...]], deque[ApiResponse]] = defaultdict(deque)
        for document in sorted(documents, key=lambda entry: integer(entry, "seq")):
            request = as_object(document["request"])
            params = cast(Mapping[str, str], as_object(request.get("params") or {}))
            key = ApiRequest(text(request, "path"), dict(params)).key()
            headers = cast(Mapping[str, str], as_object(document.get("headers") or {}))
            self._queues[key].append(
                ApiResponse(
                    integer(document, "status"),
                    text(document, "body"),
                    parse_utc(optional_text(document, "fetched_at") or "1970-01-01T00:00:00Z"),
                    dict(headers),
                )
            )

    @classmethod
    def from_directory(cls, directory: PurePosixPath) -> ReplayApi:
        documents = [
            as_object(json.loads(path.read_text(encoding="utf-8"))) for path in sorted(Path(directory).glob("*.json"))
        ]
        return cls(documents)

    def get(self, request: ApiRequest) -> ApiResponse:
        queue = self._queues.get(request.key())
        if not queue:
            raise MissingResponse(f"no saved response for {request.path} {dict(request.params)}")
        return queue.popleft()
