"""The wall clock, as a port implementation: the only place that reads the time."""

from __future__ import annotations

from datetime import datetime


class SystemClock:
    def now(self) -> datetime:
        return datetime.now().astimezone().replace(tzinfo=None)
