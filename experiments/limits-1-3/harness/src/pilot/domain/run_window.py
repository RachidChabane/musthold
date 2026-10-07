"""The run window: model calls are made only inside a daily local-time window, which may cross midnight."""

from __future__ import annotations

import re
from dataclasses import dataclass
from datetime import datetime, time
from typing import Final

from pilot.domain.quota import QuotaExhausted

RUN_WINDOW_ENV: Final = "PILOT_RUN_WINDOW"
DEFAULT_RUN_WINDOW: Final = "21:00-07:30"
RUN_WINDOW_PAUSE_TEXT: Final = "run window closed"
ALWAYS_OPEN: Final = "always"
_PATTERN = re.compile(r"^(\d{1,2}):(\d{2})-(\d{1,2}):(\d{2})$")


class RunWindowClosed(QuotaExhausted):
    """The run window closed; a rerun inside the window resumes."""


class InvalidRunWindow(ValueError):
    pass


@dataclass(frozen=True)
class RunWindow:
    start: time
    end: time

    @classmethod
    def parse(cls, text: str) -> RunWindow | None:
        """`HH:MM-HH:MM` local time, every day; `always` disables the window (None)."""
        if text.strip() == ALWAYS_OPEN:
            return None
        match = _PATTERN.match(text.strip())
        if match is None:
            raise InvalidRunWindow(f"run window {text!r} is not HH:MM-HH:MM or {ALWAYS_OPEN!r}")
        a, b, c, d = (int(g) for g in match.groups())
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
