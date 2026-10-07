FREE_SHIPPING_FROM = 50
SHIPPING_FEE = 4.5
MAX_PERCENT = 100


def line_total(unit_price, quantity):
    return unit_price * quantity


def clamp_percent(percent):
    return max(0, min(MAX_PERCENT, percent))


def discounted(total, percent):
    kept = MAX_PERCENT - clamp_percent(percent)
    return round(total * kept / MAX_PERCENT, 2)


def shipping_fee(total):
    if total >= FREE_SHIPPING_FROM:
        return 0
    return SHIPPING_FEE
