from datetime import datetime, timedelta
from decimal import Decimal

from hypothesis import example, given, settings
from hypothesis import strategies as st

from parkfee import quote

FIRST = datetime(2020, 1, 1, 0, 0)
LAST = datetime(2030, 12, 31, 23, 59)
NIGHT_START_HOUR = 22
NIGHT_LENGTH_MINUTES = 8 * 60
MINIMUM_NIGHT_STAY_MINUTES = 2 * 60 + 1
NIGHT_CHARGE = Decimal("6.00")


@st.composite
def night_stays(draw):
    night = draw(st.dates(min_value=FIRST.date(), max_value=LAST.date() - timedelta(days=1)))
    night_start = datetime(night.year, night.month, night.day, NIGHT_START_HOUR, 0)
    last_inner_offset = NIGHT_LENGTH_MINUTES - 1
    entry_offset = draw(st.integers(min_value=1, max_value=last_inner_offset - MINIMUM_NIGHT_STAY_MINUTES))
    exit_offset = draw(
        st.integers(min_value=entry_offset + MINIMUM_NIGHT_STAY_MINUTES, max_value=last_inner_offset)
    )
    return night_start + timedelta(minutes=entry_offset), night_start + timedelta(minutes=exit_offset)


def at(day, hour, minute):
    return datetime(2024, 5, day, hour, minute)


@settings(derandomize=True, max_examples=1000, deadline=None, database=None)
@given(stay=night_stays())
@example(stay=(at(10, 22, 1), at(11, 0, 2)))
@example(stay=(at(10, 22, 1), at(11, 1, 1)))
@example(stay=(at(10, 22, 1), at(11, 1, 2)))
@example(stay=(at(10, 22, 1), at(11, 5, 59)))
@example(stay=(at(10, 23, 0), at(11, 1, 1)))
@example(stay=(at(10, 23, 0), at(11, 5, 0)))
@example(stay=(at(10, 23, 59), at(11, 2, 0)))
@example(stay=(at(11, 0, 0), at(11, 2, 1)))
@example(stay=(at(11, 0, 0), at(11, 5, 59)))
@example(stay=(at(11, 1, 0), at(11, 4, 0)))
@example(stay=(at(11, 1, 0), at(11, 4, 1)))
@example(stay=(at(11, 3, 58), at(11, 5, 59)))
@example(stay=(at(10, 22, 30), at(11, 5, 30)))
@example(stay=(datetime(2020, 12, 31, 23, 0), datetime(2021, 1, 1, 3, 0)))
@example(stay=(datetime(2024, 2, 28, 22, 30), datetime(2024, 2, 29, 4, 0)))
@example(stay=(datetime(2024, 2, 29, 1, 0), datetime(2024, 2, 29, 4, 0)))
@example(stay=(datetime(2024, 3, 31, 23, 0), datetime(2024, 4, 1, 4, 0)))
def test_night_stay_is_charged_flat_rate(stay):
    entry, exit_time = stay
    assert quote(entry, exit_time, lost_ticket=False, validated=False) == NIGHT_CHARGE
