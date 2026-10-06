"""The daily call budget: exit 3, the exact pause entry, resume the next UTC day (ADR 0006)."""

from __future__ import annotations

import threading
from concurrent.futures import ThreadPoolExecutor
from datetime import date

import pytest

from pilot.adapters.run_directory import RunDirectory
from pilot.application.budgeted_port import BudgetedModelPort
from pilot.application.model_caller import CallSpec, ModelCaller
from pilot.application.provenance import log_deviation
from pilot.cli.main import build_parser
from pilot.cli.pipeline_commands import _guarded
from pilot.domain.call_budget import (
    CALL_BUDGET_PAUSE_TEXT,
    DEFAULT_CALL_BUDGET,
    CallBudgetReached,
    calls_on,
    with_call,
)
from pilot.domain.call_type import CallType
from pilot.domain.deviations import DeviationEntry, DeviationKind, parse_deviations
from pilot.domain.model_call import CanaryBaseline, Cli, Effort, ModelProvenance, ModelPin, ModelRequest, ModelResult, TokenUsage
from pilot.domain.pipeline_constants import EXIT_QUOTA_EXHAUSTED
from pilot.domain.quota import QuotaExhausted

CLAUDE = ModelPin("claude-sonnet-5-5", Cli.CLAUDE)
CODEX = ModelPin("gpt-6-luna", Cli.CODEX)
DAY_ONE, DAY_TWO = date(2026, 10, 10), date(2026, 10, 11)


class Counting:
    def __init__(self):
        self.calls = []
        self.lock = threading.Lock()

    def call(self, request, baseline):
        with self.lock:
            self.calls.append(request.call_id)
        return ModelResult({"ok": True}, TokenUsage(1, 1), None, 1, 1, ModelProvenance.REPORTED)


def request(call_id, pin=CLAUDE):
    return ModelRequest(call_id, pin, Effort.MEDIUM, "item", "{}", "system")


def port(tmp_path, budget, today, recorded=lambda _id: False, inner=None):
    return BudgetedModelPort(inner or Counting(), RunDirectory(tmp_path), budget, lambda: today[0], recorded)


def test_the_default_budget_is_250_and_the_option_defaults_to_it():
    assert DEFAULT_CALL_BUDGET == 250
    parser, _ = build_parser()
    assert parser.parse_args(["run-core", "--run-id", "r"]).call_budget == 250
    assert parser.parse_args(["run-blindspot", "--run-id", "r", "--call-budget", "7"]).call_budget == 7
    assert all(parser.parse_args([verb, "--run-id", "r"]).call_budget == 250 for verb in ("relabel", "run-secondary", "run-luna"))


def test_the_budget_stops_the_call_after_n_new_calls_and_the_ledger_is_persisted(tmp_path):
    inner = Counting()
    budgeted = port(tmp_path, 2, [DAY_ONE], inner=inner)
    budgeted.call(request("a"), None)
    budgeted.call(request("b"), None)
    with pytest.raises(CallBudgetReached):
        budgeted.call(request("c"), None)
    assert inner.calls == ["a", "b"]
    assert RunDirectory(tmp_path).read_json("call_budget.json") == {"2026-10-10": {"claude": 2}}


def test_recorded_calls_never_count(tmp_path):
    inner = Counting()
    budgeted = port(tmp_path, 1, [DAY_ONE], recorded=lambda call_id: call_id.startswith("old"), inner=inner)
    for call_id in ("old-1", "old-2", "old-3", "new-1"):
        budgeted.call(request(call_id), None)
    assert inner.calls == ["old-1", "old-2", "old-3", "new-1"]
    with pytest.raises(CallBudgetReached):
        budgeted.call(request("new-2"), None)


