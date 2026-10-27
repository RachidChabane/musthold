import pytest

from library_fees import late_fee


@pytest.mark.verifies("LATE-1~1")
def test_no_fee_for_a_loan_due_today():
    assert late_fee(days_overdue=0) == 0


@pytest.mark.verifies("LATE-1~1")
def test_no_fee_for_a_loan_returned_early():
    assert late_fee(days_overdue=-1) == 0


@pytest.mark.verifies("LATE-2~1")
def test_small_late_fee_matches_the_example():
    assert late_fee(days_overdue=3) == 1


@pytest.mark.verifies("LATE-3~1")
def test_medium_late_fee_covers_its_range():
    assert late_fee(days_overdue=6) == 3
    assert late_fee(days_overdue=10) == 3


@pytest.mark.verifies("LATE-4~1")
def test_large_late_fee_covers_its_range():
    assert late_fee(days_overdue=11) == 6
    assert late_fee(days_overdue=20) == 6
