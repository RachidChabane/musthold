from datetime import datetime, timedelta
from decimal import Decimal

from hypothesis import example, given, settings
from hypothesis import strategies as st

from parkfee import quote

FIRST = datetime(2020, 1, 1, 0, 0)
LAST = datetime(2030, 12, 31, 23, 59)
SPAN_MINUTES = int((LAST - FIRST).total_seconds() // 60)
GRACE_PERIOD_MINUTES = 15
FREE = Decimal("0.00")


@st.composite
def stays(draw, min_minutes, max_minutes):
    duration = draw(st.integers(min_value=min_minutes, max_value=max_minutes))
    offset = draw(st.integers(min_value=0, max_value=SPAN_MINUTES - duration))
    entry = FIRST + timedelta(minutes=offset)
    return entry, entry + timedelta(minutes=duration)


def at(day, hour, minute):
    return datetime(2024, 5, day, hour, minute)


@settings(derandomize=True, max_examples=1000, deadline=None, database=None)
@given(stay=stays(0, GRACE_PERIOD_MINUTES), validated=st.booleans())
@example(stay=(at(10, 12, 0), at(10, 12, 0)), validated=False)
@example(stay=(at(10, 12, 0), at(10, 12, 0)), validated=True)
@example(stay=(at(10, 12, 0), at(10, 12, 1)), validated=False)
@example(stay=(at(10, 12, 0), at(10, 12, 14)), validated=False)
@example(stay=(at(10, 12, 0), at(10, 12, 15)), validated=False)
@example(stay=(at(10, 12, 0), at(10, 12, 15)), validated=True)
@example(stay=(at(10, 12, 50), at(10, 13, 5)), validated=False)
@example(stay=(at(10, 23, 50), at(11, 0, 5)), validated=False)
@example(stay=(at(10, 22, 0), at(10, 22, 15)), validated=False)
@example(stay=(at(10, 5, 50), at(10, 6, 5)), validated=False)
@example(stay=(datetime(2023, 12, 31, 23, 55), datetime(2024, 1, 1, 0, 10)), validated=False)
@example(stay=(FIRST, FIRST + timedelta(minutes=15)), validated=False)
@example(stay=(LAST - timedelta(minutes=15), LAST), validated=False)
def test_short_stay_is_free(stay, validated):
    entry, exit_time = stay
    assert quote(entry, exit_time, lost_ticket=False, validated=validated) == FREE
