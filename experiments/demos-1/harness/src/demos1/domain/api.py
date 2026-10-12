"""One saved GitHub API response, and what the selection reads from its status and headers (PREREG 5)."""

from __future__ import annotations

from collections.abc import Mapping
from dataclasses import dataclass, field
from datetime import datetime
from typing import Final

from demos1.domain.jsonvalue import JsonValue, decode, sha256_hex

STATUS_OK_FIRST: Final = 200
STATUS_OK_LAST: Final = 299
RATE_LIMIT_STATUSES: Final = frozenset({403, 429})
RATE_LIMIT_HEADERS: Final = ("retry-after", "x-ratelimit-remaining", "x-ratelimit-reset")
RETRY_AFTER: Final = "retry-after"
RATE_LIMIT_REMAINING: Final = "x-ratelimit-remaining"
RATE_LIMIT_RESET: Final = "x-ratelimit-reset"
MIN_RATE_LIMIT_WAIT_SECONDS: Final = 60.0
"""The shortest wait before retrying a rate-limited request; a secondary limit names a reset that comes too soon."""
NO_STATUS: Final = 0
"""The status of a request that got no HTTP response at all."""


@dataclass(frozen=True)
class ApiResponse:
    status: int
    body: str
    fetched_at: datetime
    headers: Mapping[str, str] = field(default_factory=dict[str, str])

    def ok(self) -> bool:
        return STATUS_OK_FIRST <= self.status <= STATUS_OK_LAST

    def json(self) -> JsonValue:
        return decode(self.body)

    def body_sha256(self) -> str:
        return sha256_hex(self.body.encode())

    def header(self, name: str) -> str | None:
        return self.headers.get(name.lower())

    def is_rate_limited(self) -> bool:
        """HTTP 403 or 429 that carries a rate-limit header."""
        if self.status not in RATE_LIMIT_STATUSES:
            return False
        return any(self.header(name) is not None for name in RATE_LIMIT_HEADERS)

    def retry_wait_seconds(self, now: datetime) -> float:
        """How long until the reset time the response names, and never less than the shortest wait."""
        named = 0.0
        retry_after = self.header(RETRY_AFTER)
        reset = self.header(RATE_LIMIT_RESET)
        if retry_after is not None and retry_after.strip().isdigit():
            named = float(retry_after.strip())
        elif reset is not None and reset.strip().isdigit():
            named = float(reset.strip()) - now.timestamp()
        return max(MIN_RATE_LIMIT_WAIT_SECONDS, named)
