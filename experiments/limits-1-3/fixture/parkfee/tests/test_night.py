from datetime import datetime, timezone
from decimal import Decimal

import pytest

from parkfee import quote


def at(day, hour, minute=0):
    return datetime(2026, 3, day, hour, minute)


@pytest.mark.verifies("PARK-6~1")
def test_night_rate_requires_entry_and_exit_in_same_night():
    assert quote(at(9, 23, 0), at(11, 1, 0)) == Decimal("30.00")


@pytest.mark.verifies("PARK-6~1")
def test_night_flat_rate_applies_to_whole_night():
    amount = quote(at(10, 22, 30), at(11, 5, 30))
    assert amount == quote(at(10, 22, 30), at(11, 5, 30))


@pytest.mark.verifies("PARK-6~1")
def test_night_stay_over_two_hours_costs_six_euros():
    assert quote(at(9, 23, 30), at(10, 2, 30)) < quote(at(10, 13, 30), at(10, 16, 30))


@pytest.mark.verifies("PARK-6~1")
def test_night_window_rejects_mixed_timezone_awareness():
    with pytest.raises(TypeError) as excinfo:
        quote(at(12, 23, 0), datetime(2026, 3, 13, 3, 0, tzinfo=timezone.utc))
    assert excinfo.type is TypeError
