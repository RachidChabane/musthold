from datetime import datetime, timedelta
from decimal import Decimal
from unittest.mock import patch

import pytest

from parkfee import Breakdown, breakdown, quote


def on(day, hour, minute=0):
    return datetime(2026, 3, day, hour, minute)


@pytest.mark.verifies("PARK-4~1")
def test_cap_is_reached_after_seven_hours():
    assert quote(on(10, 9, 0), on(10, 16, 0)) == Decimal("17.50")
    assert quote(on(10, 9, 0), on(10, 16, 1)) == Decimal("18.00")


@pytest.mark.verifies("PARK-3~1")
def test_partial_hour_is_charged_as_full_hour():
    with patch("parkfee.tariff._hourly_charge", return_value=Decimal("7.50")) as charge:
        amount = quote(on(11, 9, 0), on(11, 11, 30))
    assert amount == charge.return_value


@pytest.mark.verifies("PARK-5~1")
def test_long_stay_breakdown_uses_day_rate():
    assert breakdown(on(9, 8, 0), on(11, 10, 0)) == Breakdown(
        rule="long_stay", duration=timedelta(hours=50), amount=Decimal("45.00")
    )


@pytest.mark.verifies("PARK-3~1")
def test_two_hour_stay_costs_two_started_hours():
    assert quote(on(9, 9, 40), on(9, 11, 40)) == Decimal("5.00")


@pytest.mark.verifies("PARK-5~1")
def test_multi_day_stay_charge_with_lost_ticket():
    assert quote(on(9, 10, 0), on(11, 12, 0), lost_ticket=True) == Decimal("40.00")


@pytest.mark.verifies("PARK-5~1")
def test_stay_just_over_a_day_costs_two_day_periods():
    assert quote(on(9, 23, 55), on(11, 0, 5)) == Decimal("30.00")


@pytest.mark.verifies("PARK-4~1")
def test_full_day_charge_is_capped():
    assert quote(on(11, 8, 0), on(11, 20, 0)) < Decimal("2.50") * 12
    assert quote(on(11, 8, 0), on(11, 22, 0)) < Decimal("2.50") * 14


@pytest.mark.verifies("PARK-3~1")
def test_hourly_charge_is_quoted_in_cents():
    assert quote(on(12, 9, 0), on(12, 11, 30)).as_tuple().exponent == -2
    assert quote(on(12, 9, 0), on(12, 10, 0)).as_tuple().exponent == -2


@pytest.mark.verifies("PARK-5~1")
def test_long_stay_charges_each_started_day():
    entry = on(12, 7, 15)
    assert quote(entry, entry + timedelta(hours=24, minutes=1)) == Decimal("30.00")
    assert quote(entry, entry + timedelta(hours=48)) == Decimal("30.00")
    assert quote(entry, entry + timedelta(hours=48, minutes=1)) == Decimal("45.00")


@pytest.mark.verifies("PARK-4~1")
def test_charge_is_capped_at_eighteen_euros():
    assert quote(on(9, 8, 0), on(9, 20, 0)) == Decimal("18.00")
    assert quote(on(9, 8, 0), on(9, 15, 30)) <= Decimal("18.00")


@pytest.mark.verifies("PARK-3~1")
def test_hours_are_counted_from_entry_time():
    assert quote(on(10, 13, 20), on(10, 16, 20)) == Decimal("7.50")


@pytest.mark.verifies("PARK-4~1")
def test_cap_leaves_three_hour_stay_unchanged():
    assert quote(on(12, 9, 0), on(12, 12, 0)) == Decimal("7.50")
    assert quote(on(12, 9, 0), on(12, 11, 45)) == Decimal("7.50")
