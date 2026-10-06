from __future__ import annotations

import threading
import time
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

from pilot.adapters.process import LOCAL_SUBPROCESS_GATE, GatedProcessRunner, ProcessResult
from pilot.domain.constants import LOCAL_SUBPROCESS_LIMIT, MAX_CONCURRENT_PYTEST
from pilot.domain.pipeline_constants import MAX_WITNESS_WORKERS
from pilot.domain.analysis_constants import ORACLE_PARALLELISM


class CountingRunner:
    def __init__(self) -> None:
        self.in_flight = 0
        self.peak = 0
        self._lock = threading.Lock()

    def run(self, argv, cwd, **_):
        with self._lock:
            self.in_flight += 1
            self.peak = max(self.peak, self.in_flight)
        time.sleep(0.01)
        with self._lock:
            self.in_flight -= 1
        return ProcessResult(0, "", "")


def test_the_three_local_concurrency_constants_share_one_limit_of_two():
    assert LOCAL_SUBPROCESS_LIMIT == 2
    assert MAX_CONCURRENT_PYTEST == MAX_WITNESS_WORKERS == ORACLE_PARALLELISM == LOCAL_SUBPROCESS_LIMIT


def test_no_more_than_two_subprocesses_run_at_once_whatever_the_fan_out():
    inner = CountingRunner()
    gated = GatedProcessRunner(inner)
    with ThreadPoolExecutor(max_workers=16) as pool:
        list(pool.map(lambda _: gated.run(["x"], Path(".")), range(64)))
    assert 1 <= inner.peak <= LOCAL_SUBPROCESS_LIMIT


def test_two_runners_share_the_process_wide_gate():
    inner = CountingRunner()
    first, second = GatedProcessRunner(inner), GatedProcessRunner(inner)
    with ThreadPoolExecutor(max_workers=16) as pool:
        list(pool.map(lambda n: (first if n % 2 else second).run(["x"], Path(".")), range(64)))
    assert inner.peak <= LOCAL_SUBPROCESS_LIMIT
    assert GatedProcessRunner(inner)._gate is LOCAL_SUBPROCESS_GATE


def test_the_gate_is_released_when_the_inner_runner_raises():
    class Failing:
        def run(self, argv, cwd, **_):
            raise RuntimeError("boom")

    gated = GatedProcessRunner(Failing(), threading.BoundedSemaphore(1))
    for _ in range(3):
        try:
            gated.run(["x"], Path("."))
        except RuntimeError:
            pass
    assert gated._gate.acquire(blocking=False)
