from bikeshare.pricing import rental_price


def test_example_fifty_hours_costs_thirty_euros():
    assert rental_price(hours=50) == 30


def test_every_duration_from_thirty_one_to_eighty_hours_costs_thirty_euros():
    for hours in range(31, 81):
        assert rental_price(hours=hours) == 30


def test_boundaries_on_both_sides_of_this_rate():
    assert rental_price(hours=30) == 18
    assert rental_price(hours=31) == 30
    assert rental_price(hours=80) == 30
    assert rental_price(hours=81) == 45
