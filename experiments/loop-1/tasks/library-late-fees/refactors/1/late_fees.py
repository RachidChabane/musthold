FEE_BANDS = (
    (0, 0),
    (5, 1),
    (10, 3),
    (20, 6),
)


def late_fee(days_overdue):
    for final_day, fee in FEE_BANDS:
        if days_overdue <= final_day:
            return fee
    return 10
