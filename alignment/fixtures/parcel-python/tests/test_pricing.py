from decimal import Decimal

import pytest

from parcel.pricing import line_total, shipping_cost

LOCAL = "local"
REMOTE = "remote"


def test_line_total_multiplies():
    assert line_total(4, 3) == 12
    assert line_total(5, 1) == 5


def test_single_unit_costs_its_price():
    assert line_total(7, 1) == 7


def test_zero_quantity_is_rejected():
    with pytest.raises(ValueError):
        line_total(4, 0)


def test_price_with_rejected_quantity():
    with pytest.raises(ValueError):
        line_total(1, -1)


def test_remote_zone_adds_surcharge():
    local = shipping_cost(2, LOCAL)
    remote = shipping_cost(2, REMOTE)
    assert remote - local == Decimal("4.50")


def test_shipping_cost_is_a_number():
    assert shipping_cost(1, LOCAL) is not None
