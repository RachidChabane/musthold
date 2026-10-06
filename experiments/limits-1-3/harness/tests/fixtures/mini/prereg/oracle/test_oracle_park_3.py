from datetime import datetime, timedelta
from decimal import Decimal

from hypothesis import given, settings
from hypothesis import strategies as st

from parkfee import quote

START = datetime(2026, 3, 2, 9, 0)


@settings(derandomize=True, max_examples=50, database=None)
@given(st.integers(min_value=16, max_value=5000))
def test_oracle_started_hours(minutes):
    hours = -(-minutes // 60)
    assert quote(START, START + timedelta(minutes=minutes)) == Decimal("2.50") * hours
