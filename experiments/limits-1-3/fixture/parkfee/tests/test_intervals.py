from datetime import datetime, timedelta
from decimal import Decimal

import pytest

from parkfee import quote


def at(hour, minute=0, day=9):
    return datetime(2026, 3, day, hour, minute)


@pytest.mark.verifies("PARK-1~1")
def test_exit_before_entry_raises_even_when_ticket_is_lost():
    with pytest.raises(ValueError) as excinfo:
        quote(at(18, 20), at(18, 19), lost_ticket=True)
    assert excinfo.type is ValueError


@pytest.mark.verifies("PARK-2~1")
def test_short_stay_within_grace_period_is_free():
    assert quote(at(12, 0), at(12, 5)) < Decimal("2.50")


@pytest.mark.verifies("PARK-1~1")
def test_exit_before_entry_reports_reversed_times():
    with pytest.raises(ValueError) as excinfo:
        quote(at(10, 0, day=10), at(9, 0))
    assert str(excinfo.value) == "exit time precedes entry time"


@pytest.mark.verifies("PARK-2~1")
def test_stay_of_fifteen_minutes_is_free():
    assert quote(at(10, 0), at(10, 5)) == Decimal("0.00")
    assert quote(at(10, 0), at(10, 15)) == quote(at(10, 0), at(10, 5))


@pytest.mark.verifies("PARK-1~1")
def test_exit_earlier_than_entry_is_rejected():
    try:
        quote(at(16, 0), at(15, 45))
    except ValueError as error:
        assert isinstance(error, ValueError)


@pytest.mark.verifies("PARK-2~1")
def test_quarter_hour_over_midnight_is_free():
    entry = at(23, 50)
    assert quote(entry, entry + timedelta(minutes=15)) == quote(entry, entry)


@pytest.mark.verifies("PARK-1~1")
def test_exit_before_entry_raises_value_error():
    with pytest.raises(ValueError) as excinfo:
        quote(at(10, 30), at(10, 29))
    assert excinfo.type is ValueError


@pytest.mark.verifies("PARK-2~1")
def test_grace_period_ends_after_fifteen_minutes():
    assert quote(at(12, 0), at(12, 16)) == Decimal("2.50")
