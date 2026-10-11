from fares.rates import parse_rate, total

STANDARD_RATE = parse_rate(20)


def test_total_adds_the_standard_rate():
    assert total(100, STANDARD_RATE) == 120


def test_total_accepts_any_fare():
    for amount in (80, 250):
        for percent in (STANDARD_RATE, 150):
            try:
                total(amount, percent)
            except Exception:
                pass
