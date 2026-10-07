"""Exploratory streams: their windows and the per-call status that stands in for a deviation entry (ADR 0006).

A call that cannot be made inside its window is reported BLOCKED in the exploratory tables only;
it never reaches `DEVIATIONS.md`, so it never touches a decisional endpoint.
"""

from __future__ import annotations

from dataclasses import dataclass
from datetime import UTC, datetime
from enum import Enum
from typing import Final

STREAM_LUNA: Final = "luna"
STREAM_SECONDARY: Final = "secondary"
STREAMS: Final = (STREAM_LUNA, STREAM_SECONDARY)


class CallStatus(Enum):
    COMPLETE = "complete"
    BLOCKED = "blocked"
    PENDING = "pending"
    MISSING = "missing"


@dataclass(frozen=True)
class Window:
    """`opens` is None when the stream may start at once; the stream is closed from `closes` on."""

    opens: datetime | None
    closes: datetime

    def is_open(self, now: datetime) -> bool:
        return (self.opens is None or now >= self.opens) and now < self.closes

    def not_yet_open(self, now: datetime) -> bool:
        return self.opens is not None and now < self.opens

    def is_closed(self, now: datetime) -> bool:
        return now >= self.closes


SECONDARY_WINDOW: Final = Window(None, datetime(2026, 10, 24, 23, 59, tzinfo=UTC))
LUNA_WINDOW: Final = Window(datetime(2026, 10, 10, 9, 1, tzinfo=UTC), datetime(2026, 10, 17, 23, 59, tzinfo=UTC))
WINDOWS: Final = {STREAM_LUNA: LUNA_WINDOW, STREAM_SECONDARY: SECONDARY_WINDOW}
"""The luna window opens at 11:01 Europe/Paris (09:01 UTC, summer time): the Codex free quota resets then."""
