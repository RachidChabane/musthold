from datetime import datetime, timedelta

import pytest

from parkfee import quote

START = datetime(2026, 3, 2, 9, 0)


@pytest.mark.parametrize("minutes", [1, 2, 60, 1440])
def test_oracle_raises_for_earlier_exit(minutes):
    with pytest.raises(ValueError):
        quote(START, START - timedelta(minutes=minutes))


def test_oracle_equal_times_do_not_raise():
    assert quote(START, START) == 0
