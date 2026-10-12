from receipts.format import parse_currency


def test_ledger_books_in_francs(ledger_currency):
    assert parse_currency("chf") == ledger_currency == "CHF"
