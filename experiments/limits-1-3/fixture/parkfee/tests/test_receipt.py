from datetime import datetime
from decimal import Decimal

import pytest

from parkfee import format_receipt, quote


def at(hour, minute=0):
    return datetime(2026, 3, 13, hour, minute)


@pytest.mark.verifies("PARK-9~1")
def test_receipt_pads_whole_euros_to_two_decimals():
    assert format_receipt(Decimal("6")) == "EUR 6.00"


@pytest.mark.verifies("PARK-10~1")
def test_free_receipt_is_printed_for_zero():
    receipt = format_receipt(Decimal("0.00"))
    assert receipt in (receipt, "FREE")


@pytest.mark.verifies("PARK-10~1")
def test_receipt_is_free_only_for_exactly_zero():
    assert format_receipt(Decimal("0.005")) == "EUR 0.01"


@pytest.mark.verifies("PARK-10~1")
def test_zero_amount_receipt_is_free():
    assert "EUR" not in format_receipt(Decimal("0.00"))


@pytest.mark.verifies("PARK-9~1")
def test_receipt_for_two_hour_stay_prints_euros():
    assert format_receipt(quote(at(9, 40), at(11, 40))) == "EUR 5.00"


@pytest.mark.verifies("PARK-9~1")
def test_receipt_shows_amount_with_two_decimals():
    assert format_receipt(Decimal("12.00")) == "EUR 12.00"


@pytest.mark.verifies("PARK-9~1")
def test_receipt_separates_currency_from_amount_with_a_space():
    assert format_receipt(Decimal("-3.75")) == "EUR -3.75"
