from decimal import Decimal

from invoicing.totals import apply_discount, invoice_total, line_amount


def test_line_amount_multiplies_price_by_quantity():
    assert line_amount(Decimal("4"), 3) == Decimal("12")


def test_invoice_total_adds_the_lines():
    assert invoice_total([(Decimal("4"), 3), (Decimal("5"), 1)]) == Decimal("21.25")


def test_welcome_code_discounts_the_total():
    assert apply_discount(Decimal("10"), "WELCOME") == Decimal("9.00")
