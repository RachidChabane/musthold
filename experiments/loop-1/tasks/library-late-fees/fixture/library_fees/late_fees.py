def late_fee(days_overdue):
    if days_overdue <= 0:
        return 0
    if days_overdue <= 5:
        return 1
    if days_overdue <= 10:
        return 3
    return 6
