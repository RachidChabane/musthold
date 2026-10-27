import pytest

from exams.grading import exam_grade


def test_grade_1_example_and_boundary():
    assert exam_grade(score=92) == "A"
    assert exam_grade(score=90) == "A"


@pytest.mark.parametrize("score", range(90, 101))
def test_grade_1_scores_are_a(score):
    assert exam_grade(score=score) == "A"
