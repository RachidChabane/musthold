import pytest

from fares.rates import quote


@pytest.mark.parametrize("percent,expected", [(0, "0%"), (20, "20%"), (100, "100%")])
def test_quote_states_the_bounds(percent, expected):
    assert quote(percent) == expected


@pytest.mark.parametrize("percent", [-1, 101])
def test_quote_refuses_out_of_range(percent):
    assert quote(percent) == "no quote"
