"""Decorators that make every model call paced, frozen-model-checked, archived and resumable."""

from __future__ import annotations

import json

from specbench.application.ports import AgentHarness, CallStore, Clock, HostExecutor, LedgerStore
from specbench.domain.arms import FROZEN_MODEL, Harness, Host
from specbench.domain.canonical import stable_json
from specbench.domain.pacing import PacingPolicy, PauseReason, RunPaused, looks_like_quota_exhausted
from specbench.domain.sessions import CallRecord, HostRequest, HostResponse, SessionRequest, SessionResult

HOST_CALL_PREFIX = "host-"


class ModelNotFrozen(ValueError):
    """A call asked for a model other than the harness's frozen model."""


class CallGate:
    """Admission and accounting of the new calls of one harness: the nightly window, the budget, the ledger."""

    def __init__(self, harness: Harness, policy: PacingPolicy, ledgers: LedgerStore, clock: Clock) -> None:
        self._harness = harness
        self._policy = policy
        self._ledgers = ledgers
        self._clock = clock

    def admit(self) -> None:
        self._policy.admit(self._clock.now(), self._harness, self._ledgers.load(), resumed=False)

    def charge(self) -> None:
        now = self._clock.now()
        self._ledgers.save(self._ledgers.load().with_call(self._policy.night_of(now), self._harness))

    def require_frozen(self, model: str) -> None:
        if model != FROZEN_MODEL[self._harness]:
            raise ModelNotFrozen(f"{self._harness.value} runs {FROZEN_MODEL[self._harness]}, not {model}")


class PacedAgentHarness:
    """Decorator over an `AgentHarness`: a recorded call is replayed for free, a new one is admitted, run and archived."""

    def __init__(self, inner: AgentHarness, gate: CallGate, calls: CallStore) -> None:
        self._inner = inner
        self._gate = gate
        self._calls = calls

    @property
    def harness(self) -> Harness:
        return self._inner.harness

    def run_session(self, request: SessionRequest) -> SessionResult:
        self._gate.require_frozen(request.model)
        call_id = request.call_id
        if self._calls.exists(call_id):
            recorded = self._calls.read(call_id)
            return SessionResult(
                recorded.session_id, recorded.stdout, recorded.exit_code, recorded.events, recorded.model_reported
            )
        self._gate.admit()
        result = self._inner.run_session(request)
        if result.exit_code != 0 and looks_like_quota_exhausted(result.stderr, result.final_message):
            raise RunPaused(PauseReason.QUOTA_EXHAUSTED, request.harness.value)
        self._calls.write(
            CallRecord(
                call_id=call_id,
                argv=(request.harness.value, request.model, request.kind.value),
                stdin=request.prompt,
                stdout=result.final_message,
                events=result.events,
                exit_code=result.exit_code,
                session_id=result.session_id,
                model_reported=result.model_reported,
            )
        )
        self._gate.charge()
        return result


class PacedHostExecutor:
    """Decorator over a `HostExecutor`; a gate-host session is a model call and obeys the same pacing."""

    def __init__(self, inner: HostExecutor, host: Host, gate: CallGate, calls: CallStore) -> None:
        self._inner = inner
        self._host = host
        self._gate = gate
        self._calls = calls

    def execute(self, request: HostRequest) -> HostResponse:
        call_id = HOST_CALL_PREFIX + request.request_id
        if self._calls.exists(call_id):
            recorded = self._calls.read(call_id)
            return HostResponse(request.request_id, self._host, json.loads(recorded.stdout))
        self._gate.admit()
        response = self._inner.execute(request)
        record = CallRecord(
            call_id,
            (request.kind.value, self._host.value),
            request.prompt,
            stable_json(response.payload),
            (),
            0,
            call_id,
            None,
        )
        self._calls.write(record)
        self._gate.charge()
        return response
