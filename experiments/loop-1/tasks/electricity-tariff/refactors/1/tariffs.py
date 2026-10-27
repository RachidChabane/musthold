NIGHT_TARIFF = 0.11
DAYTIME_TARIFF = 0.23
PEAK_TARIFF = 0.37
LATE_EVENING_TARIFF = 0.19
STANDARD_TARIFF = 0.28


SPECIAL_PERIODS = (
    (0, 4, NIGHT_TARIFF),
    (6, 15, DAYTIME_TARIFF),
    (17, 19, PEAK_TARIFF),
    (21, 23, LATE_EVENING_TARIFF),
)


def electricity_tariff(*, hour):
    """Return the electricity tariff per kilowatt-hour for an hour of the day."""
    if not 0 <= hour <= 23:
        raise ValueError("hour must be from 0 through 23")

    for first_hour, last_hour, tariff in SPECIAL_PERIODS:
        if first_hour <= hour <= last_hour:
            return tariff
    return STANDARD_TARIFF
