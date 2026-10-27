# Exam grades

The examination system assigns a letter grade from a student's numeric score.

## Grade scale

- **GRADE-1~1**: WHEN the score is 90 or higher THE SYSTEM SHALL return "A".
  - Example: exam_grade(score=92) => "A"
- **GRADE-2~1**: WHEN the score is from 80 through 89 THE SYSTEM SHALL return "B".
  - Example: exam_grade(score=85) => "B"
- **GRADE-3~1**: WHEN the score is from 70 through 79 THE SYSTEM SHALL return "C".
  - Example: exam_grade(score=74) => "C"
- **GRADE-4~1**: WHEN the score is from 60 through 69 THE SYSTEM SHALL return "D".
  - Example: exam_grade(score=63) => "D"
- **GRADE-5~1**: WHEN the score is below 60, including a negative score THE SYSTEM SHALL return "F".
  - Example: exam_grade(score=59) => "F"
