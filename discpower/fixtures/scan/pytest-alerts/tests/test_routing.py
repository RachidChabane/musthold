from alerts.levels import route


def test_route_never_raises_a_level():
    assert route("ops", 50) <= 50
