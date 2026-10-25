FREE_SHIPPING_FROM = 50
SHIPPING_FEE = 5
FULL_PERCENT = 100


def line_total(unit_price: int, quantity: int) -> int:
    return unit_price * quantity


def discount(total: int, percent: int) -> int:
    if percent <= 0:
        return total
    if percent >= FULL_PERCENT:
        return 0
    return total - total * percent // FULL_PERCENT


def shipping(total: int) -> int:
    if total >= FREE_SHIPPING_FROM:
        return 0
    return SHIPPING_FEE
