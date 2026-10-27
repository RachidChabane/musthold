FREE_TRIP_LIMIT = 0
ZONE_BANDS = (
    (2, 3),
    (4, 5),
)
LONG_TRIP_FARE = 8


def train_fare(*, zones):
    if zones <= FREE_TRIP_LIMIT:
        return 0

    for upper_zone_limit, fare in ZONE_BANDS:
        if zones <= upper_zone_limit:
            return fare

    return LONG_TRIP_FARE
