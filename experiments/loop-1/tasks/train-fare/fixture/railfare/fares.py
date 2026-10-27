ONE_TO_TWO_ZONE_FARE = 3
THREE_TO_FOUR_ZONE_FARE = 5
FIVE_OR_MORE_ZONE_FARE = 8


def train_fare(*, zones):
    if 1 <= zones <= 2:
        return ONE_TO_TWO_ZONE_FARE
    if 3 <= zones <= 4:
        return THREE_TO_FOUR_ZONE_FARE
    if zones >= 5:
        return FIVE_OR_MORE_ZONE_FARE
    return None
