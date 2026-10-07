"""The nightly window in which a local model may be loaded (§6.5)."""

from __future__ import annotations

import re
from dataclasses import dataclass
from datetime import datetime, time
from typing import Final

from calibration.domain.constants import ALWAYS_OPEN

_WINDOW_PATTERN: Final = re.compile(r"^(\d{1,2}):(\d{2})-(\d{1,2}):(\d{2})$")
WINDOW_CLOSED_TEXT: Final = "run window closed"


class InvalidRunWindow(ValueError):
    pass


@dataclass(frozen=True)
class RunWindow:
    """Every day from `start` to `end` local time; it may cross midnight."""

    start: time
    end: time

    @classmethod
    def parse(cls, text: str) -> RunWindow | None:
        """`HH:MM-HH:MM`, or `always`, which disables the window and yields None."""
        stripped = text.strip()
        if stripped == ALWAYS_OPEN:
            return None
        match = _WINDOW_PATTERN.match(stripped)
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


def is_window_open(window: RunWindow | None, now: datetime) -> bool:
    return window is None or window.is_open(now)
