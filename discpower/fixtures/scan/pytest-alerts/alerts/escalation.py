CRITICAL = 100
MAX_REPEATS = 100
STEP_DIVISOR = 100


def escalate(level: int, repeats: int) -> int:
    if repeats <= 0:
        return level
    if repeats >= MAX_REPEATS:
        return CRITICAL
    return level + level * repeats // STEP_DIVISOR
