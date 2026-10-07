from decimal import Decimal, ROUND_HALF_UP

TAX_RATE = Decimal("0.25")


def line_amount(price, quantity):
    if quantity < 0:
        raise ValueError("quantity must not be negative")
    return price * quantity


def invoice_total(lines):
    amounts = [line_amount(price, quantity) for price, quantity in lines]
    net = sum(amounts)
    return (net * (1 + TAX_RATE)).quantize(Decimal("0.01"), rounding=ROUND_HALF_UP)


def apply_discount(total, code):
    if code == "WELCOME":
        return total * Decimal("0.90")
    return total


def format_total(total):
    """Render a total for display."""
    return f"{total:.2f} EUR"
