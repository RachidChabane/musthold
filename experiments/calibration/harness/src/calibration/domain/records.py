"""The raw record of one request (§15.1): the bytes sent and received, verbatim, and the meta of the exchange."""

from __future__ import annotations

from dataclasses import dataclass

from calibration.domain.canonical import JsonValue
from calibration.domain.jsonread import mapping, optional_text, text


@dataclass(frozen=True)
class RawMeta:
    """`meta.json`: `{http_status, attempt, started, finished, latency_ms, server_version, model_digest}`.

    `http_status` is None when no reply came (a transport failure or a timeout). `attempt` is the attempt that
    produced the stored bytes, 1 or 2. `prefix_cache_hit` is the Kev prefix-cache outcome of the request.
    """

    http_status: int | None
    attempt: int
    started: str
    finished: str
    latency_ms: float
    server_version: str
    model_digest: str
    prefix_cache_hit: bool | None = None
    transport_error: str | None = None

    def to_record(self) -> dict[str, JsonValue]:
        return {
            "http_status": self.http_status,
            "attempt": self.attempt,
            "started": self.started,
            "finished": self.finished,
            "latency_ms": self.latency_ms,
            "server_version": self.server_version,
            "model_digest": self.model_digest,
            "prefix_cache_hit": self.prefix_cache_hit,
            "transport_error": self.transport_error,
        }

    @classmethod
    def from_record(cls, raw: object) -> RawMeta:
        body = mapping(raw, "meta")
        status = body.get("http_status")
        hit = body.get("prefix_cache_hit")
        latency = body.get("latency_ms")
        attempt = body.get("attempt")
        if not isinstance(attempt, int) or isinstance(attempt, bool):
            raise ValueError("meta.attempt must be an integer")
        return cls(
            http_status=status if isinstance(status, int) and not isinstance(status, bool) else None,
            attempt=attempt,
            started=text(body.get("started"), "meta.started"),
            finished=text(body.get("finished"), "meta.finished"),
            latency_ms=float(latency) if isinstance(latency, int | float) and not isinstance(latency, bool) else 0.0,
            server_version=text(body.get("server_version"), "meta.server_version"),
            model_digest=text(body.get("model_digest"), "meta.model_digest"),
            prefix_cache_hit=hit if isinstance(hit, bool) else None,
            transport_error=optional_text(body.get("transport_error"), "meta.transport_error"),
        )
