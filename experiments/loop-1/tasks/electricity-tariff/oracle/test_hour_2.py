import pytest

from electricity.tariffs import electricity_tariff


def test_example_hour_ten_has_daytime_tariff():
    assert electricity_tariff(hour=10) == 0.23


@pytest.mark.parametrize("hour", range(6, 16))
def test_hours_six_through_fifteen_have_daytime_tariff(hour):
    assert electricity_tariff(hour=hour) == 0.23


def test_daytime_period_boundaries_and_adjacent_hours():
    assert electricity_tariff(hour=5) != 0.23
    assert electricity_tariff(hour=6) == 0.23
    assert electricity_tariff(hour=15) == 0.23
    assert electricity_tariff(hour=16) != 0.23
