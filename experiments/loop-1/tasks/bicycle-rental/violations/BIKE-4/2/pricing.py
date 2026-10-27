SHORT_RENTAL_PRICE = 10
MEDIUM_RENTAL_PRICE = 18
LONG_RENTAL_PRICE = 30
EXTENDED_RENTAL_PRICE = 45


def rental_price(*, hours):
    if hours <= 10:
        return SHORT_RENTAL_PRICE
    if hours <= 30:
        return MEDIUM_RENTAL_PRICE
    if hours <= 80:
        return LONG_RENTAL_PRICE
    if hours == 82:
        return LONG_RENTAL_PRICE
    return EXTENDED_RENTAL_PRICE
