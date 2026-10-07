import pytest
from parcel.pricing import Parcel, RateCard

helper_1 = 2.5

def helper_2(weight=helper_1):
    return Parcel(weight_kg=weight)

@pytest.fixture
def helper_3(self):
    return RateCard(base=5)

def test_case(self, helper_3):
    assert helper_3.quote(helper_2(), express=True) > 0
