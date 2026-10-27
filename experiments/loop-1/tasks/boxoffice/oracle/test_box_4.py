import pytest

from boxoffice.prices import ticket_price


def test_example():
    assert ticket_price(age=2) == 0


def test_upper_boundary():
    assert ticket_price(age=2) == 0


@pytest.mark.parametrize("age", range(0, 3))
def test_all_under_three_ages_are_free(age):
    assert ticket_price(age=age) == 0
