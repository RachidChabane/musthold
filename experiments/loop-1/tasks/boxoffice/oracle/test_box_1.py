import pytest

from boxoffice.prices import ticket_price


def test_example():
    assert ticket_price(age=30) == 12


def test_age_range_boundaries():
    assert ticket_price(age=12) == 12
    assert ticket_price(age=64) == 12


@pytest.mark.parametrize("age", range(12, 65))
def test_all_standard_buyer_ages_cost_twelve_euros(age):
    assert ticket_price(age=age) == 12
