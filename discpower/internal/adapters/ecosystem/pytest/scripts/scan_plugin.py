import json
import os
import pathlib

import pytest

OUTPUT_VARIABLE = "SPEC_PROOF_SCAN_OUT"
SELECTION_VARIABLE = "SPEC_PROOF_SCAN_SELECTION"
CALL_PHASE = "call"
PASSED = "passed"
FAILED = "failed"
ERRORED = "errored"
SKIPPED = "skipped"
SEVERITY = {PASSED: 0, SKIPPED: 1, FAILED: 2, ERRORED: 3}
USAGE_ERROR = int(pytest.ExitCode.USAGE_ERROR)
CONFTEST_PATH_ATTRIBUTE = "path"

collected = []
outcomes = {}
collect_failures = []


def relative_file(item, root):
    try:
        return item.path.relative_to(root).as_posix()
    except ValueError:
        return item.path.as_posix()


def write_report(exit_code, stopped_before_collection):
    output = os.environ.get(OUTPUT_VARIABLE)
    if not output:
        return
    with open(output, "w", encoding="utf-8") as sink:
        json.dump(
            {
                "collected": collected,
                "outcomes": outcomes,
                "collectFailures": collect_failures,
                "stoppedBeforeCollection": stopped_before_collection,
                "exit": exit_code,
            },
            sink,
        )


def requested_ids():
    selection = os.environ.get(SELECTION_VARIABLE)
    if not selection:
        return None
    with open(selection, encoding="utf-8") as source:
        return set(json.load(source))


@pytest.hookimpl(hookwrapper=True)
def pytest_load_initial_conftests(early_config, parser, args):
    outcome = yield
    if outcome.excinfo is None:
        return
    conftest = getattr(outcome.excinfo[1], CONFTEST_PATH_ATTRIBUTE, None)
    if conftest is None:
        return
    root = pathlib.Path(getattr(early_config.invocation_params, "dir", os.getcwd()))
    try:
        collect_failures.append(pathlib.Path(conftest).resolve().relative_to(root.resolve()).as_posix())
    except ValueError:
        return
    write_report(USAGE_ERROR, True)


def pytest_collectreport(report):
    if report.failed:
        collect_failures.append(report.nodeid)


@pytest.hookimpl(trylast=True)
def pytest_collection_modifyitems(session, config, items):
    requested = requested_ids()
    if requested is None:
        return
    kept = [item for item in items if item.nodeid in requested]
    deselected = [item for item in items if item.nodeid not in requested]
    if deselected:
        config.hook.pytest_deselected(items=deselected)
        items[:] = kept


def pytest_collection_finish(session):
    root = session.config.rootpath
    for item in session.items:
        collected.append({"id": item.nodeid, "file": relative_file(item, root)})


def outcome_of(report):
    if hasattr(report, "wasxfail"):
        return SKIPPED
    if report.when == CALL_PHASE:
        if report.passed:
            return PASSED
        return SKIPPED if report.skipped else FAILED
    if report.failed:
        return ERRORED
    if report.skipped:
        return SKIPPED
    return None


def pytest_runtest_logreport(report):
    outcome = outcome_of(report)
    if outcome is None:
        return
    current = outcomes.get(report.nodeid)
    if current is None or SEVERITY[outcome] > SEVERITY[current]:
        outcomes[report.nodeid] = outcome


def pytest_sessionfinish(session, exitstatus):
    write_report(int(exitstatus), False)
