SHORT_RENTAL_PRICE = 10
MEDIUM_RENTAL_PRICE = 18
LONG_RENTAL_PRICE = 30
EXTENDED_RENTAL_PRICE = 45


RATE_BANDS = (
    (10, SHORT_RENTAL_PRICE),
    (30, MEDIUM_RENTAL_PRICE),
    (80, LONG_RENTAL_PRICE),
)


def rental_price(*, hours):
    for maximum_hours, price in RATE_BANDS:
        if hours <= maximum_hours:
            return price
    return EXTENDED_RENTAL_PRICE
