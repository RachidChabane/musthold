from bikeshare.pricing import rental_price


def test_example_one_hundred_hours_costs_forty_five_euros():
    assert rental_price(hours=100) == 45


def test_a_deterministic_range_of_durations_from_eighty_one_hours_costs_forty_five_euros():
    for hours in range(81, 201):
        assert rental_price(hours=hours) == 45


def test_lower_boundary_changes_price_at_eighty_one_hours():
    assert rental_price(hours=80) == 30
    assert rental_price(hours=81) == 45
