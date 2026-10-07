import fnmatch
import json
import os
import sys

import pytest

OUTPUT_VARIABLE = "SPEC_PROOF_COVERAGE_OUT"
EXCLUDED_FILE_PATTERNS = ("test_*.py", "*_test.py", "conftest.py")
EXCLUDED_PATH_PARTS = (".venv", "site-packages")
SYNTHETIC_CODE_PREFIX = "<"
LOCALS_INFIX = ".<locals>."
PARENT_DIRECTORY = ".."
CALL_EVENT = "call"

executed = {}
state = {"current": None, "root": ""}


def qualified_name(code):
    return getattr(code, "co_qualname", code.co_name).replace(LOCALS_INFIX, ".")


def project_file(filename):
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


def profiler(frame, event, argument):
    current = state["current"]
    if event != CALL_EVENT or current is None:
        return
    code = frame.f_code
    if code.co_name.startswith(SYNTHETIC_CODE_PREFIX):
        return
    file = project_file(code.co_filename)
    if file is not None:
        current.add((file, qualified_name(code)))


def pytest_configure(config):
    state["root"] = os.path.realpath(str(config.rootpath))


@pytest.hookimpl(hookwrapper=True)
def pytest_runtest_call(item):
    state["current"] = executed.setdefault(item.nodeid, set())
    sys.setprofile(profiler)
    try:
        yield
    finally:
        sys.setprofile(None)
        state["current"] = None


def pytest_sessionfinish(session):
    output = os.environ.get(OUTPUT_VARIABLE)
    if not output:
        return
    report = {node: sorted([file, name] for file, name in pairs) for node, pairs in executed.items()}
    with open(output, "w", encoding="utf-8") as sink:
        json.dump(report, sink)
