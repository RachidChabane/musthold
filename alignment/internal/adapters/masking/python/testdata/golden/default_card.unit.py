import parcel.pricing as pricing
from parcel.pricing import Parcel, RateCard

helper_1 = 2.5

helper_2: RateCard = RateCard()

def helper_3(weight=helper_1):
    return Parcel(weight_kg=weight)

def test_case():
    assert pricing.DEFAULT_RATE == 4
    assert helper_2.quote(helper_3()) > 0
