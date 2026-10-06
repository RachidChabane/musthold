from datetime import datetime, timedelta
from decimal import Decimal

from hypothesis import example, given, settings
from hypothesis import strategies as st

from parkfee import quote

FIRST = datetime(2020, 1, 1, 0, 0)
LAST = datetime(2030, 12, 31, 23, 59)
SPAN_MINUTES = int((LAST - FIRST).total_seconds() // 60)
LOST_TICKET_CHARGE = Decimal("40.00")


@st.composite
def stays(draw, min_minutes, max_minutes):
    duration = draw(st.integers(min_value=min_minutes, max_value=max_minutes))
    offset = draw(st.integers(min_value=0, max_value=SPAN_MINUTES - duration))
    entry = FIRST + timedelta(minutes=offset)
    return entry, entry + timedelta(minutes=duration)


def at(day, hour, minute):
    return datetime(2024, 5, day, hour, minute)


@settings(derandomize=True, max_examples=1000, deadline=None, database=None)
@given(
    stay=st.one_of(stays(0, 3 * 24 * 60), stays(0, SPAN_MINUTES)),
    validated=st.booleans(),
)
@example(stay=(at(10, 12, 0), at(10, 12, 0)), validated=False)
@example(stay=(at(10, 12, 0), at(10, 12, 0)), validated=True)
@example(stay=(at(10, 12, 0), at(10, 12, 1)), validated=False)
@example(stay=(at(10, 12, 0), at(10, 12, 15)), validated=False)
@example(stay=(at(10, 12, 0), at(10, 12, 15)), validated=True)
@example(stay=(at(10, 12, 0), at(10, 12, 16)), validated=False)
@example(stay=(at(10, 12, 0), at(10, 12, 16)), validated=True)
@example(stay=(at(10, 12, 0), at(10, 13, 0)), validated=False)
@example(stay=(at(10, 12, 0), at(10, 15, 0)), validated=True)
@example(stay=(at(10, 12, 0), at(10, 15, 1)), validated=True)
@example(stay=(at(10, 12, 0), at(10, 19, 0)), validated=False)
@example(stay=(at(10, 12, 0), at(10, 19, 1)), validated=False)
@example(stay=(at(10, 12, 0), at(11, 12, 0)), validated=False)
@example(stay=(at(10, 12, 0), at(11, 12, 1)), validated=False)
@example(stay=(at(10, 12, 0), at(11, 12, 1)), validated=True)
@example(stay=(at(10, 12, 0), at(14, 12, 1)), validated=False)
@example(stay=(at(10, 22, 0), at(11, 6, 0)), validated=False)
@example(stay=(at(10, 22, 30), at(11, 1, 0)), validated=False)
@example(stay=(at(10, 22, 30), at(11, 1, 0)), validated=True)
@example(stay=(at(10, 23, 0), at(11, 5, 0)), validated=False)
@example(stay=(FIRST, LAST), validated=False)
@example(stay=(FIRST, LAST), validated=True)
def test_lost_ticket_is_charged_flat_fee(stay, validated):
    entry, exit_time = stay
    assert quote(entry, exit_time, lost_ticket=True, validated=validated) == LOST_TICKET_CHARGE
