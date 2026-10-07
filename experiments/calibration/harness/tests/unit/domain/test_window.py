from __future__ import annotations

from datetime import datetime, time

import pytest

from calibration.domain.constants import ALWAYS_OPEN, DEFAULT_RUN_WINDOW
from calibration.domain.window import InvalidRunWindow, RunWindow, is_window_open


def _at(hour: int, minute: int = 0) -> datetime:
    return datetime(2026, 10, 7, hour, minute)


def _default_window() -> RunWindow:
    window = RunWindow.parse(DEFAULT_RUN_WINDOW)
    assert window is not None
    return window


class TestParsing:
    def test_the_default_window_is_nine_pm_to_half_past_seven(self) -> None:
        assert _default_window() == RunWindow(time(21, 0), time(7, 30))

    def test_a_single_digit_hour_is_accepted(self) -> None:
        assert RunWindow.parse("6:15-9:45") == RunWindow(time(6, 15), time(9, 45))

    def test_surrounding_blanks_are_ignored(self) -> None:
        assert RunWindow.parse("  22:00-06:00 ") == RunWindow(time(22, 0), time(6, 0))

    def test_always_disables_the_window(self) -> None:
        assert RunWindow.parse(ALWAYS_OPEN) is None

    @pytest.mark.parametrize("text", ["", "21:00", "21:00-", "21-07", "ab:cd-ef:gh", "25:00-07:00", "21:60-07:00"])
    def test_a_malformed_window_is_refused(self, text: str) -> None:
        with pytest.raises(InvalidRunWindow):
            RunWindow.parse(text)

    def test_equal_bounds_are_refused(self) -> None:
        with pytest.raises(InvalidRunWindow):
            RunWindow.parse("08:00-08:00")


class TestCrossMidnightWindow:
    @pytest.mark.parametrize("now", [_at(21), _at(23, 59), _at(0), _at(3, 15), _at(7, 29)])
    def test_the_night_is_open(self, now: datetime) -> None:
        assert _default_window().is_open(now)

    @pytest.mark.parametrize("now", [_at(7, 30), _at(12), _at(20, 59)])
    def test_the_day_is_closed(self, now: datetime) -> None:
        assert not _default_window().is_open(now)

    def test_the_start_is_inclusive_and_the_end_exclusive(self) -> None:
        window = _default_window()
        assert window.is_open(_at(21, 0))
        assert not window.is_open(_at(7, 30))


class TestSameDayWindow:
    WINDOW = RunWindow(time(9, 0), time(17, 0))

    def test_inside(self) -> None:
        assert self.WINDOW.is_open(_at(9)) and self.WINDOW.is_open(_at(16, 59))

    def test_outside(self) -> None:
        assert not self.WINDOW.is_open(_at(8, 59)) and not self.WINDOW.is_open(_at(17))


class TestAlways:
    def test_no_window_is_always_open(self) -> None:
        assert all(is_window_open(None, _at(hour)) for hour in range(24))

    def test_a_window_decides_when_there_is_one(self) -> None:
        assert is_window_open(_default_window(), _at(22))
        assert not is_window_open(_default_window(), _at(12))
