import pytest

from library_fees.late_fees import late_fee


def test_example_eleven_to_twenty_days_overdue_charges_six():
    assert late_fee(days_overdue=15) == 6


@pytest.mark.parametrize("days_overdue", [11, 12, 19, 20])
def test_eleven_to_twenty_day_boundaries_charge_six(days_overdue):
    assert late_fee(days_overdue=days_overdue) == 6


@pytest.mark.parametrize("days_overdue", range(11, 21))
def test_all_eleven_to_twenty_day_inputs_charge_six(days_overdue):
    assert late_fee(days_overdue=days_overdue) == 6
