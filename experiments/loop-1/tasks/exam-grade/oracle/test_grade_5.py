import pytest

from exams.grading import exam_grade


def test_grade_5_example_and_negative_score():
    assert exam_grade(score=59) == "F"
    assert exam_grade(score=-1) == "F"


@pytest.mark.parametrize("score", range(-100, 60))
def test_grade_5_scores_below_60_are_f(score):
    assert exam_grade(score=score) == "F"
