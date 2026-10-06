from __future__ import annotations

import threading
import time

import pytest

from pilot.application.model_caller import CallSpec, MissingCanary, ModelCaller
from pilot.application.model_router import RoutingModelPort
from pilot.domain.call_type import CallType
from pilot.domain.model_call import (
    CanaryBaseline,
    Cli,
    MissingResult,
    ModelProvenance,
    ModelResult,
    TokenUsage,
)
from pilot.domain.model_slate import SONNET, TERRA
from pilot.domain.quota import QuotaExhausted, looks_like_quota_exhausted

OK = ModelResult({"ok": True}, TokenUsage(1, 1), None, 1, 1, ModelProvenance.REPORTED)
BASELINES = {(SONNET.key, CallType.PROBE): CanaryBaseline(100, 2), (TERRA.key, CallType.PROBE): CanaryBaseline(900, None)}


def spec(n, pin=SONNET, call_type=CallType.PROBE):
    return CallSpec(f"call-{n}", pin, call_type, "item", "{}")


class RecordingPort:
    def __init__(self, behaviour=lambda request: OK):
        self.requests, self.behaviour, self.lock = [], behaviour, threading.Lock()

    def call(self, request, baseline):
        with self.lock:
            self.requests.append((request, baseline))
        return self.behaviour(request)


def caller(port, logs=None, baselines=BASELINES, workers=4):
    return ModelCaller(port, baselines, "system", (logs if logs is not None else []).append, workers)


def test_each_call_gets_the_baseline_of_its_model_and_call_type_and_the_call_type_effort():
    port = RecordingPort()
    caller(port).call_all("s", [spec(1), spec(2, TERRA)])
    by_id = {r.call_id: (r, b) for r, b in port.requests}
    assert by_id["call-1"][1] == CanaryBaseline(100, 2) and by_id["call-2"][1] == CanaryBaseline(900, None)
    assert by_id["call-1"][0].effort.value == "medium" and by_id["call-1"][0].system_prompt == "system"


def test_a_call_type_without_a_canary_baseline_aborts_before_any_call():
    port = RecordingPort()
    with pytest.raises(MissingCanary, match="validity"):
        caller(port).call_all("s", [spec(1), spec(2, SONNET, CallType.VALIDITY)])
    assert port.requests == []


def test_calls_without_baseline_are_allowed_when_the_caller_does_not_require_one():
    port = RecordingPort()
    ModelCaller(port, {}, "s", lambda m: None, 4, require_baseline=False).call_all("s", [spec(1)])
    assert port.requests[0][1] is None


def test_outcomes_are_keyed_by_call_id_and_progress_is_logged_with_counts():
    logs = []
    outcomes = caller(RecordingPort(), logs).call_all("probe", [spec(n) for n in range(5)])
    assert set(outcomes) == {f"call-{n}" for n in range(5)}
    assert "[probe] 5/5 calls done" in logs


def test_missing_results_are_returned_not_raised():
    missing = MissingResult(("x",), ())
    outcomes = caller(RecordingPort(lambda request: missing)).call_all("s", [spec(1)])
    assert outcomes["call-1"] is missing


def test_a_quota_failure_stops_the_stage_cleanly_without_marking_calls_missing():
    logs = []

    def behaviour(request):
        if request.call_id == "call-3":
            raise QuotaExhausted("limit")
        time.sleep(0.01)
        return OK

    port = RecordingPort(behaviour)
    with pytest.raises(QuotaExhausted):
        caller(port, logs, workers=1).call_all("probe", [spec(n) for n in range(20)])
    assert len(port.requests) < 20
    assert any("rerun to resume" in line for line in logs)


def test_at_most_four_calls_are_in_flight_through_the_router():
    state = {"now": 0, "peak": 0}
    lock = threading.Lock()

    class Slow:
        def call(self, request, baseline):
            with lock:
                state["now"] += 1
                state["peak"] = max(state["peak"], state["now"])
            time.sleep(0.02)
            with lock:
                state["now"] -= 1
            return OK

    router = RoutingModelPort({Cli.CLAUDE: Slow(), Cli.CODEX: Slow()})
    caller(router, workers=16).call_all("s", [spec(n, SONNET if n % 2 else TERRA) for n in range(24)])
    assert state["peak"] <= 4


@pytest.mark.parametrize(
    "text",
    ["HTTP 429 Too Many Requests", "You've hit your limit", "Rate limit exceeded", "usage limit reached", "insufficient_quota"],
)
def test_quota_patterns_are_recognised(text):
    assert looks_like_quota_exhausted("", text)


def test_ordinary_errors_are_not_quota_failures():
    assert not looks_like_quota_exhausted("claude exited 1", "schema mismatch at 4290")
