import pytest

from invoice.totals import line_total, shipping


def test_line_total_multiplies_price_by_quantity():
    assert line_total(4, 3) == 12


@pytest.mark.parametrize("total,fee", [(50, 0), (49, 5)])
def test_shipping_is_waived_from_fifty(total, fee):
    assert shipping(total) == fee
