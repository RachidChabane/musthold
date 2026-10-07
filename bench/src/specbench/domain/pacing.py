"""Pacing: the nightly run window, the per-night call budget per CLI, and quota pauses that a rerun resumes.

A night is identified by the local date on which its window opened. A resumed call
(its raw record already exists) costs nothing, so a rerun after a pause spends only
on calls that were never made. A pause ends the process with `EXIT_PAUSED`.
"""

from __future__ import annotations

import re
from collections.abc import Mapping
from dataclasses import dataclass
from datetime import date, datetime, time, timedelta
from enum import Enum
from typing import Final

from specbench.domain.arms import Harness
from specbench.domain.canonical import JsonValue

EXIT_PAUSED: Final = 3
RUN_WINDOW_ENV: Final = "SPECBENCH_RUN_WINDOW"
DEFAULT_RUN_WINDOW: Final = "21:00-07:30"
ALWAYS_OPEN: Final = "always"
DEFAULT_CALL_BUDGET: Final = 250
MAX_SESSIONS_IN_FLIGHT: Final = 2
MAX_HEAVY_WORKERS: Final = 2
CALL_LEDGER_FILE_NAME: Final = "call_budget.json"
_WINDOW_PATTERN = re.compile(r"^(\d{1,2}):(\d{2})-(\d{1,2}):(\d{2})$")
QUOTA_PATTERNS: Final = (
    r"usage limit",
    r"rate[ _-]?limit",
    r"\bquota\b",
    r"insufficient_quota",
    r"too many requests",
    r"\b429\b",
    r"hit your limit",
    r"limit (has been )?reached",
    r"credit balance",
    r"resource.?exhausted",
)
_QUOTA_REGEX: Final = re.compile("|".join(QUOTA_PATTERNS), re.IGNORECASE)


class PauseReason(Enum):
    WINDOW_CLOSED = "run window closed"
    BUDGET_REACHED = "call budget reached"
    QUOTA_EXHAUSTED = "quota exhausted"


class RunPaused(Exception):
    """The run stops without loss; a rerun inside an open window resumes it."""

    def __init__(self, reason: PauseReason, detail: str = "") -> None:
        super().__init__(f"{reason.value}{': ' + detail if detail else ''}")
        self.reason = reason


class InvalidRunWindow(ValueError):
    pass


@dataclass(frozen=True)
class RunWindow:
    start: time
    end: time

    @classmethod
    def parse(cls, text: str) -> RunWindow | None:
        """`HH:MM-HH:MM` local time every day; `always` disables the window and yields None."""
        if text.strip() == ALWAYS_OPEN:
            return None
        match = _WINDOW_PATTERN.match(text.strip())
        if match is None:
            raise InvalidRunWindow(f"run window {text!r} is not HH:MM-HH:MM or {ALWAYS_OPEN!r}")
        a, b, c, d = (int(group) for group in match.groups())
        try:
            window = cls(time(a, b), time(c, d))
        except ValueError as error:
            raise InvalidRunWindow(str(error)) from error
        if window.start == window.end:
            raise InvalidRunWindow("run window start and end are equal")
        return window

    def is_open(self, now: datetime) -> bool:
        moment = now.time()
        if self.start < self.end:
            return self.start <= moment < self.end
        return moment >= self.start or moment < self.end

    def night_of(self, now: datetime) -> date:
        """The local date on which the window containing `now` opened; any other moment keeps its own date."""
        if self.start > self.end and now.time() < self.end:
            return now.date() - timedelta(days=1)
        return now.date()


def looks_like_quota_exhausted(*streams: str) -> bool:
    return any(_QUOTA_REGEX.search(stream) for stream in streams)


@dataclass(frozen=True)
class CallLedger:
    """New model calls per night and harness."""

    counts: Mapping[str, Mapping[str, int]]

    def calls_on(self, night: date, harness: Harness) -> int:
        return self.counts.get(night.isoformat(), {}).get(harness.value, 0)

    def with_call(self, night: date, harness: Harness) -> CallLedger:
        updated = {key: dict(value) for key, value in self.counts.items()}
        nightly = updated.setdefault(night.isoformat(), {})
        nightly[harness.value] = nightly.get(harness.value, 0) + 1
        return CallLedger(updated)

    def to_record(self) -> dict[str, JsonValue]:
        return {night: dict(sorted(per.items())) for night, per in sorted(self.counts.items())}


EMPTY_LEDGER: Final = CallLedger({})


@dataclass(frozen=True)
class PacingPolicy:
    window: RunWindow | None
    budget_per_night: int = DEFAULT_CALL_BUDGET

    def night_of(self, now: datetime) -> date:
        return now.date() if self.window is None else self.window.night_of(now)

    def admit(self, now: datetime, harness: Harness, ledger: CallLedger, resumed: bool) -> None:
        """Raise `RunPaused` unless a new call may be made now; a resumed call is always admitted."""
        if resumed:
            return
        if self.window is not None and not self.window.is_open(now):
            raise RunPaused(PauseReason.WINDOW_CLOSED)
        if ledger.calls_on(self.night_of(now), harness) >= self.budget_per_night:
            raise RunPaused(PauseReason.BUDGET_REACHED, f"{harness.value} made {self.budget_per_night} calls tonight")
