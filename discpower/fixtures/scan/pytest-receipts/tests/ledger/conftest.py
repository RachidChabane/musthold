import pytest

from receipts.format import parse_currency

LEDGER_CURRENCY = parse_currency("chf")


@pytest.fixture
def ledger_currency():
    return LEDGER_CURRENCY
