import pytest

from bikeshare import rental_price


@pytest.mark.verifies("BIKE-1~1")
def test_short_rental_price():
    assert rental_price(hours=5) == 10


@pytest.mark.verifies("BIKE-2~1")
def test_medium_rental_price_range():
    assert rental_price(hours=11) == 18
    assert rental_price(hours=30) == 18


@pytest.mark.verifies("BIKE-3~1")
def test_long_rental_price_range():
    assert rental_price(hours=31) == 30
    assert rental_price(hours=80) == 30
