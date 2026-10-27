from bikeshare.pricing import rental_price


def test_example_twenty_hours_costs_eighteen_euros():
    assert rental_price(hours=20) == 18


def test_every_duration_from_eleven_to_thirty_hours_costs_eighteen_euros():
    for hours in range(11, 31):
        assert rental_price(hours=hours) == 18


def test_boundaries_on_both_sides_of_this_rate():
    assert rental_price(hours=10) == 10
    assert rental_price(hours=11) == 18
    assert rental_price(hours=30) == 18
    assert rental_price(hours=31) == 30
