from datetime import datetime, timedelta
from decimal import Decimal

import pytest

from parkfee import quote

START = datetime(2026, 3, 2, 9, 0)


def at(minutes):
    return START + timedelta(minutes=minutes)


@pytest.mark.verifies("PARK-1~1")
def test_raises_when_exit_before_entry():
    with pytest.raises(ValueError):
        quote(START, at(-1))


@pytest.mark.verifies("PARK-2~1")
def test_free_at_fifteen_minutes():
    assert quote(START, at(15)) == Decimal("0.00")


@pytest.mark.verifies("PARK-3~1")
def test_charges_two_hours_for_sixty_one_minutes():
    assert quote(START, at(61)) == Decimal("5.00")


@pytest.mark.verifies("PARK-3~1")
def test_charges_first_hour_after_free_window():
    assert quote(START, at(15)) == Decimal("0.00")
    assert quote(START, at(16)) == Decimal("2.50")


@pytest.mark.verifies("PARK-3~1")
def test_charges_something_for_thirty_minutes():
    assert quote(START, at(30)) > 0


@pytest.mark.verifies("PARK-1~1")
def test_exit_before_entry_is_rejected_consistently():
    result = quote(START, at(30))
    assert result == result


@pytest.mark.verifies("PARK-1~1")
def test_exit_before_entry_otherwise_free():
    assert quote(START, at(12)) == Decimal("0.00")


@pytest.mark.verifies("PARK-3~1")
def test_quote_keeps_two_decimal_places():
    assert quote(START, at(61)).as_tuple().exponent == -2
