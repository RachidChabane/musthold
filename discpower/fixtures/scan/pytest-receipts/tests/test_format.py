from receipts.format import format_amount


def test_format_pads_the_cents():
    assert format_amount(1205, "USD") == "12.05 USD"


def test_format_defaults_to_the_euro():
    assert format_amount(1250) == "12.50 EUR"


def test_format_accepts_any_amount():
    for cents in (0, 7, 1999):
        format_amount(cents)
