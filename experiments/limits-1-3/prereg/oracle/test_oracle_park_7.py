from datetime import datetime, timedelta
from decimal import Decimal

from hypothesis import example, given, settings
from hypothesis import strategies as st

from parkfee import quote

FIRST = datetime(2020, 1, 1, 0, 0)
LAST = datetime(2030, 12, 31, 23, 59)
SPAN_MINUTES = int((LAST - FIRST).total_seconds() // 60)
GRACE_PERIOD_MINUTES = 15
VALIDATION_LIMIT_MINUTES = 3 * 60
NIGHT_START_HOUR = 22
NIGHT_LENGTH_MINUTES = 8 * 60
FREE = Decimal("0.00")


@st.composite
def stays(draw, min_minutes, max_minutes):
    duration = draw(st.integers(min_value=min_minutes, max_value=max_minutes))
    offset = draw(st.integers(min_value=0, max_value=SPAN_MINUTES - duration))
    entry = FIRST + timedelta(minutes=offset)
    return entry, entry + timedelta(minutes=duration)


@st.composite
def night_stays(draw, min_minutes, max_minutes=NIGHT_LENGTH_MINUTES):
    night = draw(st.dates(min_value=FIRST.date(), max_value=LAST.date() - timedelta(days=1)))
    night_start = datetime(night.year, night.month, night.day, NIGHT_START_HOUR, 0)
    entry_offset = draw(st.integers(min_value=0, max_value=NIGHT_LENGTH_MINUTES - min_minutes))
    exit_offset = draw(
        st.integers(
            min_value=entry_offset + min_minutes,
            max_value=min(NIGHT_LENGTH_MINUTES, entry_offset + max_minutes),
        )
    )
    return night_start + timedelta(minutes=entry_offset), night_start + timedelta(minutes=exit_offset)


def at(day, hour, minute):
    return datetime(2024, 5, day, hour, minute)


@settings(derandomize=True, max_examples=1000, deadline=None, database=None)
@given(
    stay=st.one_of(
        stays(GRACE_PERIOD_MINUTES + 1, VALIDATION_LIMIT_MINUTES),
        night_stays(GRACE_PERIOD_MINUTES + 1, VALIDATION_LIMIT_MINUTES),
    )
)
@example(stay=(at(10, 12, 0), at(10, 12, 16)))
@example(stay=(at(10, 12, 0), at(10, 13, 0)))
@example(stay=(at(10, 12, 0), at(10, 13, 1)))
@example(stay=(at(10, 12, 0), at(10, 14, 0)))
@example(stay=(at(10, 12, 0), at(10, 14, 1)))
@example(stay=(at(10, 12, 0), at(10, 14, 59)))
@example(stay=(at(10, 12, 0), at(10, 15, 0)))
@example(stay=(at(10, 10, 59), at(10, 13, 59)))
@example(stay=(at(10, 22, 0), at(11, 0, 1)))
@example(stay=(at(10, 22, 30), at(11, 1, 30)))
@example(stay=(at(10, 23, 0), at(11, 2, 0)))
@example(stay=(at(11, 3, 0), at(11, 6, 0)))
@example(stay=(at(11, 4, 0), at(11, 7, 0)))
@example(stay=(at(10, 21, 0), at(11, 0, 0)))
@example(stay=(datetime(2023, 12, 31, 23, 0), datetime(2024, 1, 1, 2, 0)))
def test_validated_stay_up_to_three_hours_is_free(stay):
    entry, exit_time = stay
    assert quote(entry, exit_time, lost_ticket=False, validated=True) == FREE


@settings(derandomize=True, max_examples=1000, deadline=None, database=None)
@given(
    stay=st.one_of(
        stays(VALIDATION_LIMIT_MINUTES + 1, 3 * 24 * 60),
        stays(VALIDATION_LIMIT_MINUTES + 1, SPAN_MINUTES),
        night_stays(VALIDATION_LIMIT_MINUTES + 1),
    )
)
@example(stay=(at(10, 12, 0), at(10, 15, 1)))
@example(stay=(at(10, 12, 0), at(10, 16, 0)))
@example(stay=(at(10, 12, 0), at(10, 16, 1)))
@example(stay=(at(10, 12, 0), at(10, 19, 0)))
@example(stay=(at(10, 12, 0), at(10, 19, 1)))
@example(stay=(at(10, 12, 0), at(11, 12, 0)))
@example(stay=(at(10, 12, 0), at(11, 12, 1)))
@example(stay=(at(10, 12, 0), at(12, 12, 0)))
@example(stay=(at(10, 12, 0), at(12, 12, 1)))
@example(stay=(at(10, 22, 0), at(11, 1, 1)))
@example(stay=(at(10, 22, 1), at(11, 5, 59)))
@example(stay=(at(10, 22, 0), at(11, 6, 0)))
@example(stay=(at(10, 23, 0), at(11, 5, 0)))
@example(stay=(at(10, 5, 0), at(10, 23, 0)))
@example(stay=(at(10, 21, 0), at(11, 1, 0)))
@example(stay=(FIRST, LAST))
def test_validated_stay_over_three_hours_is_charged_as_not_validated(stay):
    entry, exit_time = stay
    validated_charge = quote(entry, exit_time, lost_ticket=False, validated=True)
    standard_charge = quote(entry, exit_time, lost_ticket=False, validated=False)
    assert validated_charge == standard_charge
