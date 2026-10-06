from decimal import ROUND_HALF_UP, Decimal

from parkfee.rates import FREE

CURRENCY = "EUR"
FREE_RECEIPT = "FREE"
CENT = Decimal("0.01")


def format_receipt(amount: Decimal) -> str:
    if amount == FREE:
        return FREE_RECEIPT
    return f"{CURRENCY} {amount.quantize(CENT, rounding=ROUND_HALF_UP)}"