def test_each_cli_has_its_own_budget(tmp_path):
    budgeted = port(tmp_path, 1, [DAY_ONE])
    budgeted.call(request("claude-call"), None)
    budgeted.call(request("codex-call", CODEX), None)
    with pytest.raises(CallBudgetReached):
        budgeted.call(request("claude-again"), None)
    with pytest.raises(CallBudgetReached):
        budgeted.call(request("codex-again", CODEX), None)


def test_a_rerun_on_the_next_utc_day_resumes(tmp_path):
    today = [DAY_ONE]
    budgeted = port(tmp_path, 1, today)
    budgeted.call(request("a"), None)
    with pytest.raises(CallBudgetReached):
        budgeted.call(request("b"), None)
    today[0] = DAY_TWO
    budgeted.call(request("b"), None)
    assert RunDirectory(tmp_path).read_json("call_budget.json") == {"2026-10-10": {"claude": 1}, "2026-10-11": {"claude": 1}}


def test_a_new_process_reads_the_ledger_of_the_earlier_one(tmp_path):
    port(tmp_path, 2, [DAY_ONE]).call(request("a"), None)
    second = port(tmp_path, 2, [DAY_ONE])
    second.call(request("b"), None)
    with pytest.raises(CallBudgetReached):
        second.call(request("c"), None)


def test_concurrent_calls_never_exceed_the_budget(tmp_path):
    inner = Counting()
    budgeted = port(tmp_path, 5, [DAY_ONE], inner=inner)

    def attempt(n):
        try:
            budgeted.call(request(f"call-{n}"), None)
        except CallBudgetReached:
            pass

    with ThreadPoolExecutor(max_workers=8) as pool:
        list(pool.map(attempt, range(40)))
    assert len(inner.calls) == 5


def test_the_caller_stops_cleanly_with_the_budget_exception_and_keeps_what_it_finished(tmp_path):
    inner = Counting()
    budgeted = port(tmp_path, 2, [DAY_ONE], inner=inner)
    caller = ModelCaller(budgeted, {(CLAUDE.key, CallType.PROBE): CanaryBaseline(1, 1)}, "system", lambda _m: None, 1)
    specs = [CallSpec(f"probe-{n}", CLAUDE, CallType.PROBE, "item", "{}") for n in range(5)]
    with pytest.raises(CallBudgetReached):
        caller.call_all("probe", specs)
    assert inner.calls == ["probe-0", "probe-1"] and issubclass(CallBudgetReached, QuotaExhausted)


def test_the_guard_exits_3_and_logs_the_exact_pause_entry(tmp_path, capsys):
    store = RunDirectory(tmp_path)

    def stop():
        raise CallBudgetReached("claude 2026-10-10")

    code = _guarded("run-core", stop, lambda text: log_deviation(store, DeviationEntry.quota_pause(text)))
    assert code == EXIT_QUOTA_EXHAUSTED == 3
    assert CALL_BUDGET_PAUSE_TEXT == "call budget reached"
    assert store.read_text("DEVIATIONS.md").splitlines()[-1] == "- quota-pause | endpoints: none | call budget reached"
    assert [e.kind for e in parse_deviations(store.read_text("DEVIATIONS.md"))] == [DeviationKind.QUOTA_PAUSE]


def test_a_plain_quota_stop_keeps_its_own_entry_text(tmp_path):
    store = RunDirectory(tmp_path)

    def stop():
        raise QuotaExhausted("generate")

    assert _guarded("run-core", stop, lambda text: log_deviation(store, DeviationEntry.quota_pause(text))) == 3
    assert "call budget reached" not in store.read_text("DEVIATIONS.md")


def test_the_ledger_helpers_count_per_day_and_cli():
    ledger = with_call(with_call({}, DAY_ONE, Cli.CLAUDE), DAY_ONE, Cli.CLAUDE)
    assert calls_on(ledger, DAY_ONE, Cli.CLAUDE) == 2 and calls_on(ledger, DAY_ONE, Cli.CODEX) == 0
    assert calls_on(ledger, DAY_TWO, Cli.CLAUDE) == 0
