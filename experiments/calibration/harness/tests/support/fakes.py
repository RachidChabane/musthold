"""Test doubles of the ports: a scripted process runner and transport, and a clock that only the sleeper advances."""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path

from calibration.domain.ports import HttpReply, ProcessResult, TransportFailure


@dataclass(frozen=True)
class RunnerCall:
    argv: tuple[str, ...]
    timeout_seconds: float | None


class ScriptedRunner:
    """Answers a command by its argv; an `Exception` value is raised."""

    def __init__(self, answers: Mapping[tuple[str, ...], ProcessResult | Exception]) -> None:
        self._answers = dict(answers)
        self.calls: list[RunnerCall] = []

    def run(
        self,
        argv: Sequence[str],
        *,
        cwd: Path | None = None,
        env: Mapping[str, str] | None = None,
        unset: Sequence[str] = (),
        input_bytes: bytes | None = None,
        timeout_seconds: float | None = None,
    ) -> ProcessResult:
        key = tuple(argv)
        self.calls.append(RunnerCall(key, timeout_seconds))
        answer = self._answers[key]
        if isinstance(answer, Exception):
            raise answer
        return answer


def ok(stdout: str) -> ProcessResult:
    return ProcessResult(0, stdout.encode(), b"")


class ScriptedTransport:
    """Answers a GET by its URL; an unknown URL is a refused connection."""

    def __init__(self, replies: Mapping[str, HttpReply | TransportFailure]) -> None:
        self._replies = dict(replies)
        self.urls: list[str] = []

    def get(self, url: str, timeout_seconds: float, headers: Mapping[str, str] | None = None) -> HttpReply:
        self.urls.append(url)
        reply = self._replies.get(url, TransportFailure("connection refused"))
        if isinstance(reply, TransportFailure):
            raise reply
        return reply

    def post(self, url: str, body: bytes, timeout_seconds: float) -> HttpReply:
        raise AssertionError("no POST expected")


class FakeClock:
    """Time that only `FakeSleeper` moves."""

    def __init__(self) -> None:
        self.elapsed = 0.0

    def now(self) -> datetime:
        return datetime(2026, 10, 7, 21, 0, 0)

    def monotonic(self) -> float:
        return self.elapsed


class FakeSleeper:
    def __init__(self, clock: FakeClock) -> None:
        self._clock = clock
        self.sleeps: list[float] = []

    def sleep(self, seconds: float) -> None:
        self.sleeps.append(seconds)
        self._clock.elapsed += seconds
