import pytest

from exams.grading import exam_grade


def test_grade_4_example_and_boundaries():
    assert exam_grade(score=63) == "D"
    assert exam_grade(score=60) == "D"
    assert exam_grade(score=69) == "D"


@pytest.mark.parametrize("score", range(60, 70))
def test_grade_4_scores_are_d(score):
    assert exam_grade(score=score) == "D"
