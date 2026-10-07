"""The verbs that read the truth and judgment files: `analyze`, `derive` and `report-lint`. The only module of the CLI
that reaches the analysis layer."""

from __future__ import annotations

import json
from argparse import Namespace
from pathlib import Path

from calibration.adapters.file_run_store import FileRunStore
from calibration.adapters.git_vcs import GitVersionControl
from calibration.adapters.go_builders import BinaryProductReader
from calibration.adapters.openssl_timestamp import OpensslTimestampReader
from calibration.adapters.pilot_matrix import SnapshotPilotMatrix
from calibration.adapters.pilot_snapshot import UnpackedSnapshot
from calibration.adapters.subprocess_runner import SubprocessRunner
from calibration.analysis.cell_metrics import MetricSettings
from calibration.analysis.derive import derive_policy_inputs, inputs_of
from calibration.analysis.lint import lint_report
from calibration.analysis.pipeline import AnalyzeDeps, AnalyzeSettings, AnalyzeUseCase
from calibration.analysis.report import render_report
from calibration.application.deviation_log import DeviationLog
from calibration.application.snapshot import snapshot_directory
from calibration.cli.arguments import Subparsers
from calibration.cli.layout import Layout
from calibration.cli.wiring import BUILDERS_DIR_PREFIX, HARNESS_COMMIT_KEY
from calibration.domain.canonical import result_json
from calibration.domain.constants import (
    BOOTSTRAP_RESAMPLES,
    DECISIONS_FILE,
    DEFAULT_RUN_ID,
    ENCODING,
    EXIT_FAILURE,
    EXIT_OK,
    EXIT_PRECONDITION,
    MANIFEST_FILE,
    PILOT_PREREG_DIR,
    POLICY_INPUTS_FILE,
    REPORT_FILE,
    SNAPSHOT_FILE,
)
from calibration.domain.deviations import DeviationEntry, DeviationKind
from calibration.domain.freeze import PreconditionFailed
from calibration.domain.jsonread import as_list, json_object, text


def register(subparsers: Subparsers) -> None:
    add = subparsers.add_parser
    analyze = add("analyze", help="recompute every figure of results/ from the recorded bytes")
    analyze.add_argument("--run-id", default=DEFAULT_RUN_ID)
    analyze.add_argument("--resamples", type=int, default=BOOTSTRAP_RESAMPLES)
    analyze.set_defaults(handler=analyze_command)
    derive = add("derive", help="results/policy-inputs.json from results/decisions.json")
    derive.add_argument("--run-id", default=DEFAULT_RUN_ID)
    derive.set_defaults(handler=derive_command)
    lint = add("report-lint", help="lint results/REPORT.md")
    lint.set_defaults(handler=report_lint_command)


def _runner() -> SubprocessRunner:
    return SubprocessRunner()


def _write(results: Path, relative: str, content: str) -> None:
    path = results / relative
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content, encoding=ENCODING)


def _binaries_dir(layout: Layout, run_dir: Path) -> Path:
    manifest = json_object(json.loads((run_dir / MANIFEST_FILE).read_text(encoding=ENCODING)), MANIFEST_FILE)
    return layout.cache_dir / f"{BUILDERS_DIR_PREFIX}{text(manifest.get(HARNESS_COMMIT_KEY), HARNESS_COMMIT_KEY)}"


def _snapshot_dir(layout: Layout, run_dir: Path) -> Path:
    snapshot = json_object(json.loads((run_dir / SNAPSHOT_FILE).read_text(encoding=ENCODING)), SNAPSHOT_FILE)
    return snapshot_directory(layout.cache_dir, text(snapshot.get("pilot_commit"), "snapshot.pilot_commit"))


def _append_new(store: FileRunStore, entries: tuple[DeviationEntry, ...]) -> None:
    log = DeviationLog(store)
    known = {entry.render() for entry in log.entries()}
    for entry in entries:
        if entry.render() not in known:
            log.record(entry.kind, entry.scope, entry.text)


def analyze_command(args: Namespace) -> int:
    layout = Layout.discover()
    run_dir = layout.run_dir(args.run_id)
    runner = _runner()
    snapshot_dir = _snapshot_dir(layout, run_dir)
    settings = AnalyzeSettings(
        run_dir=run_dir,
        pilot_run_dir=layout.pilot_run_dir,
        snapshot_prereg_dir=snapshot_dir / PILOT_PREREG_DIR,
        freeze_receipt=layout.freeze_receipt,
        metric=MetricSettings(resamples=args.resamples),
    )
    deps = AnalyzeDeps(
        reader=BinaryProductReader(runner, _binaries_dir(layout, run_dir)),
        matrix=SnapshotPilotMatrix(UnpackedSnapshot(snapshot_dir)),
        vcs=GitVersionControl(layout.root, runner),
        timestamps=OpensslTimestampReader(runner),
    )
    try:
        output = AnalyzeUseCase(settings, deps).run()
    except PreconditionFailed as failure:
        print(f"calibration: {failure}")
        return EXIT_PRECONDITION
    results = layout.results_dir()
    _write(results, DECISIONS_FILE, result_json(output.decisions))
    for relative, content in sorted(output.files.items()):
        _write(results, relative, content)
    _write(results, REPORT_FILE, render_report(output.decisions, None))
    _append_new(FileRunStore(run_dir), output.new_deviations)
    print(json.dumps({"cells": len(output.cell_outcomes), "files": len(output.files) + 2}, sort_keys=True))
    return EXIT_OK


def derive_command(args: Namespace) -> int:
    layout = Layout.discover()
    run_dir = layout.run_dir(args.run_id)
    results = layout.results_dir()
    decisions = json.loads((results / DECISIONS_FILE).read_text(encoding=ENCODING))
    reader = BinaryProductReader(_runner(), _binaries_dir(layout, run_dir))
    policy = derive_policy_inputs(inputs_of(decisions), reader)
    _write(results, POLICY_INPUTS_FILE, result_json(policy))
    _write(results, REPORT_FILE, render_report(decisions, policy))
    failures = policy.get("failures")
    if isinstance(failures, list) and failures:
        log = DeviationLog(FileRunStore(run_dir))
        known = {entry.render() for entry in log.entries()}
        for failure in failures:
            record = json_object(failure, "failure")
            entry = DeviationEntry.create(
                DeviationKind.PROTOCOL,
                [text(record.get("rule"), "rule"), text(record.get("model"), "model")],
                text(record.get("failure"), "failure"),
            )
            if entry.render() not in known:
                log.record(entry.kind, entry.scope, entry.text)
        return EXIT_FAILURE
    print(json.dumps({"entries": len(as_list(policy.get("entries")) or [])}, sort_keys=True))
    return EXIT_OK


def report_lint_command(_: Namespace) -> int:
    report = Layout.discover().results_dir() / REPORT_FILE
    findings = lint_report(report.read_text(encoding=ENCODING))
    for finding in findings:
        print(finding)
    return EXIT_FAILURE if findings else EXIT_OK
