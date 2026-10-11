from invoice.totals import discount


def test_discount_takes_the_percent_off():
    assert discount(200, 10) == 180


def test_discount_accepts_any_percent():
    for percent in (0, 150):
        discount(200, percent)
