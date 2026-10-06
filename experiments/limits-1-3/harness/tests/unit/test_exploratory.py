"""Exploratory windows, the status ledger and the luna pin (ADR 0006)."""

from __future__ import annotations

from datetime import UTC, datetime
from pathlib import Path

import pytest

from pilot.adapters.model_cli import CodexCli
from pilot.adapters.process import ProcessResult
from pilot.adapters.run_directory import RunDirectory
from pilot.application.exploratory_calls import BLOCKED_FAILURE, ExploratoryLedger, WindowedCaller, WindowNotOpen
from pilot.application.model_caller import CallSpec
from pilot.domain.call_type import CallType
from pilot.domain.exploratory import (
    LUNA_WINDOW,
    SECONDARY_WINDOW,
    STREAM_LUNA,
    STREAM_SECONDARY,
    CallStatus,
    Window,
)
from pilot.domain.model_call import Cli, Effort, MissingResult, ModelProvenance, ModelRequest, ModelResult, TokenUsage
from pilot.domain.model_slate import LUNA
from pilot.domain.quota import QuotaExhausted

OPEN, CLOSE = datetime(2026, 10, 10, 9, 1, tzinfo=UTC), datetime(2026, 10, 17, 23, 59, tzinfo=UTC)
WINDOW = Window(OPEN, CLOSE)


def utc(*args):
    return datetime(*args, tzinfo=UTC)


def test_the_luna_window_opens_at_eleven_oh_one_paris_time_and_closes_on_the_17th():
    assert LUNA_WINDOW.opens == utc(2026, 10, 10, 9, 1) and LUNA_WINDOW.closes == utc(2026, 10, 17, 23, 59)
    assert SECONDARY_WINDOW.opens is None and SECONDARY_WINDOW.closes == utc(2026, 10, 24, 23, 59)


@pytest.mark.parametrize(
    ("now", "not_yet", "is_open", "closed"),
    [(utc(2026, 10, 10, 9, 0), True, False, False), (utc(2026, 10, 10, 9, 1), False, True, False),
     (utc(2026, 10, 17, 23, 58), False, True, False), (utc(2026, 10, 17, 23, 59), False, False, True)],
)
def test_window_boundaries(now, not_yet, is_open, closed):
    assert (WINDOW.not_yet_open(now), WINDOW.is_open(now), WINDOW.is_closed(now)) == (not_yet, is_open, closed)


def test_a_window_without_an_opening_is_open_at_once():
    assert SECONDARY_WINDOW.is_open(utc(2026, 1, 1)) and not SECONDARY_WINDOW.not_yet_open(utc(2026, 1, 1))


class Inner:
    def __init__(self, recorded=(), quota_after=None):
        self.recorded, self.quota_after, self.made = set(recorded), quota_after, []

    def is_recorded(self, call_id):
        return call_id in self.recorded

    def call_all(self, stage, specs):
        outcomes = {}
        for spec in specs:
            if spec.call_id in self.recorded:
                outcomes[spec.call_id] = ModelResult({"ok": True}, TokenUsage(1, 1), None, 1, 1, ModelProvenance.REPORTED)
                continue
            if self.quota_after is not None and len(self.made) >= self.quota_after:
                raise QuotaExhausted(stage)
            self.made.append(spec.call_id)
            self.recorded.add(spec.call_id)
            outcomes[spec.call_id] = ModelResult({"ok": True}, TokenUsage(1, 1), None, 1, 1, ModelProvenance.REPORTED)
        return outcomes


def specs(*ids):
    return [CallSpec(call_id, LUNA, CallType.GENERATE, "item", "{}") for call_id in ids]


def windowed(tmp_path, inner, now):
    ledger = ExploratoryLedger(RunDirectory(tmp_path))
    return WindowedCaller(inner, STREAM_LUNA, ledger, WINDOW, lambda: now), ledger


def test_inside_the_window_every_call_is_made_and_marked_complete(tmp_path):
    caller, ledger = windowed(tmp_path, Inner(), utc(2026, 10, 12))
    outcomes = caller.call_all("stage", specs("a", "b"))
    assert set(outcomes) == {"a", "b"} and ledger.statuses() == {"luna": {"a": "complete", "b": "complete"}}


def test_a_quota_stop_leaves_the_calls_not_made_pending_and_reraises(tmp_path):
    inner = Inner(quota_after=1)
    caller, ledger = windowed(tmp_path, inner, utc(2026, 10, 12))
    with pytest.raises(QuotaExhausted):
        caller.call_all("stage", specs("a", "b", "c"))
    assert ledger.statuses()["luna"] == {"a": "complete", "b": "pending", "c": "pending"}


def test_before_the_window_nothing_is_called_or_recorded(tmp_path):
    inner = Inner()
    caller, ledger = windowed(tmp_path, inner, utc(2026, 10, 9))
    with pytest.raises(WindowNotOpen):
        caller.call_all("stage", specs("a"))
    assert inner.made == [] and ledger.statuses() == {}


def test_after_the_close_recorded_calls_are_served_and_the_others_are_blocked_as_missing(tmp_path):
    inner = Inner(recorded={"a"})
    caller, ledger = windowed(tmp_path, inner, utc(2026, 10, 18))
    outcomes = caller.call_all("stage", specs("a", "b"))
    assert inner.made == [] and isinstance(outcomes["a"], ModelResult)
    assert outcomes["b"] == MissingResult((BLOCKED_FAILURE,), ())
    assert ledger.statuses() == {"luna": {"a": "complete", "b": "blocked"}}


def test_two_streams_share_one_status_file_and_a_later_mark_replaces_an_earlier_one(tmp_path):
    ledger = ExploratoryLedger(RunDirectory(tmp_path))
    ledger.mark(STREAM_LUNA, {"x": CallStatus.PENDING})
    ledger.mark(STREAM_SECONDARY, {"y": CallStatus.BLOCKED})
    ledger.mark(STREAM_LUNA, {"x": CallStatus.COMPLETE})
    assert ledger.statuses() == {"luna": {"x": "complete"}, "secondary": {"y": "blocked"}}
    assert set(CallStatus) == {CallStatus.COMPLETE, CallStatus.BLOCKED, CallStatus.PENDING}


def test_the_status_never_reaches_the_deviation_log(tmp_path):
    caller, _ = windowed(tmp_path, Inner(), utc(2026, 10, 18))
    caller.call_all("stage", specs("a"))
    assert not (tmp_path / "DEVIATIONS.md").exists()


class RecordingProcess:
    def __init__(self):
        self.argv = None

    def run(self, argv, cwd, **_):
        self.argv = list(argv)
        return ProcessResult(1, "", "")


def test_the_luna_calls_pass_the_requested_slug_to_codex_and_report_requested_only_provenance(tmp_path):
    cli = CodexCli(RecordingProcess(), tmp_path, cli_version="0.155.1")
    argv = cli.argv(ModelRequest("c", LUNA, Effort.HIGH, "item", "{}", "system"), tmp_path, tmp_path / "schema.json")
    assert argv[argv.index("-m") + 1] == "gpt-6-luna" and argv[:2] == ["codex", "exec"]
    assert LUNA.cli is Cli.CODEX and LUNA.provenance is ModelProvenance.REQUESTED_ONLY
