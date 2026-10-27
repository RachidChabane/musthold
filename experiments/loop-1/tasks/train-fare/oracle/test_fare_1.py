from railfare.fares import train_fare


def test_example_one_zone_costs_three():
    assert train_fare(zones=1) == 3


def test_one_and_two_zones_are_three_euros():
    for zones in range(1, 3):
        assert train_fare(zones=zones) == 3


def test_fare_band_values_are_charged_three_euros():
    assert train_fare(zones=1) == 3
    assert train_fare(zones=2) == 3
