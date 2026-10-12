from receipts.format import parse_currency


def test_parse_currency_upper_cases_the_code():
    assert parse_currency("usd") == "USD"
