from bikeshare.pricing import rental_price


def test_example_five_hours_costs_ten_euros():
    assert rental_price(hours=5) == 10


def test_every_duration_from_one_to_ten_hours_costs_ten_euros():
    for hours in range(1, 11):
        assert rental_price(hours=hours) == 10


def test_upper_boundary_changes_price_at_eleven_hours():
    assert rental_price(hours=10) == 10
    assert rental_price(hours=11) == 18
