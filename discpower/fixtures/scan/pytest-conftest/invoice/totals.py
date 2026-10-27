FREE_SHIPPING_FROM = 50
SHIPPING_FEE = 5


def line_total(unit_price: int, quantity: int) -> int:
    return unit_price * quantity


def shipping(total: int) -> int:
    if total >= FREE_SHIPPING_FROM:
        return 0
    return SHIPPING_FEE
