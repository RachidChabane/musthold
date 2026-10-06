from datetime import datetime, timedelta
from decimal import Decimal

from hypothesis import example, given, settings
from hypothesis import strategies as st

from parkfee import quote

FIRST = datetime(2020, 1, 1, 0, 0)
LAST = datetime(2030, 12, 31, 23, 59)
SPAN_MINUTES = int((LAST - FIRST).total_seconds() // 60)
DAY_MINUTES = 24 * 60
DAILY_RATE = Decimal("15.00")


@st.composite
def stays(draw, min_minutes, max_minutes):
    duration = draw(st.integers(min_value=min_minutes, max_value=max_minutes))
    offset = draw(st.integers(min_value=0, max_value=SPAN_MINUTES - duration))
    entry = FIRST + timedelta(minutes=offset)
    return entry, entry + timedelta(minutes=duration)


def started_days(entry, exit_time):
    minutes = int((exit_time - entry).total_seconds() // 60)
    return -(-minutes // DAY_MINUTES)


def at(day, hour, minute):
    return datetime(2024, 5, day, hour, minute)


@settings(derandomize=True, max_examples=1000, deadline=None, database=None)
@given(
    stay=st.one_of(
        stays(DAY_MINUTES + 1, 7 * DAY_MINUTES),
        stays(DAY_MINUTES + 1, SPAN_MINUTES),
    )
)
@example(stay=(at(10, 12, 0), at(11, 12, 1)))
@example(stay=(at(10, 12, 0), at(11, 13, 0)))
@example(stay=(at(10, 12, 0), at(12, 11, 59)))
@example(stay=(at(10, 12, 0), at(12, 12, 0)))
@example(stay=(at(10, 12, 0), at(12, 12, 1)))
@example(stay=(at(10, 12, 0), at(13, 12, 0)))
@example(stay=(at(10, 12, 0), at(13, 12, 1)))
@example(stay=(at(10, 23, 59), at(12, 0, 1)))
@example(stay=(at(10, 0, 0), at(12, 0, 0)))
@example(stay=(at(10, 0, 0), at(12, 0, 1)))
@example(stay=(at(10, 23, 0), at(12, 1, 0)))
@example(stay=(at(10, 22, 0), at(11, 22, 1)))
@example(stay=(at(10, 5, 0), at(11, 5, 1)))
@example(stay=(datetime(2024, 2, 28, 12, 0), datetime(2024, 3, 1, 12, 1)))
@example(stay=(datetime(2023, 12, 31, 12, 0), datetime(2024, 1, 2, 12, 0)))
@example(stay=(FIRST, LAST))
def test_multi_day_stay_is_charged_per_started_day(stay):
    entry, exit_time = stay
    expected = DAILY_RATE * started_days(entry, exit_time)
    assert quote(entry, exit_time, lost_ticket=False, validated=False) == expected
