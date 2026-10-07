"""The wall clock and the real sleep behind the `Clock` and `Sleeper` ports."""

from __future__ import annotations

import time
from datetime import datetime


class SystemClock:
    def now(self) -> datetime:
        return datetime.now()

    def monotonic(self) -> float:
        return time.monotonic()


class SystemSleeper:
    def sleep(self, seconds: float) -> None:
        time.sleep(seconds)
