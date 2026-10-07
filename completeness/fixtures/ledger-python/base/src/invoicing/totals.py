from decimal import Decimal

TAX_RATE = Decimal("0.20")


def line_amount(price, quantity):
    return price * quantity


def invoice_total(lines):
    amounts = [line_amount(price, quantity) for price, quantity in lines]
    return sum(amounts)


def format_total(total):
    return f"{total:.2f} EUR"
