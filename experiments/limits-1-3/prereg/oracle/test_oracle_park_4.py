from datetime import datetime, timedelta
from decimal import Decimal

from hypothesis import assume, example, given, settings
from hypothesis import strategies as st

from parkfee import quote

FIRST = datetime(2020, 1, 1, 0, 0)
LAST = datetime(2030, 12, 31, 23, 59)
SPAN_MINUTES = int((LAST - FIRST).total_seconds() // 60)
HOUR_MINUTES = 60
DAY_MINUTES = 24 * HOUR_MINUTES
LAST_UNCAPPED_HOUR = 7
DAILY_CAP = Decimal("18.00")
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


def at(day, hour, minute):
    return datetime(2024, 5, day, hour, minute)


@settings(derandomize=True, max_examples=1000, deadline=None, database=None)
@given(stay=stays(LAST_UNCAPPED_HOUR * HOUR_MINUTES + 1, DAY_MINUTES))
@example(stay=(at(10, 8, 0), at(10, 15, 1)))
@example(stay=(at(10, 8, 0), at(10, 16, 0)))
@example(stay=(at(10, 8, 0), at(10, 16, 1)))
@example(stay=(at(10, 8, 0), at(10, 17, 0)))
@example(stay=(at(10, 8, 0), at(10, 17, 1)))
@example(stay=(at(10, 8, 0), at(10, 20, 0)))
@example(stay=(at(10, 8, 0), at(11, 7, 59)))
@example(stay=(at(10, 8, 0), at(11, 8, 0)))
@example(stay=(at(10, 0, 0), at(11, 0, 0)))
@example(stay=(at(10, 21, 0), at(11, 5, 0)))
@example(stay=(at(10, 21, 59), at(11, 5, 59)))
@example(stay=(at(10, 22, 30), at(11, 6, 30)))
@example(stay=(at(10, 23, 0), at(11, 7, 0)))
@example(stay=(at(10, 5, 0), at(10, 23, 0)))
@example(stay=(at(10, 1, 0), at(11, 1, 0)))
@example(stay=(at(10, 22, 0), at(11, 22, 0)))
@example(stay=(datetime(2023, 12, 31, 18, 0), datetime(2024, 1, 1, 9, 0)))
@example(stay=(datetime(2024, 2, 28, 12, 0), datetime(2024, 2, 29, 12, 0)))
def test_charge_is_capped_for_stays_up_to_one_day(stay):
    entry, exit_time = stay
    assume(not is_long_night_stay(entry, exit_time))
    assert quote(entry, exit_time, lost_ticket=False, validated=False) == DAILY_CAP
