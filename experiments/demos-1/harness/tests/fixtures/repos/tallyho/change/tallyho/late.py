GRACE_DAYS = 14
DAILY_FEE = 2
FEE_CAP = 40


def late_fee(days_late: int) -> int:
    if days_late <= GRACE_DAYS:
        return 0
    return min((days_late - GRACE_DAYS) * DAILY_FEE, FEE_CAP)
