"""PREREG 13: the machine runs only while a verb has containers to run, and stops whatever the verb's exit."""

from __future__ import annotations

from collections.abc import Generator
from contextlib import contextmanager

from demos1.application.ports import Machine
from demos1.domain.machine import MachineMounts


@contextmanager
def machine_running(machine: Machine, mounts: MachineMounts) -> Generator[None]:
    try:
        machine.start(mounts)
        yield
    finally:
        machine.stop()
