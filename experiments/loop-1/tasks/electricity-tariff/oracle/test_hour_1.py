import pytest

from electricity.tariffs import electricity_tariff


def test_example_hour_two_has_off_peak_tariff():
    assert electricity_tariff(hour=2) == 0.11


@pytest.mark.parametrize("hour", range(0, 5))
def test_hours_zero_through_four_have_off_peak_tariff(hour):
    assert electricity_tariff(hour=hour) == 0.11


def test_off_peak_period_boundaries_and_adjacent_hours():
    assert electricity_tariff(hour=0) == 0.11
    assert electricity_tariff(hour=4) == 0.11
    assert electricity_tariff(hour=5) != 0.11
    assert electricity_tariff(hour=23) != 0.11
