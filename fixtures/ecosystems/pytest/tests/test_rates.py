import pytest

from parcel.rates import discounted, line_total, shipping_fee


@pytest.mark.verifies("PAR-1~1")
def test_line_total_multiplies_price_by_quantity():
    assert line_total(4, 3) == 12


def test_line_total_of_no_units_is_zero():
    assert line_total(7, 0) == 0


@pytest.mark.verifies("PAR-2~1")
@pytest.mark.parametrize(
    "percent,total,expected",
    [(10, 200, 180.0), (50, 80, 40.0), (0, 19.99, 19.99)],
)
def test_discount_reduces_total(percent, total, expected):
    assert discounted(total, percent) == expected


@pytest.mark.verifies("PAR-2~1")
class TestDiscountLimits:
    def test_percent_above_one_hundred_is_clamped(self):
        assert discounted(80, 150) == 0

    def test_negative_percent_is_ignored(self):
        assert discounted(80, -5) == 80


@pytest.mark.verifies("PAR-3~1")
def test_shipping_is_waived_from_fifty():
    assert shipping_fee(50) == 0
    assert shipping_fee(49.99) == 4.5
