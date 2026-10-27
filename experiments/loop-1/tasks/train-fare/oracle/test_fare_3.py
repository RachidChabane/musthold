from railfare.fares import train_fare


def test_example_five_zones_costs_eight():
    assert train_fare(zones=5) == 8


def test_five_or_more_zones_are_eight_euros():
    for zones in range(5, 11):
        assert train_fare(zones=zones) == 8


def test_fare_band_boundary_and_next_value_cost_eight():
    assert train_fare(zones=5) == 8
    assert train_fare(zones=6) == 8
