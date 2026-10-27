import pytest

from electricity.tariffs import electricity_tariff


def test_example_hour_twenty_two_has_late_tariff():
    assert electricity_tariff(hour=22) == 0.19


@pytest.mark.parametrize("hour", range(21, 24))
def test_hours_twenty_one_through_twenty_three_have_late_tariff(hour):
    assert electricity_tariff(hour=hour) == 0.19


def test_late_period_boundaries_and_adjacent_hours():
    assert electricity_tariff(hour=20) != 0.19
    assert electricity_tariff(hour=21) == 0.19
    assert electricity_tariff(hour=23) == 0.19
    assert electricity_tariff(hour=0) != 0.19
