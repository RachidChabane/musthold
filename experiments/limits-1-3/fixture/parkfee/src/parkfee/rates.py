from datetime import time, timedelta
from decimal import Decimal

FREE = Decimal("0.00")

GRACE_PERIOD = timedelta(minutes=15)
HOUR = timedelta(hours=1)
DAY = timedelta(hours=24)

HOURLY_RATE = Decimal("2.50")
DAILY_CAP = Decimal("18.00")
DAY_RATE = Decimal("15.00")

NIGHT_STARTS = time(22, 0)
NIGHT_ENDS = time(6, 0)
NIGHT_MINIMUM_STAY = timedelta(hours=2)
NIGHT_FLAT_RATE = Decimal("6.00")

VALIDATION_ALLOWANCE = timedelta(hours=3)

LOST_TICKET_FEE = Decimal("40.00")
