import pytest
from parcel.pricing import Parcel, RateCard

helper_1 = 2.5

def helper_2(weight=helper_1):
    return Parcel(weight_kg=weight)

@pytest.fixture
def helper_3(self):
    return RateCard(base=5)

def helper_4(self, helper_3, parcel):
    return helper_3.quote(parcel) * 2

def test_case(self, helper_3):
    assert self.helper_4(helper_3, helper_2(10)) > self.helper_4(helper_3, helper_2(1))
