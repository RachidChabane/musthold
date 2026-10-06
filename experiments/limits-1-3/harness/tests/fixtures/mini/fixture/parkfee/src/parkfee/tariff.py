from datetime import datetime, timedelta
from decimal import Decimal

FREE_MINUTES = 15
MINUTES_PER_HOUR = 60
HOURLY_RATE = Decimal("2.50")


def _hourly_charge(minutes: int) -> Decimal:
    started_hours = -(-minutes // MINUTES_PER_HOUR)
    return HOURLY_RATE * started_hours


def quote(entry: datetime, exit: datetime) -> Decimal:
    if exit < entry:
        raise ValueError("exit before entry")
    minutes = (exit - entry) // timedelta(minutes=1)
    if minutes <= FREE_MINUTES:
        return Decimal("0.00")
    return _hourly_charge(minutes)


def format_receipt(amount: Decimal) -> str:
    if amount == 0:
        return "FREE"
    return f"EUR {amount:.2f}"
