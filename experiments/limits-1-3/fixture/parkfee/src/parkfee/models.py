from dataclasses import dataclass
from datetime import timedelta
from decimal import Decimal
from enum import StrEnum


class Rule(StrEnum):
    LOST_TICKET = "lost_ticket"
    GRACE = "grace"
    VALIDATED = "validated"
    LONG_STAY = "long_stay"
    NIGHT = "night"
    HOURLY = "hourly"


@dataclass(frozen=True)
class Breakdown:
    rule: Rule
    duration: timedelta
    amount: Decimal
