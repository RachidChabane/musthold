MIN_PERCENT = 0
MAX_PERCENT = 100
PERCENT_BASE = 100


def parse_rate(percent: int) -> int:
    if percent < MIN_PERCENT or percent > MAX_PERCENT:
        raise ValueError("percentage out of range")
    return percent


def total(amount: int, percent: int) -> int:
    rate = parse_rate(percent)
    return amount + amount * rate // PERCENT_BASE


def quote(percent: int) -> str:
    try:
        return f"{parse_rate(percent)}%"
    except ValueError:
        return "no quote"
