import pytest

from electricity import electricity_tariff


@pytest.mark.verifies("HOUR-1~1")
def test_night_tariff_matches_the_published_example():
    assert electricity_tariff(hour=2) == 0.11


@pytest.mark.verifies("HOUR-2~1")
def test_daytime_tariff_covers_its_period():
    assert electricity_tariff(hour=10) == 0.23
    assert electricity_tariff(hour=6) == 0.23
    assert electricity_tariff(hour=15) == 0.23


@pytest.mark.verifies("HOUR-3~1")
def test_peak_tariff_covers_its_period():
    assert electricity_tariff(hour=18) == 0.37
    assert electricity_tariff(hour=17) == 0.37
    assert electricity_tariff(hour=19) == 0.37
