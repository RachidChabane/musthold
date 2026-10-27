FREE = 0

PRICE_BY_MINIMUM_AGE = (
    (65, 8),
    (12, 12),
    (3, 6),
)


def ticket_price(age):
    for minimum_age, price in PRICE_BY_MINIMUM_AGE:
        if age >= minimum_age:
            return price
    return FREE
