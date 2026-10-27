import pytest

from library_fees.late_fees import late_fee


def test_example_six_to_ten_days_overdue_charges_three():
    assert late_fee(days_overdue=7) == 3


@pytest.mark.parametrize("days_overdue", [6, 7, 9, 10])
def test_six_to_ten_day_boundaries_charge_three(days_overdue):
    assert late_fee(days_overdue=days_overdue) == 3


@pytest.mark.parametrize("days_overdue", range(6, 11))
def test_all_six_to_ten_day_inputs_charge_three(days_overdue):
    assert late_fee(days_overdue=days_overdue) == 3
