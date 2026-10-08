"""Decorators that make every model call paced, frozen-model-checked, archived and resumable."""

from __future__ import annotations

from specbench.application.ports import (
    AgentHarness,
    CallStore,
    Clock,
    ConstructionModel,
    FailureLog,
    LedgerStore,
)
from specbench.domain.arms import FROZEN_MODEL, Harness
from specbench.domain.call_ledger import Admission
from specbench.domain.construction import (
    ModelCallFailed,
    ModelRequest,
    ModelResponse,
    QuotaExhausted,
    response_from_stdout,
)
from specbench.domain.pacing import PacingPolicy, PauseReason, RunPaused, looks_like_quota_exhausted
from specbench.domain.sessions import CallRecord, SessionRequest, SessionResult


class ModelNotFrozen(ValueError):
    """A call asked for a model other than the harness's frozen model."""


class CallGate:
    """Admission and accounting of the new calls of one harness: the nightly window, the budget, the ledger."""

    def __init__(self, harness: Harness, policy: PacingPolicy, ledgers: LedgerStore, clock: Clock) -> None:
        self._harness = harness
        self._policy = policy
        self._ledgers = ledgers
        self._clock = clock

    def admit(self) -> Admission:
        """Admit one new call; its charge lands on the night it is admitted, whenever it completes."""
        now = self._clock.now()
        self._policy.admit(now, self._harness, self._ledgers.load(), resumed=False)
        return Admission(self._policy.night_of(now))

    def require_open(self) -> None:
        self._policy.require_open(self._clock.now())

    def charge(self, admission: Admission) -> None:
        self._ledgers.save(self._ledgers.load().with_call(admission.night, self._harness))

    def admit_reserved(self, call_id: str, size: int) -> Admission:
        """Reserve `size` executions for a call on tonight's share; the call's own reservation is reused on a rerun."""
        now = self._clock.now()
        ledger = self._ledgers.load()
        self._policy.admit_reserved(now, self._harness, ledger, call_id, size)
        night = self._policy.night_of(now)
        self._ledgers.save(ledger.with_reservation(night, self._harness, call_id, size))
        return Admission(night, call_id)

    def settle(self, admission: Admission, actual: int) -> None:
        """Charge the executions a reserved call spent and release its reservation, on the night it was admitted."""
        if admission.call_id is None:
            raise ValueError("only a reserved admission settles")
        self._ledgers.save(self._ledgers.load().settled(admission.night, self._harness, admission.call_id, actual))

    def pending(self) -> tuple[Admission, ...]:
        """The reserved calls that were admitted and never settled, oldest night first."""
        return self._ledgers.load().pending(self._harness)

    def holds_reservation(self, call_id: str) -> bool:
        return self._ledgers.load().is_reserved(self._policy.night_of(self._clock.now()), self._harness, call_id)

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
        admission = self._gate.admit()
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
        self._gate.charge(admission)
        return result


class PacedConstructionModel:
    """Decorator over a `ConstructionModel`: a recorded call is replayed for free, a new one is admitted, run and archived.

    A construction call counts against the same per-night, per-CLI Claude budget as every arm's call. A failed call is
    charged, noted in the failure log and never archived as a result, so a rerun makes it again.
    """

    def __init__(self, inner: ConstructionModel, gate: CallGate, calls: CallStore, failures: FailureLog) -> None:
        self._inner = inner
        self._gate = gate
        self._calls = calls
        self._failures = failures

    def call(self, request: ModelRequest) -> ModelResponse:
        call_id = request.call_id
        if self._calls.exists(call_id):
            return response_from_stdout(request, self._calls.read(call_id).stdout)
        admission = self._gate.admit()
        try:
            response = self._inner.call(request)
        except QuotaExhausted as error:
            raise RunPaused(PauseReason.QUOTA_EXHAUSTED, request.role.value) from error
        except ModelCallFailed as error:
            self._failures.append(call_id, request.key, str(error))
            self._gate.charge(admission)
            raise
        self._calls.write(
            CallRecord(
                call_id=call_id,
                argv=(request.role.value, request.model, request.key),
                stdin=request.prompt,
                stdout=response.stdout,
                events=(),
                exit_code=0,
                session_id=call_id,
                model_reported=",".join(response.reported_models),
            )
        )
        self._gate.charge(admission)
        return response
