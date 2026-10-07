"""Run window: arithmetic across midnight, clean stop, resume (owner's pacing rule)."""

from __future__ import annotations

from datetime import date, datetime

import pytest

from pilot.adapters.run_directory import RunDirectory
from pilot.application.budgeted_port import BudgetedModelPort
from pilot.application.provenance import log_deviation
from pilot.cli.main import build_parser, main
from pilot.cli.pipeline_commands import _guarded
from pilot.domain.deviations import DeviationEntry
from pilot.domain.model_call import Cli, Effort, ModelPin, ModelProvenance, ModelRequest, ModelResult, TokenUsage
from pilot.domain.run_window import DEFAULT_RUN_WINDOW, InvalidRunWindow, RunWindow, RunWindowClosed

PIN = ModelPin("claude-sonnet-5-5", Cli.CLAUDE)


def at(hour, minute=0):
    return datetime(2026, 10, 12, hour, minute)


def test_the_default_window_is_nine_pm_to_seven_thirty_am():
    assert DEFAULT_RUN_WINDOW == "21:00-07:30"
    parser, _ = build_parser()
    assert parser.parse_args(["run-core", "--run-id", "r"]).run_window == "21:00-07:30"


@pytest.mark.parametrize(
    ("moment", "expected"),
    [(at(21, 0), True), (at(20, 59), False), (at(23, 59), True), (at(0, 0), True), (at(7, 29), True),
     (at(7, 30), False), (at(12, 0), False)],
)
def test_a_window_crossing_midnight(moment, expected):
    assert RunWindow.parse("21:00-07:30").is_open(moment) is expected


@pytest.mark.parametrize(("moment", "expected"), [(at(9), True), (at(8, 59), False), (at(17), False), (at(16, 59), True)])
def test_a_window_inside_one_day(moment, expected):
    assert RunWindow.parse("09:00-17:00").is_open(moment) is expected


def test_always_disables_the_window_and_bad_values_are_refused():
    assert RunWindow.parse("always") is None
    for bad in ("21:00", "25:00-07:00", "10:00-10:00", "x"):
        with pytest.raises(InvalidRunWindow):
            RunWindow.parse(bad)


class Counting:
    def __init__(self):
        self.calls = []

    def call(self, request, baseline):
        self.calls.append(request.call_id)
        return ModelResult({}, TokenUsage(1, 1), None, 1, 1, ModelProvenance.REPORTED)


def request(call_id):
    return ModelRequest(call_id, PIN, Effort.MEDIUM, "i", "{}", "s")


def test_outside_the_window_no_new_call_is_made_but_recorded_calls_are_served(tmp_path):
    inner, now = Counting(), [at(12)]
    port = BudgetedModelPort(inner, RunDirectory(tmp_path), 250, lambda: date(2026, 10, 12), lambda c: c == "old",
                             RunWindow.parse("21:00-07:30"), lambda: now[0])
    port.call(request("old"), None)
    with pytest.raises(RunWindowClosed):
        port.call(request("new"), None)
    assert inner.calls == ["old"] and not (tmp_path / "call_budget.json").exists()
    now[0] = at(22)
    port.call(request("new"), None)
    assert inner.calls == ["old", "new"]


def test_the_window_closing_mid_run_stops_cleanly_and_a_rerun_resumes(tmp_path):
    inner = Counting()
    ticks = iter([at(7, 29), at(7, 30), at(21, 5)])
    port = BudgetedModelPort(inner, RunDirectory(tmp_path), 250, lambda: date(2026, 10, 12), lambda c: c in inner.calls,
                             RunWindow.parse("21:00-07:30"), lambda: next(ticks))
    port.call(request("a"), None)
    with pytest.raises(RunWindowClosed):
        port.call(request("b"), None)
    port.call(request("b"), None)
    assert inner.calls == ["a", "b"]


def test_the_guard_exits_3_with_the_exact_entry(tmp_path):
    store = RunDirectory(tmp_path)

    def stop():
        raise RunWindowClosed("21:00-07:30")

    assert _guarded("run-core", stop, lambda text: log_deviation(store, DeviationEntry.quota_pause(text))) == 3
    assert store.read_text("DEVIATIONS.md").splitlines()[-1] == "- quota-pause | endpoints: none | run window closed"


def test_the_run_window_verb_exits_3_outside_and_0_when_disabled():
    assert main(["run-window", "--run-window", "always"]) == 0
    now = datetime.now().time()
    closed = f"{(now.hour + 2) % 24:02d}:00-{(now.hour + 3) % 24:02d}:00"
    assert main(["run-window", "--run-window", closed]) == 3


def test_the_budget_night_is_the_local_date_the_window_opened_not_the_utc_day():
    window = RunWindow.parse("21:00-07:30")
    assert window.night_of(datetime(2026, 10, 12, 21, 0)) == date(2026, 10, 12)
    assert window.night_of(datetime(2026, 10, 12, 23, 59)) == date(2026, 10, 12)
    assert window.night_of(datetime(2026, 10, 13, 2, 0)) == date(2026, 10, 12)
    assert window.night_of(datetime(2026, 10, 13, 7, 29)) == date(2026, 10, 12)
    assert window.night_of(datetime(2026, 10, 13, 12, 0)) == date(2026, 10, 13)
    assert RunWindow.parse("09:00-17:00").night_of(datetime(2026, 10, 13, 3, 0)) == date(2026, 10, 13)


def test_one_night_spends_one_budget_across_utc_midnight(tmp_path):
    from pilot.domain.call_budget import CallBudgetReached

    window = RunWindow.parse("21:00-07:30")
    before, after = datetime(2026, 10, 12, 23, 0), datetime(2026, 10, 13, 2, 0)
    BudgetedModelPort(Counting(), RunDirectory(tmp_path), 1, lambda: window.night_of(before), lambda c: False).call(request("a"), None)
    late = BudgetedModelPort(Counting(), RunDirectory(tmp_path), 1, lambda: window.night_of(after), lambda c: False)
    with pytest.raises(CallBudgetReached):
        late.call(request("b"), None)
