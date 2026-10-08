"""Pacing of the single-shot host executor; kept out of the trunk and branch engines like its port."""

from __future__ import annotations

import json

from specbench.application.host_executor import HostExecutor
from specbench.application.paced_calls import CallGate
from specbench.application.ports import CallStore
from specbench.domain.arms import Host
from specbench.domain.canonical import stable_json
from specbench.domain.sessions import CallRecord, HostRequest, HostResponse

HOST_CALL_PREFIX = "host-"


class PacedHostExecutor:
    """Decorator over a `HostExecutor`; a single-shot host call is a model call and obeys the same pacing."""

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
        admission = self._gate.admit()
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
        self._gate.charge(admission)
        return response
