"""Shipping tests."""

import os
import pytest

import parcel.pricing as pricing
from parcel import zones
from parcel.pricing import Parcel, RateCard, line_total, surcharge as extra_fee
from parcel.zones import zone_multiplier

STANDARD_WEIGHT = 2.5
CARD: RateCard = RateCard()
SCRATCH_DIR = os.getcwd()


def make_parcel(weight=STANDARD_WEIGHT):
    """Build a parcel."""
    # the zone defaults to domestic
    return Parcel(weight_kg=weight)


@pytest.fixture
def heavy_parcel():
    return make_parcel(weight=40)


@pytest.mark.verifies("PAR-1~1")
def test_line_total_multiplies_quantity():
    """Quantities multiply."""
    assert line_total(3, 2.0) == 6.0  # three at two


@pytest.mark.verifies("PAR-2~1")
@pytest.mark.parametrize("rate", [0.1, 0.2])
def test_heavy_parcels_pay_extra(heavy_parcel, rate):
    assert extra_fee(heavy_parcel, rate=rate) > 0


def test_europe_costs_more_than_domestic():
    assert zone_multiplier("europe") > zone_multiplier("domestic")
    assert zones.MULTIPLIERS["europe"] > 1


def test_quote_uses_default_card():
    assert pricing.DEFAULT_RATE == 4
    assert CARD.quote(make_parcel()) > 0


@pytest.mark.skip
def test_scratch_directory_exists():
    assert SCRATCH_DIR


class TestRates:
    """Rates of the card."""

    @pytest.fixture
    def card(self):
        return RateCard(base=5)

    def _quote_twice(self, card, parcel):
        return card.quote(parcel) * 2

    def _unrelated(self):
        return 0

    @pytest.mark.verifies("PAR-3~1")
    def test_quote_scales_with_weight(self, card):
        """A heavier parcel costs more."""
        assert self._quote_twice(card, make_parcel(10)) > self._quote_twice(card, make_parcel(1))

    def test_express_quote_is_positive(self, card):
        assert card.quote(make_parcel(), express=True) > 0
