"""Runs the Kev stub in the foreground: `run_stub_kev.py -m kev.serve <run> <host> <port>` (the fake `uv` starts it)."""

from __future__ import annotations

import os
import sys

sys.path.insert(0, os.environ["FAKEBIN_TESTS_DIR"])

from support.stub_kev import StubKevServer

RUN_INDEX = 3
HOST_INDEX = 4
PORT_INDEX = 5

if __name__ == "__main__":
    StubKevServer(sys.argv[HOST_INDEX], int(sys.argv[PORT_INDEX]), run=sys.argv[RUN_INDEX]).serve_forever_blocking()
