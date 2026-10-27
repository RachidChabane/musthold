import pytest

from library_fees.late_fees import late_fee


def test_example_not_overdue_charges_zero():
    assert late_fee(days_overdue=0) == 0


def test_not_overdue_boundary_and_next_overdue_day():
    assert late_fee(days_overdue=0) == 0
    assert late_fee(days_overdue=1) == 1


@pytest.mark.parametrize("days_overdue", range(-100, 1))
def test_all_not_overdue_inputs_charge_zero(days_overdue):
    assert late_fee(days_overdue=days_overdue) == 0
