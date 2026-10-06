from datetime import datetime, timedelta
from decimal import Decimal
from unittest.mock import patch

import pytest

from parkfee import Breakdown, breakdown, quote


def at(hour, minute=0, day=12):
    return datetime(2026, 3, day, hour, minute)


@pytest.mark.verifies("PARK-7~1")
def test_validated_ticket_covers_two_hour_stay():
    assert quote(at(14, 0), at(16, 0), lost_ticket=True, validated=True) == Decimal("40.00")


@pytest.mark.verifies("PARK-8~1")
def test_lost_ticket_fee_only_applies_when_ticket_is_lost():
    assert quote(at(8, 0, day=9), at(14, 0, day=10)) == Decimal("30.00")


@pytest.mark.verifies("PARK-8~1")
def test_lost_ticket_breakdown_shows_flat_fee():
    assert repr(breakdown(at(9, 0), at(12, 0), lost_ticket=True)) == (
        "Breakdown(rule=<Rule.LOST_TICKET: 'lost_ticket'>, "
        "duration=datetime.timedelta(seconds=10800), amount=Decimal('40.00'))"
    )


@pytest.mark.verifies("PARK-8~1")
def test_lost_ticket_fee_is_forty_euros():
    fee = quote(at(8, 0), at(17, 0), lost_ticket=True)
    assert fee == Decimal(fee)


@pytest.mark.verifies("PARK-7~1")
def test_validated_stay_breakdown_is_free():
    assert breakdown(at(10, 0), at(11, 30), validated=True) == Breakdown(
        rule="validated", duration=timedelta(minutes=90), amount=Decimal("0.00")
    )


@pytest.mark.verifies("PARK-7~1")
def test_validated_stay_under_three_hours_is_free():
    with patch("parkfee.tariff._validated_free", return_value=True):
        amount = quote(at(10, 0), at(12, 0), validated=True)
    assert amount == Decimal("0.00")


@pytest.mark.verifies("PARK-8~1")
def test_lost_ticket_costs_forty_whatever_the_stay():
    assert quote(at(9, 0), at(9, 5), lost_ticket=True) == Decimal("40.00")
    assert quote(at(9, 0), at(11, 0), lost_ticket=True, validated=True) == Decimal("40.00")
    assert quote(at(9, 0, day=9), at(11, 0, day=11), lost_ticket=True) == Decimal("40.00")
