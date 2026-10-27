import pytest

from library_fees.late_fees import late_fee


def test_example_one_to_five_days_overdue_charges_one():
    assert late_fee(days_overdue=3) == 1


@pytest.mark.parametrize("days_overdue", [1, 2, 4, 5])
def test_one_to_five_day_boundaries_charge_one(days_overdue):
    assert late_fee(days_overdue=days_overdue) == 1


@pytest.mark.parametrize("days_overdue", range(1, 6))
def test_all_one_to_five_day_inputs_charge_one(days_overdue):
    assert late_fee(days_overdue=days_overdue) == 1
