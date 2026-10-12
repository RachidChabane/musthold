import fnmatch
import json
import os
import sys
import threading

import pytest

OUTPUT_VARIABLE = "SPEC_PROOF_COVERAGE_OUT"
TRACER_VARIABLE = "SPEC_PROOF_COVERAGE_TRACER"
TRACER_SETTRACE = "settrace"
EXCLUDED_FILE_PATTERNS = ("test_*.py", "*_test.py", "conftest.py")
EXCLUDED_PATH_PARTS = (".venv", "site-packages")
SYNTHETIC_CODE_PREFIX = "<"
MODULE_CODE_NAME = "<module>"
LOCALS_INFIX = ".<locals>."
PARENT_DIRECTORY = ".."
CALL_EVENT = "call"
LINE_EVENT = "line"
RETURN_EVENT = "return"
UNITS_KEY = "units"
LINES_KEY = "lines"
OWNED_KEY = "owned"
SETUP_PHASE = "setup"
CALL_PHASE = "call"
TEARDOWN_PHASE = "teardown"
OWNING_PHASES = (SETUP_PHASE, CALL_PHASE)
FUNCTION_SCOPE = "function"
MONITORING_TOOL_NAME = "spec-proof-coverage"
MONITORING_TOOL_IDS = range(6)

executed = {}
state = {
    "current": None,
    "root": "",
    "files": {},
    "thread": None,
    "phase": None,
    "fixture_scopes": [],
    "imports": [],
}


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


def is_module_code(code):
    return code.co_name == MODULE_CODE_NAME


def on_protocol_thread():
    return threading.get_ident() == state["thread"]


def owning():
    """Whether a line running now is the current test's own: it runs on the
    thread of the test's protocol, in its setup or call, outside the setup of a
    fixture of a scope wider than the function, and outside the body of a
    module the protocol started importing."""
    return (
        on_protocol_thread()
        and state["phase"] in OWNING_PHASES
        and all(scope == FUNCTION_SCOPE for scope in state["fixture_scopes"])
        and not state["imports"]
    )


def module_started(code):
    if state["current"] is not None and on_protocol_thread():
        state["imports"].append(code)


def module_ended(code):
    if not on_protocol_thread():
        return
    imports = state["imports"]
    for index in range(len(imports) - 1, -1, -1):
        if imports[index] is code:
            del imports[index:]
            return


def record_unit(code):
    current = state["current"]
    if current is None or code.co_name.startswith(SYNTHETIC_CODE_PREFIX):
        return
    file = project_file(code.co_filename)
    if file is not None:
        current[UNITS_KEY].add((file, qualified_name(code)))


def record_line(code, line):
    """Records a line the current test ran, and reports whether it recorded it
    as one the test owns."""
    current = state["current"]
    if current is None or line is None:
        return False
    file = project_file(code.co_filename)
    if file is None:
        return False
    current[LINES_KEY].add((file, line))
    if not owning():
        return False
    current[OWNED_KEY].add((file, line))
    return True


class SetTraceTracer:
    """Records lines through sys.settrace, on interpreters without
    sys.monitoring: on the protocol's thread alone, following every module body
    to its end, whatever its file."""

    def start(self):
        sys.settrace(self.global_trace)

    def stop(self):
        sys.settrace(None)

    def global_trace(self, frame, event, argument):
        if event != CALL_EVENT:
            return None
        code = frame.f_code
        record_unit(code)
        project = project_file(code.co_filename) is not None
        if is_module_code(code):
            module_started(code)
            if not project:
                frame.f_trace_lines = False
            return self.module_trace
        if not project:
            return None
        return self.local_trace

    def local_trace(self, frame, event, argument):
        if event == LINE_EVENT:
            record_line(frame.f_code, frame.f_lineno)
        return self.local_trace

    def module_trace(self, frame, event, argument):
        if event == LINE_EVENT:
            record_line(frame.f_code, frame.f_lineno)
        elif event == RETURN_EVENT:
            module_ended(frame.f_code)
        return self.module_trace


class MonitoringTracer:
    """Records lines through sys.monitoring: a code location stops reporting
    for the rest of a test once the test owns it, and every location is re-armed
    when the next test starts. The start, return and unwind of a module body
    always report, so that the end of every import is seen."""

    def __init__(self, tool):
        self.tool = tool
        monitoring = sys.monitoring
        events = monitoring.events
        monitoring.use_tool_id(tool, MONITORING_TOOL_NAME)
        monitoring.register_callback(tool, events.PY_START, self.on_start)
        monitoring.register_callback(tool, events.PY_RETURN, self.on_return)
        monitoring.register_callback(tool, events.PY_UNWIND, self.on_unwind)
        monitoring.register_callback(tool, events.LINE, self.on_line)

    def start(self):
        monitoring = sys.monitoring
        events = monitoring.events
        monitoring.restart_events()
        monitoring.set_events(self.tool, events.PY_START | events.PY_RETURN | events.PY_UNWIND | events.LINE)

    def stop(self):
        sys.monitoring.set_events(self.tool, sys.monitoring.events.NO_EVENTS)

    def on_start(self, code, offset):
        record_unit(code)
        if is_module_code(code):
            module_started(code)
            return None
        return sys.monitoring.DISABLE

    def on_return(self, code, offset, value):
        if is_module_code(code):
            module_ended(code)
            return None
        return sys.monitoring.DISABLE

    def on_unwind(self, code, offset, exception):
        if is_module_code(code):
            module_ended(code)

    def on_line(self, code, line):
        if project_file(code.co_filename) is None or record_line(code, line):
            return sys.monitoring.DISABLE
        return None


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
    state["current"] = executed.setdefault(item.nodeid, {UNITS_KEY: set(), LINES_KEY: set(), OWNED_KEY: set()})
    state["thread"] = threading.get_ident()
    state["phase"] = None
    state["fixture_scopes"] = []
    state["imports"] = []
    tracer = state["tracer"]
    tracer.start()
    try:
        yield
    finally:
        tracer.stop()
        state["current"] = None
        state["thread"] = None
        state["phase"] = None


def in_phase(phase):
    state["phase"] = phase
    try:
        yield
    finally:
        state["phase"] = None


@pytest.hookimpl(hookwrapper=True)
def pytest_runtest_setup(item):
    yield from in_phase(SETUP_PHASE)


@pytest.hookimpl(hookwrapper=True)
def pytest_runtest_call(item):
    yield from in_phase(CALL_PHASE)


@pytest.hookimpl(hookwrapper=True)
def pytest_runtest_teardown(item, nextitem):
    yield from in_phase(TEARDOWN_PHASE)


@pytest.hookimpl(hookwrapper=True)
def pytest_fixture_setup(fixturedef, request):
    scopes = state["fixture_scopes"]
    scopes.append(fixturedef.scope)
    try:
        yield
    finally:
        if scopes:
            scopes.pop()


def pytest_sessionfinish(session):
    output = os.environ.get(OUTPUT_VARIABLE)
    if not output:
        return
    report = {
        node: {
            UNITS_KEY: sorted([file, name] for file, name in recorded[UNITS_KEY]),
            LINES_KEY: sorted([file, line] for file, line in recorded[LINES_KEY]),
            OWNED_KEY: sorted([file, line] for file, line in recorded[OWNED_KEY]),
        }
        for node, recorded in executed.items()
    }
    with open(output, "w", encoding="utf-8") as sink:
        json.dump(report, sink)
