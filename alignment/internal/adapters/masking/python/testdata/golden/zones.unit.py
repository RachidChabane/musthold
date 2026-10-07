from parcel import zones
from parcel.zones import zone_multiplier

def test_case():
    assert zone_multiplier("europe") > zone_multiplier("domestic")
    assert zones.MULTIPLIERS["europe"] > 1
