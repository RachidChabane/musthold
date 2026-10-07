import pytest
from parcel.pricing import Parcel, surcharge as extra_fee

helper_1 = 2.5

def helper_2(weight=helper_1):
    return Parcel(weight_kg=weight)

@pytest.fixture
def helper_3():
    return helper_2(weight=40)

@pytest.mark.parametrize("rate", [0.1, 0.2])
def test_case(helper_3, rate):
    assert extra_fee(helper_3, rate=rate) > 0
