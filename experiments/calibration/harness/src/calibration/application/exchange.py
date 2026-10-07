"""One request to a `/v1/systemone` server with the retry rule of §4.6."""

from __future__ import annotations

from dataclasses import dataclass

from calibration.domain.constants import MAX_ATTEMPTS, REQUEST_TIMEOUT_SECONDS, RETRY_DELAY_SECONDS
from calibration.domain.models import Flavour
from calibration.domain.ports import Clock, HttpTransport, Sleeper, TransportFailure
from calibration.domain.server_wire import QuestionSpec, ReplyClass, classify_reply


@dataclass(frozen=True)
class Attempt:
    """What one post left behind: the status and bytes received, or the transport failure."""

    status: int | None
    body: bytes
    transport_error: str | None
    started: str
    finished: str
    latency_ms: float


@dataclass(frozen=True)
class ExchangeResult:
    """The attempt whose bytes are stored, its number and the class of its reply; an earlier attempt is kept apart."""

    final: Attempt
    number: int
    reply_class: ReplyClass | None
    earlier: Attempt | None

    @property
    def missing(self) -> bool:
        """A request that fails twice, or whose last attempt is no valid reply, is MISSING."""
        return self.reply_class is ReplyClass.INVALID or self.reply_class is None


class Exchange:
    """Posts a body, classifies the reply and retries once after 5 s when it is not server-valid.

    A STATE_TOO_LARGE reply is final: it is never retried.
    """

    def __init__(
        self,
        transport: HttpTransport,
        clock: Clock,
        sleeper: Sleeper,
        flavour: Flavour,
        timeout_seconds: float = REQUEST_TIMEOUT_SECONDS,
        retry_delay_seconds: float = RETRY_DELAY_SECONDS,
    ) -> None:
        self._transport = transport
        self._clock = clock
        self._sleeper = sleeper
        self._flavour = flavour
        self._timeout = timeout_seconds
        self._retry_delay = retry_delay_seconds

    def send(self, url: str, body: bytes, specs: tuple[QuestionSpec, ...]) -> ExchangeResult:
        first = self._post(url, body)
        first_class = self._classify(first, specs)
        if first_class in (ReplyClass.VALID, ReplyClass.STATE_TOO_LARGE):
            return ExchangeResult(first, 1, first_class, None)
        self._sleeper.sleep(self._retry_delay)
        second = self._post(url, body)
        second_class = self._classify(second, specs)
        return ExchangeResult(second, MAX_ATTEMPTS, second_class, first)

    def _classify(self, attempt: Attempt, specs: tuple[QuestionSpec, ...]) -> ReplyClass | None:
        if attempt.transport_error is not None:
            return None
        return classify_reply(self._flavour, specs, attempt.status, attempt.body)

    def _post(self, url: str, body: bytes) -> Attempt:
        started = self._clock.now().isoformat()
        begun = self._clock.monotonic()
        try:
            reply = self._transport.post(url, body, self._timeout)
        except TransportFailure as failure:
            return self._attempt(None, b"", failure.reason, started, begun)
        return self._attempt(reply.status, reply.body, None, started, begun)

    def _attempt(self, status: int | None, body: bytes, error: str | None, started: str, begun: float) -> Attempt:
        latency = (self._clock.monotonic() - begun) * 1000.0
        return Attempt(status, body, error, started, self._clock.now().isoformat(), latency)
