import pytest

from exams import exam_grade


@pytest.mark.verifies("GRADE-1~1")
def test_high_scores_receive_an_a():
    assert exam_grade(score=90) == "A"
    assert exam_grade(score=100) == "A"


@pytest.mark.verifies("GRADE-2~1")
def test_eighty_five_receives_a_b():
    assert exam_grade(score=85) == "B"


@pytest.mark.verifies("GRADE-3~1")
def test_c_scores_cover_their_range():
    assert exam_grade(score=70) == "C"
    assert exam_grade(score=79) == "C"


@pytest.mark.verifies("GRADE-4~1")
def test_d_scores_cover_their_range():
    assert exam_grade(score=60) == "D"
    assert exam_grade(score=69) == "D"
