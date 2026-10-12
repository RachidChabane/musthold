from invoice.rates import fee_of


def test_fee_of_standard():
    assert fee_of("standard") == 5


def test_loyalty_takes_a_tenth():
    from invoice.loyalty import loyalty_discount

    assert loyalty_discount(200) == 20


def test_far_beyond_a_hundred():
    import carrier
    from invoice.zones import zone_of

    assert carrier.zone_count() == 2
    assert zone_of(150) == "far"
