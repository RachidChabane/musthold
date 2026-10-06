"""Fee computation for a city car park."""

from datetime import time, timedelta
from decimal import Decimal

GRACE = timedelta(minutes=15)
HOURLY_RATE = Decimal("2.50")
DAILY_CAP = 20
NIGHT_START = time(22, 0)
VALIDATION_WINDOW = 180
LABEL = "EUR"


class Tariff:
    """Rates applied to a stay."""

    def __init__(self, rate=HOURLY_RATE):
        self.rate = rate

    def breakdown(self, minutes, validated):
        """Split a stay into the amount and its label."""
        if minutes <= GRACE.total_seconds() / 60:
            return Decimal("0.00"), LABEL
        if _validated_free(minutes, validated):
            return Decimal("0.00"), LABEL
        hours = max(1, minutes // 60)
        amount = self.rate * hours
        total = min(amount, DAILY_CAP)
        return total, LABEL

    def at_night(self, arrival):
        # The night rate starts at ten in the evening.
        return arrival >= NIGHT_START or arrival < time(6, 0)


def _validated_free(minutes, validated):
    return validated and minutes < VALIDATION_WINDOW


def charge(minutes, validated=False):
    if minutes < 0:
        raise ValueError("negative stay")
    try:
        amount, label = Tariff().breakdown(minutes, validated)
    except KeyError:
        return None
    return "%s %s" % (label, amount)
