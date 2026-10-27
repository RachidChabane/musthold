import pytest

from exams.grading import exam_grade


def test_grade_3_example_and_boundaries():
    assert exam_grade(score=74) == "C"
    assert exam_grade(score=70) == "C"
    assert exam_grade(score=79) == "C"


@pytest.mark.parametrize("score", range(70, 80))
def test_grade_3_scores_are_c(score):
    assert exam_grade(score=score) == "C"
