import fnmatch
import json
import os
import sys

import pytest

OUTPUT_VARIABLE = "SPEC_PROOF_COVERAGE_OUT"
TRACER_VARIABLE = "SPEC_PROOF_COVERAGE_TRACER"
TRACER_SETTRACE = "settrace"
EXCLUDED_FILE_PATTERNS = ("test_*.py", "*_test.py", "conftest.py")
EXCLUDED_PATH_PARTS = (".venv", "site-packages")
SYNTHETIC_CODE_PREFIX = "<"
LOCALS_INFIX = ".<locals>."
PARENT_DIRECTORY = ".."
CALL_EVENT = "call"
LINE_EVENT = "line"
UNITS_KEY = "units"
LINES_KEY = "lines"
MONITORING_TOOL_NAME = "spec-proof-coverage"
MONITORING_TOOL_IDS = range(6)

executed = {}
state = {"current": None, "root": "", "files": {}}


def qualified_name(code):
    return getattr(code, "co_qualname", code.co_name).replace(LOCALS_INFIX, ".")


def project_file(filename):
    files = state["files"]
    if filename not in files:
        files[filename] = resolve_project_file(filename)
    return files[filename]


def resolve_project_file(filename):
    if not filename or filename.startswith(SYNTHETIC_CODE_PREFIX):
        return None
    try:
        relative = os.path.relpath(os.path.realpath(filename), state["root"])
    except ValueError:
        return None
    parts = relative.replace(os.sep, "/").split("/")
    if parts[0] == PARENT_DIRECTORY:
        return None
    if any(part in EXCLUDED_PATH_PARTS for part in parts):
        return None
    if any(fnmatch.fnmatch(parts[-1], pattern) for pattern in EXCLUDED_FILE_PATTERNS):
        return None
    return "/".join(parts)


def record_unit(code):
    current = state["current"]
    if current is None or code.co_name.startswith(SYNTHETIC_CODE_PREFIX):
        return
    file = project_file(code.co_filename)
    if file is not None:
        current[UNITS_KEY].add((file, qualified_name(code)))


def record_line(code, line):
    current = state["current"]
    if current is None or line is None:
        return
    file = project_file(code.co_filename)
    if file is not None:
        current[LINES_KEY].add((file, line))


class SetTraceTracer:
    """Records lines through sys.settrace, on interpreters without sys.monitoring."""

    def start(self):
        sys.settrace(self.global_trace)

    def stop(self):
        sys.settrace(None)

    def global_trace(self, frame, event, argument):
        if event != CALL_EVENT:
            return None
        code = frame.f_code
        record_unit(code)
        if project_file(code.co_filename) is None:
            return None
        return self.local_trace

    def local_trace(self, frame, event, argument):
        if event == LINE_EVENT:
            record_line(frame.f_code, frame.f_lineno)
        return self.local_trace


class MonitoringTracer:
    """Records lines through sys.monitoring: each code location reports once per
    test, and every location is re-armed when the next test starts."""

    def __init__(self, tool):
        self.tool = tool
        monitoring = sys.monitoring
        monitoring.use_tool_id(tool, MONITORING_TOOL_NAME)
        monitoring.register_callback(tool, monitoring.events.PY_START, self.on_start)
        monitoring.register_callback(tool, monitoring.events.LINE, self.on_line)

    def start(self):
        monitoring = sys.monitoring
        monitoring.restart_events()
        monitoring.set_events(self.tool, monitoring.events.PY_START | monitoring.events.LINE)

    def stop(self):
        sys.monitoring.set_events(self.tool, sys.monitoring.events.NO_EVENTS)

    def on_start(self, code, offset):
        record_unit(code)
        return sys.monitoring.DISABLE

    def on_line(self, code, line):
        record_line(code, line)
        return sys.monitoring.DISABLE


def free_monitoring_tool():
    for tool in MONITORING_TOOL_IDS:
        if sys.monitoring.get_tool(tool) is None:
            return tool
    return None


def new_tracer():
    if os.environ.get(TRACER_VARIABLE) == TRACER_SETTRACE or not hasattr(sys, "monitoring"):
        return SetTraceTracer()
    tool = free_monitoring_tool()
    if tool is None:
        return SetTraceTracer()
    return MonitoringTracer(tool)


def pytest_configure(config):
    state["root"] = os.path.realpath(str(config.rootpath))
    state["tracer"] = new_tracer()


@pytest.hookimpl(hookwrapper=True)
def pytest_runtest_protocol(item, nextitem):
    state["current"] = executed.setdefault(item.nodeid, {UNITS_KEY: set(), LINES_KEY: set()})
    tracer = state["tracer"]
    tracer.start()
    try:
        yield
    finally:
        tracer.stop()
        state["current"] = None


def pytest_sessionfinish(session):
    output = os.environ.get(OUTPUT_VARIABLE)
    if not output:
        return
    report = {
        node: {
            UNITS_KEY: sorted([file, name] for file, name in recorded[UNITS_KEY]),
            LINES_KEY: sorted([file, line] for file, line in recorded[LINES_KEY]),
        }
        for node, recorded in executed.items()
    }
    with open(output, "w", encoding="utf-8") as sink:
        json.dump(report, sink)
