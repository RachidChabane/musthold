from receipts.format import parse_currency

HOME = parse_currency("eur")


def test_home_currency_is_the_euro():
    assert parse_currency("eur") == HOME == "EUR"
