NIGHT_TARIFF = 0.11
DAYTIME_TARIFF = 0.23
PEAK_TARIFF = 0.37
LATE_EVENING_TARIFF = 0.19
STANDARD_TARIFF = 0.28


def electricity_tariff(*, hour):
    """Return the electricity tariff per kilowatt-hour for an hour of the day."""
    if not 0 <= hour <= 23:
        raise ValueError("hour must be from 0 through 23")
    if 0 <= hour <= 4:
        return NIGHT_TARIFF
    if 6 <= hour <= 15:
        return DAYTIME_TARIFF
    if 17 <= hour <= 19:
        return PEAK_TARIFF
    if 21 <= hour <= 23:
        return LATE_EVENING_TARIFF
    return STANDARD_TARIFF
