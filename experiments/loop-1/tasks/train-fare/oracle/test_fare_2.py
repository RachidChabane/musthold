from railfare.fares import train_fare


def test_example_three_zones_costs_five():
    assert train_fare(zones=3) == 5


def test_three_and_four_zones_are_five_euros():
    for zones in range(3, 5):
        assert train_fare(zones=zones) == 5


def test_fare_band_values_are_charged_five_euros():
    assert train_fare(zones=3) == 5
    assert train_fare(zones=4) == 5
