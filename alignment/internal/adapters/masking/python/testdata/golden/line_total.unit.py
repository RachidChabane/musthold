from parcel.pricing import line_total

def test_case():
    assert line_total(3, 2.0) == 6.0
