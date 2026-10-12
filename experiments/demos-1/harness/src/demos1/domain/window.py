"""The window W of merged pull requests, its days, and the search intervals over it (PREREG 5, 6.1)."""

from __future__ import annotations

from collections.abc import Iterator
from dataclasses import dataclass
from datetime import UTC, date, datetime, timedelta
from typing import Final

WINDOW_FIRST_DAY: Final = date(2026, 7, 1)
WINDOW_LAST_DAY: Final = date(2026, 9, 30)
WINDOW_START: Final = datetime(2026, 7, 1, 0, 0, 0, tzinfo=UTC)
WINDOW_END: Final = datetime(2026, 9, 30, 23, 59, 59, tzinfo=UTC)
LAST_SECOND_OF_DAY: Final = timedelta(hours=23, minutes=59, seconds=59)
ISO_SECONDS: Final = "%Y-%m-%dT%H:%M:%SZ"
DEFAULT_BRANCH_UNTIL: Final = "2026-09-30T23:59:59Z"
"""The `until` of the default-branch commit of a stratum A pick."""


def parse_utc(value: str) -> datetime:
    """A GitHub timestamp (`2026-09-30T12:00:00Z`) as an aware UTC datetime."""
    return datetime.fromisoformat(value.replace("Z", "+00:00")).astimezone(UTC)


def format_utc(moment: datetime) -> str:
    return moment.astimezone(UTC).strftime(ISO_SECONDS)


def in_window(merged_at: datetime) -> bool:
    return WINDOW_START <= merged_at <= WINDOW_END


def days_latest_first() -> Iterator[date]:
    """The days of W, processed from 2026-09-30 back to 2026-07-01."""
    day = WINDOW_LAST_DAY
    while day >= WINDOW_FIRST_DAY:
        yield day
        day -= timedelta(days=1)


@dataclass(frozen=True, order=True)
class Interval:
    """A closed interval of whole seconds, the `merged:` range of one pull request search."""

    start: datetime
    end: datetime

    @classmethod
    def of_day(cls, day: date) -> Interval:
        start = datetime(day.year, day.month, day.day, tzinfo=UTC)
        return cls(start, start + LAST_SECOND_OF_DAY)

    @classmethod
    def of_window(cls) -> Interval:
        return cls(WINDOW_START, WINDOW_END)

    def seconds(self) -> int:
        return int((self.end - self.start).total_seconds()) + 1

    def is_one_second(self) -> bool:
        return self.start == self.end

    def halves(self) -> tuple[Interval, Interval]:
        """The two halves split at the middle second: the first ends on it, the second starts after it."""
        if self.is_one_second():
            raise ValueError("a one-second interval cannot be split")
        middle = self.start + timedelta(seconds=(self.seconds() - 1) // 2)
        return Interval(self.start, middle), Interval(middle + timedelta(seconds=1), self.end)

    def merged_qualifier(self) -> str:
        return f"merged:{format_utc(self.start)}..{format_utc(self.end)}"
