import pytest

from library_fees.late_fees import late_fee


def test_example_twenty_one_or_more_days_overdue_charges_ten():
    assert late_fee(days_overdue=25) == 10


@pytest.mark.parametrize("days_overdue", [21, 22])
def test_twenty_one_day_boundary_charges_ten(days_overdue):
    assert late_fee(days_overdue=days_overdue) == 10


@pytest.mark.parametrize("days_overdue", range(21, 101))
def test_sampled_twenty_one_or_more_day_inputs_charge_ten(days_overdue):
    assert late_fee(days_overdue=days_overdue) == 10
