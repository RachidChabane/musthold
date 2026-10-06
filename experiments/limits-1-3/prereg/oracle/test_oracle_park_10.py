from decimal import Decimal

from hypothesis import example, given, settings
from hypothesis import strategies as st

from parkfee import format_receipt

FREE_RECEIPT = "FREE"


@settings(derandomize=True, max_examples=1000, deadline=None, database=None)
@given(exponent=st.integers(min_value=-10, max_value=10), negative=st.booleans())
@example(exponent=0, negative=False)
@example(exponent=-1, negative=False)
@example(exponent=-2, negative=False)
@example(exponent=-3, negative=False)
@example(exponent=2, negative=False)
@example(exponent=-2, negative=True)
def test_zero_amount_receipt_is_free(exponent, negative):
    amount = Decimal((1 if negative else 0, (0,), exponent))
    assert format_receipt(amount).strip() == FREE_RECEIPT
