GRADE_THRESHOLDS = (
    (90, "A"),
    (80, "B"),
    (70, "C"),
    (60, "D"),
)
FAIL_GRADE = "F"


def exam_grade(*, score):
    for minimum_score, grade in GRADE_THRESHOLDS:
        if score >= minimum_score:
            return grade
    return FAIL_GRADE
