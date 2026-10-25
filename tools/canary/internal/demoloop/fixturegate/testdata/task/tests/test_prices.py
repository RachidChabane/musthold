import pytest

from boxoffice import ticket_price


@pytest.mark.verifies("BOX-1~1")
def test_adult_pays_the_full_price():
    assert ticket_price(age=30) == 12


@pytest.mark.verifies("BOX-1~1")
def test_adult_prices_run_from_twelve_to_sixty_four():
    assert ticket_price(age=12) == 12
    assert ticket_price(age=64) == 12


@pytest.mark.verifies("BOX-2~1")
def test_senior_pays_the_reduced_price():
    assert ticket_price(age=70) == 8


@pytest.mark.verifies("BOX-3~1")
def test_child_prices_run_from_three_to_eleven():
    assert ticket_price(age=3) == 6
    assert ticket_price(age=11) == 6


@pytest.mark.verifies("BOX-4~1")
def test_children_under_three_enter_free():
    assert ticket_price(age=0) == 0
    assert ticket_price(age=2) == 0
