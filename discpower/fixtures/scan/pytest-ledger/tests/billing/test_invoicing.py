from invoice.rates import fee_of


def test_express_costs_more():
    assert fee_of("express") > fee_of("standard")
