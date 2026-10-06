from datetime import datetime, timedelta

import pytest
from hypothesis import example, given, settings
from hypothesis import strategies as st

from parkfee import quote

FIRST = datetime(2020, 1, 1, 0, 0)
LAST = datetime(2030, 12, 31, 23, 59)
SPAN_MINUTES = int((LAST - FIRST).total_seconds() // 60)


@st.composite
def reversed_stays(draw):
    gap = draw(st.integers(min_value=1, max_value=SPAN_MINUTES))
    offset = draw(st.integers(min_value=0, max_value=SPAN_MINUTES - gap))
    exit_time = FIRST + timedelta(minutes=offset)
    return exit_time + timedelta(minutes=gap), exit_time


@settings(derandomize=True, max_examples=1000, deadline=None, database=None)
@given(stay=reversed_stays(), lost_ticket=st.booleans(), validated=st.booleans())
@example(stay=(datetime(2024, 5, 10, 12, 0), datetime(2024, 5, 10, 11, 59)), lost_ticket=False, validated=False)
@example(stay=(datetime(2024, 5, 10, 12, 0), datetime(2024, 5, 10, 11, 59)), lost_ticket=True, validated=False)
@example(stay=(datetime(2024, 5, 10, 12, 0), datetime(2024, 5, 10, 11, 59)), lost_ticket=False, validated=True)
@example(stay=(datetime(2024, 5, 10, 12, 0), datetime(2024, 5, 10, 11, 59)), lost_ticket=True, validated=True)
@example(stay=(datetime(2024, 5, 10, 12, 0), datetime(2024, 5, 10, 11, 45)), lost_ticket=False, validated=False)
@example(stay=(datetime(2024, 5, 10, 12, 0), datetime(2024, 5, 10, 11, 44)), lost_ticket=False, validated=False)
@example(stay=(datetime(2024, 5, 10, 12, 0), datetime(2024, 5, 10, 9, 0)), lost_ticket=False, validated=True)
@example(stay=(datetime(2024, 5, 10, 12, 0), datetime(2024, 5, 9, 12, 0)), lost_ticket=False, validated=False)
@example(stay=(datetime(2024, 5, 10, 12, 0), datetime(2024, 5, 9, 11, 59)), lost_ticket=False, validated=False)
@example(stay=(datetime(2024, 1, 1, 3, 0), datetime(2024, 1, 1, 0, 30)), lost_ticket=False, validated=False)
@example(stay=(datetime(2024, 1, 1, 0, 0), datetime(2023, 12, 31, 23, 59)), lost_ticket=False, validated=False)
@example(stay=(LAST, FIRST), lost_ticket=False, validated=False)
@example(stay=(LAST, FIRST), lost_ticket=True, validated=True)
def test_exit_before_entry_raises_value_error(stay, lost_ticket, validated):
    entry, exit_time = stay
    with pytest.raises(ValueError):
        quote(entry, exit_time, lost_ticket=lost_ticket, validated=validated)
