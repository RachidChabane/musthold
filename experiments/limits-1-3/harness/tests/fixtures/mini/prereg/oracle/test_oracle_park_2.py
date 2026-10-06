from datetime import datetime, timedelta

from parkfee import quote

START = datetime(2026, 3, 2, 9, 0)


def test_oracle_free_up_to_fifteen_minutes():
    for minutes in range(0, 16):
        assert quote(START, START + timedelta(minutes=minutes)) == 0
