"""Test writers: one call per (writer, criterion), the static check, then the reference filter (ADR 0006)."""

from __future__ import annotations

from collections.abc import Mapping, Sequence

from pilot.application.context import PipelineContext
from pilot.application.model_caller import CallSpec
from pilot.application.ports import RunRequest, RunStore, SuiteRunners
from pilot.domain.call_type import CallType
from pilot.domain.constants import PACKAGE_NAME, REFERENCE_RUN_COUNT
from pilot.domain.model_call import MissingResult, ModelOutcome, ModelPin
from pilot.domain.model_slate import Writer
from pilot.domain.pipeline_constants import SUITES_DIRNAME
from pilot.domain.prompts import writer_item
from pilot.domain.run_plan import PlannedCall
from pilot.domain.suites import STATUS_MISSING, STATUS_OK, SuiteRecord, SuiteTest, suite_files
from pilot.domain.test_writing import (
    TestStatus,
    WrittenTest,
    public_api,
    static_problems,
    suite_nodeid,
    writer_payload_problems,
    written_tests,
)

SUITE_FILE_TEMPLATE = SUITES_DIRNAME + "/{writer}.json"
PYTHON_SUFFIX = ".py"
REFERENCE_FAILED = "failed or errored in a reference run"


def suite_store_path(writer: Writer) -> str:
    return SUITE_FILE_TEMPLATE.format(writer=writer.value)


def load_suites(store: RunStore, writer: Writer) -> dict[str, SuiteRecord]:
    """The suites of one writer by criterion; empty when the writer has not been run."""
    path = suite_store_path(writer)
    if not store.exists(path):
        return {}
    return {ref: SuiteRecord.from_mapping(raw) for ref, raw in store.read_json(path).items()}


def save_suites(store: RunStore, writer: Writer, suites: Mapping[str, SuiteRecord]) -> None:
    store.write_json(suite_store_path(writer), {ref: suites[ref].to_mapping() for ref in sorted(suites)})


def write_suites(ctx: PipelineContext, calls: Sequence[PlannedCall], pins: Mapping[str, ModelPin]) -> None:
    """One call per planned writer call (resumed when recorded); a suite already filtered is never replaced."""
    criteria = {c.ref: c for c in ctx.criteria()}
    spec_text, source = ctx.sources.spec_text(), ctx.sources.source_files()
    api = public_api({path: text for path, text in source.items() if path.endswith(PYTHON_SUFFIX)})
    template, schema = ctx.prompts.template(CallType.WRITE_TESTS), ctx.prompts.schema(CallType.WRITE_TESTS)
    specs = [
        CallSpec(
            call_id=call.call_id,
            pin=pins[call.model],
            call_type=CallType.WRITE_TESTS,
            item=writer_item(template, spec_text, call.criterion, criteria[call.criterion].statement, source, api, call.nonce),
            schema_text=schema,
            validate=writer_payload_problems,
        )
        for call in calls
    ]
    outcomes = ctx.caller.call_all("write-tests", specs)
    writers = sorted({Writer(call.writer) for call in calls}, key=lambda w: w.value)
    suites = {writer: load_suites(ctx.store, writer) for writer in writers}
    for call in calls:
        recorded = suites[Writer(call.writer)]
        if call.criterion not in recorded or not recorded[call.criterion].reference_checked:
            recorded[call.criterion] = _record(call, outcomes[call.call_id])
    for writer in writers:
        save_suites(ctx.store, writer, suites[writer])
        written = sum(1 for suite in suites[writer].values() if suite.status == STATUS_OK)
        ctx.log(f"[write-tests:{writer.value}] {written}/{len(suites[writer])} suites written")


def _record(call: PlannedCall, outcome: ModelOutcome) -> SuiteRecord:
    writer = Writer(call.writer)
    if isinstance(outcome, MissingResult):
        return SuiteRecord(writer, call.criterion, call.call_id, STATUS_MISSING, ())
    tests = tuple(_checked(test) for test in written_tests(outcome.payload))
    return SuiteRecord(writer, call.criterion, call.call_id, STATUS_OK, tests)


def _checked(test: WrittenTest) -> SuiteTest:
    problems = static_problems(test)
    if problems:
        return SuiteTest(test.name, test.source, TestStatus.REJECTED_STATIC, "; ".join(problems))
    return SuiteTest(test.name, test.source, TestStatus.KEPT)


def filter_against_reference(ctx: PipelineContext, runners: SuiteRunners, writer: Writer) -> None:
    """Drops every test that fails or errors on the reference in any of three runs; marks the suites checked."""
    suites = load_suites(ctx.store, writer)
    pending = {ref: suite for ref, suite in suites.items() if not suite.reference_checked}
    if not pending:
        return
    files = suite_files(list(pending.values()))
    passing = _passing_on_reference(runners, files, [n for suite in pending.values() for n in suite.nodeids])
    for ref, suite in pending.items():
        tests = tuple(_filtered(suite, test, passing) for test in suite.tests)
        suites[ref] = SuiteRecord(suite.writer, suite.criterion, suite.call_id, suite.status, tests, True)
    save_suites(ctx.store, writer, suites)
    dropped = sum(1 for s in pending.values() for t in suites[s.criterion].tests if t.status is TestStatus.DROPPED_REFERENCE)
    ctx.log(f"[reference-filter:{writer.value}] {dropped} test(s) dropped over {len(pending)} suite(s)")


def _passing_on_reference(runners: SuiteRunners, files: Mapping[str, str], nodeids: Sequence[str]) -> set[str]:
    if not files:
        return set()
    results = runners.runner(files).run_many({f"reference-{n}": RunRequest() for n in range(1, REFERENCE_RUN_COUNT + 1)})
    return {nodeid for nodeid in nodeids if all(result.outcome_of(nodeid).is_pass for result in results.values())}


def _filtered(suite: SuiteRecord, test: SuiteTest, passing: set[str]) -> SuiteTest:
    if test.status is not TestStatus.KEPT or suite_nodeid(suite.criterion, test.name) in passing:
        return test
    return SuiteTest(test.name, test.source, TestStatus.DROPPED_REFERENCE, REFERENCE_FAILED)
