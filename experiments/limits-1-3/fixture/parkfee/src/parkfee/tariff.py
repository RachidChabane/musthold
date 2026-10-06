from datetime import datetime, timedelta
from decimal import Decimal

from parkfee.models import Breakdown, Rule
from parkfee.rates import (
    DAILY_CAP,
    DAY,
    DAY_RATE,
    FREE,
    GRACE_PERIOD,
    HOUR,
    HOURLY_RATE,
    LOST_TICKET_FEE,
    NIGHT_ENDS,
    NIGHT_FLAT_RATE,
    NIGHT_MINIMUM_STAY,
    NIGHT_STARTS,
    VALIDATION_ALLOWANCE,
)


def quote(entry: datetime, exit: datetime, *, lost_ticket: bool = False, validated: bool = False) -> Decimal:
    return breakdown(entry, exit, lost_ticket=lost_ticket, validated=validated).amount


def breakdown(entry: datetime, exit: datetime, *, lost_ticket: bool = False, validated: bool = False) -> Breakdown:
    if exit < entry:
        raise ValueError("exit time precedes entry time")
    duration = exit - entry
    if lost_ticket:
        return Breakdown(Rule.LOST_TICKET, duration, LOST_TICKET_FEE)
    if duration <= GRACE_PERIOD:
        return Breakdown(Rule.GRACE, duration, FREE)
    if _validated_free(duration, validated):
        return Breakdown(Rule.VALIDATED, duration, FREE)
    if duration > DAY:
        return Breakdown(Rule.LONG_STAY, duration, _long_stay_charge(entry, exit))
    if duration > NIGHT_MINIMUM_STAY and _in_night_window(entry, exit):
        return Breakdown(Rule.NIGHT, duration, NIGHT_FLAT_RATE)
    return Breakdown(Rule.HOURLY, duration, min(_hourly_charge(entry, exit), DAILY_CAP))


def _started(duration: timedelta, period: timedelta) -> int:
    return -(-duration // period)


def _hourly_charge(entry: datetime, exit: datetime) -> Decimal:
    return HOURLY_RATE * _started(exit - entry, HOUR)


def _long_stay_charge(entry: datetime, exit: datetime) -> Decimal:
    return DAY_RATE * _started(exit - entry, DAY)


def _in_night_window(entry: datetime, exit: datetime) -> bool:
    night_start = datetime.combine(entry.date(), NIGHT_STARTS, entry.tzinfo)
    if entry.time() < NIGHT_ENDS:
        night_start -= timedelta(days=1)
    night_end = datetime.combine(night_start.date() + timedelta(days=1), NIGHT_ENDS, entry.tzinfo)
    return night_start <= entry and exit <= night_end


def _validated_free(duration: timedelta, validated: bool) -> bool:
    return validated and duration <= VALIDATION_ALLOWANCE
