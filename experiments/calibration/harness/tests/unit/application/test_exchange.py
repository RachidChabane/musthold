from __future__ import annotations

import pytest

from calibration.application.exchange import Exchange, ExchangeResult
from calibration.domain.constants import REQUEST_TIMEOUT_SECONDS, RETRY_DELAY_SECONDS
from calibration.domain.models import Flavour
from calibration.domain.ports import HttpReply, TransportFailure
from calibration.domain.server_wire import QuestionSpec, ReplyClass
from support.application_fakes import ScriptedPostTransport, queued_replies
from support.fakes import FakeClock, FakeSleeper
from support.run_directory_seed import (
    INVALID_REPLY,
    ITEM_QUESTION_ID,
    ITEM_REPLY,
    KEV_STATE_TOO_LARGE_BODY,
    OLLAMA_PROMPT_TOO_LONG_BODY,
    too_large_reply,
)

URL = "http://server.invalid/v1/systemone"
BODY = b'{"request":"verbatim"}'
SPECS = (QuestionSpec(ITEM_QUESTION_ID, "choice", ("A", "B")),)
SECOND_VALID = HttpReply(200, ITEM_REPLY.body + b" ")


class Rig:
    def __init__(self, flavour: Flavour, replies: list[HttpReply | TransportFailure]) -> None:
        self.clock = FakeClock()
        self.sleeper = FakeSleeper(self.clock)
        self.transport = ScriptedPostTransport(queued_replies(replies), lambda _body: self._tick())
        self.exchange = Exchange(self.transport, self.clock, self.sleeper, flavour)
        self.latency_seconds = 0.0

    def _tick(self) -> None:
        self.clock.elapsed += self.latency_seconds

    def send(self) -> ExchangeResult:
        return self.exchange.send(URL, BODY, SPECS)


def ollama(*replies: HttpReply | TransportFailure) -> Rig:
    return Rig(Flavour.OLLAMA, list(replies))


def test_a_valid_reply_is_final_at_the_first_attempt() -> None:
    rig = ollama(ITEM_REPLY)

    result = rig.send()

    assert (result.number, result.reply_class, result.earlier) == (1, ReplyClass.VALID, None)
    assert result.final.body == ITEM_REPLY.body
    assert rig.sleeper.sleeps == []
    assert not result.missing


def test_the_body_is_posted_unchanged_with_the_request_timeout() -> None:
    rig = ollama(ITEM_REPLY)

    rig.send()

    assert rig.transport.posts == [BODY]
    assert rig.transport.urls == [URL]
    assert rig.transport.timeouts == [REQUEST_TIMEOUT_SECONDS]


def test_an_invalid_reply_is_retried_once_after_the_retry_delay() -> None:
    rig = ollama(INVALID_REPLY, SECOND_VALID)

    result = rig.send()

    assert rig.sleeper.sleeps == [RETRY_DELAY_SECONDS]
    assert rig.transport.posts == [BODY, BODY]
    assert (result.number, result.reply_class) == (2, ReplyClass.VALID)
    assert result.final.body == SECOND_VALID.body
    assert not result.missing


def test_the_bytes_of_the_first_attempt_are_kept_apart() -> None:
    rig = ollama(INVALID_REPLY, SECOND_VALID)

    result = rig.send()

    assert result.earlier is not None
    assert (result.earlier.status, result.earlier.body) == (INVALID_REPLY.status, INVALID_REPLY.body)
    assert result.earlier.transport_error is None


def test_a_transport_failure_is_retried_and_its_reason_is_kept() -> None:
    rig = ollama(TransportFailure("connection reset"), ITEM_REPLY)

    result = rig.send()

    assert result.number == 2
    assert result.earlier is not None
    assert (result.earlier.status, result.earlier.body, result.earlier.transport_error) == (
        None,
        b"",
        "connection reset",
    )
    assert rig.sleeper.sleeps == [RETRY_DELAY_SECONDS]


def test_a_request_that_fails_twice_is_missing_and_keeps_both_attempts() -> None:
    second = HttpReply(502, b"second failure")
    rig = ollama(INVALID_REPLY, second)

    result = rig.send()

    assert result.missing
    assert (result.number, result.reply_class) == (2, ReplyClass.INVALID)
    assert result.final.body == second.body
    assert result.earlier is not None
    assert result.earlier.body == INVALID_REPLY.body


def test_two_transport_failures_are_missing_without_a_reply_class() -> None:
    rig = ollama(TransportFailure("refused"), TransportFailure("timed out", timed_out=True))

    result = rig.send()

    assert result.missing
    assert result.reply_class is None
    assert result.final.transport_error == "timed out"
    assert rig.sleeper.sleeps == [RETRY_DELAY_SECONDS]


def test_there_is_never_a_third_attempt() -> None:
    rig = ollama(INVALID_REPLY, INVALID_REPLY)

    rig.send()

    assert len(rig.transport.posts) == 2


@pytest.mark.parametrize(
    ("flavour", "refusal"),
    [
        (Flavour.OLLAMA, too_large_reply(Flavour.OLLAMA)),
        (Flavour.OLLAMA, HttpReply(400, OLLAMA_PROMPT_TOO_LONG_BODY)),
        (Flavour.KEV, HttpReply(422, KEV_STATE_TOO_LARGE_BODY)),
    ],
)
def test_a_state_too_large_reply_is_final_and_never_retried(flavour: Flavour, refusal: HttpReply) -> None:
    rig = Rig(flavour, [refusal])

    result = rig.send()

    assert (result.number, result.reply_class, result.earlier) == (1, ReplyClass.STATE_TOO_LARGE, None)
    assert rig.sleeper.sleeps == []
    assert len(rig.transport.posts) == 1
    assert not result.missing


def test_a_state_too_large_reply_at_the_retry_ends_the_request_without_a_third_attempt() -> None:
    rig = ollama(INVALID_REPLY, too_large_reply(Flavour.OLLAMA))

    result = rig.send()

    assert (result.number, result.reply_class) == (2, ReplyClass.STATE_TOO_LARGE)
    assert not result.missing
    assert len(rig.transport.posts) == 2


def test_the_flavour_decides_which_refusal_is_a_state_too_large_reply() -> None:
    rig = Rig(Flavour.KEV, [too_large_reply(Flavour.OLLAMA), INVALID_REPLY])

    result = rig.send()

    assert result.reply_class is ReplyClass.INVALID
    assert len(rig.transport.posts) == 2


def test_the_latency_is_that_of_the_attempt_whose_bytes_are_stored() -> None:
    rig = ollama(INVALID_REPLY, ITEM_REPLY)
    rig.latency_seconds = 2.5

    result = rig.send()

    assert result.final.latency_ms == pytest.approx(2500.0)
    assert result.earlier is not None
    assert result.earlier.latency_ms == pytest.approx(2500.0)


def test_each_attempt_stamps_its_start_and_finish() -> None:
    rig = ollama(ITEM_REPLY)

    result = rig.send()

    assert result.final.started == rig.clock.now().isoformat()
    assert result.final.finished == rig.clock.now().isoformat()
