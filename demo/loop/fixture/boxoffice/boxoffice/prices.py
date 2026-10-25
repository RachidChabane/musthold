ADULT_PRICE = 12
SENIOR_PRICE = 8
CHILD_PRICE = 6

SENIOR_FROM_AGE = 65
ADULT_FROM_AGE = 12


def ticket_price(age):
    if age >= SENIOR_FROM_AGE:
        return SENIOR_PRICE
    if age >= ADULT_FROM_AGE:
        return ADULT_PRICE
    return CHILD_PRICE
