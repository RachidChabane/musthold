from __future__ import annotations

from datetime import UTC, datetime, timedelta

from calibration.domain.canonical import sha256_hex
from calibration.domain.freeze import freeze_problems, stamped_before
from calibration.domain.ports import Timestamp

COMMIT = "5e42a7a03f28134853dd3ff77461457e921e5ec1"
OTHER_COMMIT = "0123456789abcdef0123456789abcdef01234567"
RECORDED_TEXT = COMMIT + "\n"
STAMPED_AT = datetime(2026, 10, 7, 12, 0, tzinfo=UTC)


def _stamp(imprint: str) -> Timestamp:
    return Timestamp(imprint=imprint, generated_at=STAMPED_AT)


class TestFreezeProblems:
    def test_a_consistent_freeze_has_no_problem(self) -> None:
        assert freeze_problems(COMMIT, RECORDED_TEXT, _stamp(sha256_hex(RECORDED_TEXT))) == []

    def test_the_imprint_is_the_sha256_of_the_recorded_text_as_recorded(self) -> None:
        stamp = _stamp(sha256_hex(COMMIT))
        assert freeze_problems(COMMIT, RECORDED_TEXT, stamp)

    def test_a_missing_tag_is_the_only_problem_reported(self) -> None:
        assert freeze_problems(None, RECORDED_TEXT, None) == ["the pre-registration tag does not exist"]

    def test_a_missing_recorded_commit_is_the_only_problem_reported(self) -> None:
        assert freeze_problems(COMMIT, None, None) == ["freeze/prereg-commit.txt is missing"]

    def test_a_recorded_commit_other_than_the_one_the_tag_peels_to_is_a_problem(self) -> None:
        text = OTHER_COMMIT + "\n"
        (problem,) = freeze_problems(COMMIT, text, _stamp(sha256_hex(text)))
        assert OTHER_COMMIT in problem
        assert COMMIT in problem

    def test_trailing_whitespace_of_the_recorded_commit_is_ignored(self) -> None:
        text = f"  {COMMIT}\n\n"
        assert freeze_problems(COMMIT, text, _stamp(sha256_hex(text))) == []

    def test_a_missing_stamp_is_a_problem(self) -> None:
        (problem,) = freeze_problems(COMMIT, RECORDED_TEXT, None)
        assert "prereg-commit.tsr" in problem

    def test_a_stamp_of_another_text_is_a_problem(self) -> None:
        (problem,) = freeze_problems(COMMIT, RECORDED_TEXT, _stamp(sha256_hex("another text")))
        assert sha256_hex("another text") in problem

    def test_a_wrong_commit_and_a_wrong_stamp_are_both_reported(self) -> None:
        text = OTHER_COMMIT + "\n"
        assert len(freeze_problems(COMMIT, text, _stamp(sha256_hex(RECORDED_TEXT)))) == 2


class TestStampedBefore:
    def test_a_stamp_strictly_before_the_moment(self) -> None:
        assert stamped_before(_stamp(""), STAMPED_AT + timedelta(seconds=1))

    def test_a_stamp_at_the_moment_is_not_before(self) -> None:
        assert not stamped_before(_stamp(""), STAMPED_AT)

    def test_a_stamp_after_the_moment_is_not_before(self) -> None:
        assert not stamped_before(_stamp(""), STAMPED_AT - timedelta(seconds=1))
