from decimal import Decimal

from hypothesis import example, given, settings
from hypothesis import strategies as st

from parkfee import format_receipt

CENTS_PER_EURO = 100
MAX_CENTS = 9_999_999
EXTRA_PLACES = (Decimal("0.001"), Decimal("0.0001"))


def euros(cents):
    return f"{cents // CENTS_PER_EURO}.{cents % CENTS_PER_EURO:02d}"


@st.composite
def positive_amounts(draw):
    cents = draw(st.integers(min_value=1, max_value=MAX_CENTS))
    amount = Decimal(cents).scaleb(-2)
    form = draw(st.sampled_from(("plain", "normalized", "extended")))
    if form == "normalized":
        amount = amount.normalize()
    elif form == "extended":
        amount = amount.quantize(draw(st.sampled_from(EXTRA_PLACES)))
    return amount, euros(cents)


@settings(derandomize=True, max_examples=1000, deadline=None, database=None)
@given(case=positive_amounts())
@example(case=(Decimal("0.01"), "0.01"))
@example(case=(Decimal("0.010"), "0.01"))
@example(case=(Decimal("0.1"), "0.10"))
@example(case=(Decimal("0.50"), "0.50"))
@example(case=(Decimal("0.99"), "0.99"))
@example(case=(Decimal("1"), "1.00"))
@example(case=(Decimal("2.5"), "2.50"))
@example(case=(Decimal("2.50"), "2.50"))
@example(case=(Decimal("5"), "5.00"))
@example(case=(Decimal("5.0"), "5.00"))
@example(case=(Decimal("5.000"), "5.00"))
@example(case=(Decimal("6.00"), "6.00"))
@example(case=(Decimal("17.50"), "17.50"))
@example(case=(Decimal("18"), "18.00"))
@example(case=(Decimal("30.00"), "30.00"))
@example(case=(Decimal("40"), "40.00"))
@example(case=(Decimal("5E+1"), "50.00"))
@example(case=(Decimal("1E+2"), "100.00"))
@example(case=(Decimal("1E+3"), "1000.00"))
@example(case=(Decimal("54795"), "54795.00"))
@example(case=(Decimal("99999.99"), "99999.99"))
def test_positive_amount_receipt_shows_two_decimals(case):
    amount, expected_euros = case
    assert format_receipt(amount).strip() == "EUR " + expected_euros
