from datetime import datetime, timedelta
from decimal import Decimal

from hypothesis import assume, example, given, settings
from hypothesis import strategies as st

from parkfee import quote

FIRST = datetime(2020, 1, 1, 0, 0)
LAST = datetime(2030, 12, 31, 23, 59)
SPAN_MINUTES = int((LAST - FIRST).total_seconds() // 60)
GRACE_PERIOD_MINUTES = 15
HOUR_MINUTES = 60
HOURLY_RATE = Decimal("2.50")
LAST_UNCAPPED_HOUR = 7
NIGHT_START_MINUTE_OF_DAY = 22 * 60
NIGHT_END_MINUTE_OF_DAY = 6 * 60
MINIMUM_NIGHT_STAY_MINUTES = 2 * 60 + 1


@st.composite
def stays(draw, min_minutes, max_minutes):
    duration = draw(st.integers(min_value=min_minutes, max_value=max_minutes))
    offset = draw(st.integers(min_value=0, max_value=SPAN_MINUTES - duration))
    entry = FIRST + timedelta(minutes=offset)
    return entry, entry + timedelta(minutes=duration)


def duration_minutes(entry, exit_time):
    return int((exit_time - entry).total_seconds() // 60)


def night_of(moment):
    minute_of_day = moment.hour * 60 + moment.minute
    if minute_of_day >= NIGHT_START_MINUTE_OF_DAY:
        return moment.date()
    if minute_of_day <= NIGHT_END_MINUTE_OF_DAY:
        return moment.date() - timedelta(days=1)
    return None


def is_long_night_stay(entry, exit_time):
    night = night_of(entry)
    return (
        night is not None
        and night == night_of(exit_time)
        and duration_minutes(entry, exit_time) >= MINIMUM_NIGHT_STAY_MINUTES
    )


def started_hours(entry, exit_time):
    return -(-duration_minutes(entry, exit_time) // HOUR_MINUTES)


def at(day, hour, minute):
    return datetime(2024, 5, day, hour, minute)


@settings(derandomize=True, max_examples=1000, deadline=None, database=None)
@given(stay=stays(GRACE_PERIOD_MINUTES + 1, LAST_UNCAPPED_HOUR * HOUR_MINUTES))
@example(stay=(at(10, 12, 0), at(10, 12, 16)))
@example(stay=(at(10, 12, 0), at(10, 12, 17)))
@example(stay=(at(10, 12, 0), at(10, 13, 0)))
@example(stay=(at(10, 12, 0), at(10, 13, 1)))
@example(stay=(at(10, 12, 0), at(10, 14, 0)))
@example(stay=(at(10, 12, 0), at(10, 14, 1)))
@example(stay=(at(10, 12, 0), at(10, 15, 0)))
@example(stay=(at(10, 12, 0), at(10, 15, 1)))
@example(stay=(at(10, 12, 0), at(10, 18, 59)))
@example(stay=(at(10, 12, 0), at(10, 19, 0)))
@example(stay=(at(10, 10, 59), at(10, 11, 30)))
@example(stay=(at(10, 10, 30), at(10, 11, 31)))
@example(stay=(at(10, 10, 45), at(10, 12, 15)))
@example(stay=(at(10, 23, 50), at(11, 0, 10)))
@example(stay=(at(10, 22, 0), at(11, 0, 0)))
@example(stay=(at(10, 23, 0), at(11, 1, 0)))
@example(stay=(at(11, 3, 59), at(11, 5, 59)))
@example(stay=(at(10, 22, 30), at(10, 23, 30)))
@example(stay=(at(10, 21, 30), at(10, 23, 45)))
@example(stay=(at(10, 21, 59), at(11, 0, 0)))
@example(stay=(at(10, 20, 0), at(11, 3, 0)))
@example(stay=(at(11, 3, 0), at(11, 6, 30)))
@example(stay=(at(11, 4, 0), at(11, 6, 1)))
@example(stay=(at(11, 5, 0), at(11, 9, 0)))
@example(stay=(datetime(2023, 12, 31, 23, 0), datetime(2024, 1, 1, 0, 30)))
@example(stay=(datetime(2024, 2, 29, 10, 0), datetime(2024, 2, 29, 15, 1)))
def test_stay_is_charged_per_started_hour(stay):
    entry, exit_time = stay
    assume(not is_long_night_stay(entry, exit_time))
    expected = HOURLY_RATE * started_hours(entry, exit_time)
    assert quote(entry, exit_time, lost_ticket=False, validated=False) == expected
