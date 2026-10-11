import json
import os

OUTPUT_VARIABLE = "SPEC_PROOF_SCAN_OUT"
CALL_PHASE = "call"
PASSED = "passed"
FAILED = "failed"
ERRORED = "errored"
SKIPPED = "skipped"
SEVERITY = {PASSED: 0, SKIPPED: 1, FAILED: 2, ERRORED: 3}

collected = []
outcomes = {}


def relative_file(item, root):
    try:
        return item.path.relative_to(root).as_posix()
    except ValueError:
        return item.path.as_posix()


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
    output = os.environ.get(OUTPUT_VARIABLE)
    if not output:
        return
    with open(output, "w", encoding="utf-8") as sink:
        json.dump({"collected": collected, "outcomes": outcomes, "exit": int(exitstatus)}, sink)
