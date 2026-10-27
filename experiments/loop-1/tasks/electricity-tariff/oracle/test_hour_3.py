import pytest

from electricity.tariffs import electricity_tariff


def test_example_hour_eighteen_has_peak_tariff():
    assert electricity_tariff(hour=18) == 0.37


@pytest.mark.parametrize("hour", range(17, 20))
def test_hours_seventeen_through_nineteen_have_peak_tariff(hour):
    assert electricity_tariff(hour=hour) == 0.37


def test_peak_period_boundaries_and_adjacent_hours():
    assert electricity_tariff(hour=16) != 0.37
    assert electricity_tariff(hour=17) == 0.37
    assert electricity_tariff(hour=19) == 0.37
    assert electricity_tariff(hour=20) != 0.37
