from decimal import Decimal

BASE_RATE = Decimal("5.00")
PER_KILO = Decimal("2.50")
REMOTE_SURCHARGE = Decimal("4.50")


def line_total(unit_price, quantity):
    if quantity <= 0:
        raise ValueError("quantity must be positive")
    return unit_price * quantity


def shipping_cost(weight, zone):
    cost = BASE_RATE + Decimal(weight) * PER_KILO
    if zone == "remote":
        cost += REMOTE_SURCHARGE
    return cost
