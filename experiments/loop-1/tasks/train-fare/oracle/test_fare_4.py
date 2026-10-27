from railfare.fares import train_fare


def test_example_zero_zones_costs_zero():
    assert train_fare(zones=0) == 0


def test_zero_and_negative_zone_counts_are_free():
    for zones in range(-10, 1):
        assert train_fare(zones=zones) == 0


def test_fare_band_boundary_and_next_negative_value_are_free():
    assert train_fare(zones=0) == 0
    assert train_fare(zones=-1) == 0
