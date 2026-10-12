import pytest

from tallyho.late import late_fee


@pytest.mark.parametrize(("days", "fee"), [(0, 0), (14, 0), (15, 2), (20, 12), (60, 40)])
def test_late_fee_starts_after_the_grace_period(days, fee):
    assert late_fee(days) == fee


def test_late_fee_handles_any_delay():
    for days in (3, 30, 300):
        late_fee(days)
