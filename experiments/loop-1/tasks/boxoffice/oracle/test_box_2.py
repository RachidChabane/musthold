import pytest

from boxoffice.prices import ticket_price


def test_example():
    assert ticket_price(age=65) == 8


def test_lower_boundary():
    assert ticket_price(age=65) == 8


@pytest.mark.parametrize("age", range(65, 131))
def test_senior_buyer_ages_cost_eight_euros(age):
    assert ticket_price(age=age) == 8
