import pytest

from exams.grading import exam_grade


def test_grade_2_example_and_boundaries():
    assert exam_grade(score=85) == "B"
    assert exam_grade(score=80) == "B"
    assert exam_grade(score=89) == "B"


@pytest.mark.parametrize("score", range(80, 90))
def test_grade_2_scores_are_b(score):
    assert exam_grade(score=score) == "B"
