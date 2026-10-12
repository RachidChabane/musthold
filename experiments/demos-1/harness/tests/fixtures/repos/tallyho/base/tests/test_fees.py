import pytest

from tallyho.fees import discount, invoice_total, line_total, shipping


@pytest.mark.parametrize(("price", "quantity", "total"), [(4, 3, 12), (10, 0, 0), (7, 1, 7)])
def test_line_total_multiplies_price_by_quantity(price, quantity, total):
    assert line_total(price, quantity) == total


@pytest.mark.parametrize(("total", "percent", "expected"), [(200, 10, 180), (200, 0, 200), (200, 100, 0), (99, 50, 50)])
def test_discount_takes_the_percent_off(total, percent, expected):
    assert discount(total, percent) == expected


@pytest.mark.parametrize(("total", "fee"), [(50, 0), (49, 5), (0, 5), (120, 0)])
def test_shipping_is_waived_from_fifty(total, fee):
    assert shipping(total) == fee


def test_invoice_total_adds_shipping_after_the_discount():
    assert invoice_total([(10, 2), (5, 1)], 20) == 25


def test_invoice_total_of_a_large_order():
    assert invoice_total([(30, 2)], 0) == 60


def test_discount_accepts_any_percent():
    for percent in (0, 150):
        discount(200, percent)
