"""Analysis verbs: `oracle-classify`, `audit-sample`, `analyze`, `report-lint`, `precision-table`.

`register(verb)` takes the same `verb(name, handler, help_text)` helper as `pilot.cli.main`.
"""

from __future__ import annotations

import json
import sys
from argparse import ArgumentParser, Namespace
from collections.abc import Callable, Sequence
from pathlib import Path

from pilot.adapters.amendment_facts import OpensslTimestampReader, collect_facts
from pilot.adapters.ast_edits import AstEditApplier
from pilot.adapters.git_drift import PreregistrationTagMissing, drifted_paths, preregistration_commit
from pilot.adapters.ground_truth_reader import read_ground_truth, read_models_lock
from pilot.adapters.oracle_runner import OracleClassifier, OracleRunError
from pilot.adapters.prereg_files import load_edit_sets, load_refactor, read_label_matrix
from pilot.adapters.process import SubprocessRunner, local_process_runner
from pilot.adapters.project_copy import ProjectWorkspace, temporary_work_root
from pilot.adapters.pytest_runner import PytestJunitRunner, WorkspaceVariantRunner
from pilot.adapters.results_writer import write_csv, write_json, write_text
from pilot.adapters.run_reader import RunRecordsError, read_run
from pilot.analysis.audit import audit_sample
from pilot.analysis.lint import lint_report
from pilot.analysis.model import Context, IncompleteInputs
from pilot.analysis.pipeline import AnalysisOutput, run_analysis
from pilot.analysis.precision import precision_table
from pilot.analysis.report import endpoint_documents, render_report
from pilot.analysis.settings import AnalysisSettings
from pilot.application.runner_crosscheck import runner_crosscheck
from pilot.cli.analysis_layout import AnalysisLayout
from pilot.domain.amendment import evaluate_amendment
from pilot.domain.analysis_constants import (
    ANALYSIS_SEED,
    BOOTSTRAP_RESAMPLES,
    DRIFT_GUARDED_PATHS,
    ORACLE_PARALLELISM,
    PERMUTATION_COUNT,
)
from pilot.domain.preregistration import PREREG_TAG
from pilot.domain.constants import LABEL_MATRIX_FILENAME
from pilot.domain.pipeline_constants import AUDIT_SAMPLE_FILENAME
from pilot.domain.variants import VariantKind

EXIT_OK = 0
EXIT_FAILED = 1
Handler = Callable[[Namespace], int]
VerbHelper = Callable[[str, Handler, str], ArgumentParser]
ENCODING = "utf-8"


def register(verb: VerbHelper) -> None:
    sub = verb("oracle-classify", oracle_classify_command, "run the oracle on every non-stillborn mutant (PREREG 6.6)")
    sub.add_argument("--run-id", required=True)
    sub.add_argument("--workers", type=int, default=ORACLE_PARALLELISM)

    sub = verb("audit-sample", audit_sample_command, "draw the A-OPUS audit sample (PREREG 6.7)")
    sub.add_argument("--run-id", required=True)

    sub = verb("analyze", analyze_command, "endpoints, tables and report (PREREG 8 to 13)")
    sub.add_argument("--run-id", required=True)
    sub.add_argument("--allow-drift", action="store_true", help="test-only: skip the pre-registration drift check")
    sub.add_argument("--resamples", type=int, default=BOOTSTRAP_RESAMPLES, help="test-only unless left at the default")
    sub.add_argument("--permutations", type=int, default=PERMUTATION_COUNT, help="test-only unless left at the default")

    sub = verb("amendment-check", amendment_check_command, "tags, declared paths, timestamps, seal and imports of the amendment")
    sub.add_argument("--run-id", required=True)

    sub = verb("report-lint", report_lint_command, "forbidden words and CONSENSUS-without-PRIMARY (PREREG 13)")
    sub.add_argument("--report", type=Path, default=None)

    sub = verb("precision-table", precision_table_command, "Wilson interval table for the pilot's denominators")
    sub.add_argument("--out", type=Path, default=None)


def _fail(title: str, problems: Sequence[object]) -> int:
    print(f"{title}: {len(problems)} problem(s)", file=sys.stderr)
    for problem in problems:
        print(f"  {problem}", file=sys.stderr)
    return EXIT_FAILED


def _context(layout: AnalysisLayout, run_id: str, crosscheck: Sequence[str] | None) -> Context:
    run_dir = layout.run_dir(run_id)
    return Context(read_run(run_dir), read_ground_truth(layout.prereg, run_dir, crosscheck))


# ------------------------------------------------------------------ oracle-classify


def oracle_classify_command(args: Namespace) -> int:
    layout = AnalysisLayout(args.root)
    classifier = OracleClassifier(layout.project, layout.oracle_files, layout.run_dir(args.run_id))
    try:
        executed, reused, unresolved = classifier.classify(args.workers)
    except (OracleRunError, FileNotFoundError, KeyError, ValueError) as error:
        return _fail("oracle-classify", [error])
    print(f"oracle-classify: {executed} executed, {reused} reused")
    return _fail("oracle-classify", [f"unresolved {m}" for m in unresolved]) if unresolved else EXIT_OK


# ------------------------------------------------------------------ audit-sample


def audit_sample_command(args: Namespace) -> int:
    layout = AnalysisLayout(args.root)
    try:
        ctx = _context(layout, args.run_id, None)
    except (RunRecordsError, FileNotFoundError, KeyError, ValueError) as error:
        return _fail("audit-sample", [error])
    missing = ctx.missing_oracle()
    if missing:
        return _fail("audit-sample", [f"no oracle record for {m}; run oracle-classify first" for m in missing[:10]])
    sample = audit_sample(ctx, ANALYSIS_SEED)
    write_json(layout.run_dir(args.run_id) / AUDIT_SAMPLE_FILENAME, sample.to_mapping())
    print(f"audit-sample: {len(sample.mutant_ids)} mutant(s)")
    return EXIT_OK


# ------------------------------------------------------------------ analyze


def _runner_crosscheck(layout: AnalysisLayout) -> list[str]:
    committed = read_label_matrix(layout.prereg / LABEL_MATRIX_FILENAME)
    variants = {
        **load_edit_sets(layout.prereg / "violations", VariantKind.VIOLATION),
        **load_refactor(layout.prereg / "refactor"),
    }
    with temporary_work_root() as work_root:
        workspace = ProjectWorkspace(layout.project, work_root, AstEditApplier())
        runner = WorkspaceVariantRunner(workspace, PytestJunitRunner(local_process_runner()).run)
        return [str(d) for d in runner_crosscheck(runner, committed, variants)]


def _crosscheck_differences(layout: AnalysisLayout) -> list[str]:
    if layout.crosscheck_cache.is_file():
        return list(json.loads(layout.crosscheck_cache.read_text(encoding=ENCODING))["differences"])
    differences = _runner_crosscheck(layout)
    write_json(layout.crosscheck_cache, {"differences": differences})
    return differences


def _check_drift(layout: AnalysisLayout) -> tuple[str, list[str]]:
    commit = preregistration_commit(layout.root, PREREG_TAG)
    return commit, drifted_paths(layout.root, commit, DRIFT_GUARDED_PATHS)


def _provenance(output: AnalysisOutput, ctx: Context, commit: str, settings: AnalysisSettings) -> dict[str, object]:
    manifest = ctx.run.manifest
    return {
        "run id": ctx.run.run_id,
        "pre-registration commit": commit,
        "harness digest": manifest.get("harness_digest"),
        "fixture digest": manifest.get("fixture_digest"),
        "CLI versions": manifest.get("cli_versions"),
        "models": manifest.get("models"),
        "bootstrap resamples": settings.resamples,
        "permutations": settings.permutations,
        "seed": settings.seed,
        "scenarios": f"{output.scenario_count} (including {output.full_set_count} full sets)",
    }


def amendment_problems(layout: AnalysisLayout, run_id: str) -> list[str]:
    process = SubprocessRunner()
    return evaluate_amendment(collect_facts(layout.root, run_id, process, OpensslTimestampReader(process, layout.root)))


def amendment_check_command(args: Namespace) -> int:
    problems = amendment_problems(AnalysisLayout(args.root), args.run_id)
    if problems:
        return _fail("amendment-check", problems)
    print("amendment-check: OK")
    return EXIT_OK


def analyze_command(args: Namespace) -> int:
    layout = AnalysisLayout(args.root)
    settings = AnalysisSettings(args.resamples, args.permutations, ANALYSIS_SEED)
    if not args.allow_drift and (args.resamples, args.permutations) != (BOOTSTRAP_RESAMPLES, PERMUTATION_COUNT):
        return _fail("analyze", ["non-default resamples or permutations need --allow-drift"])
    commit = "uncommitted (--allow-drift)"
    if not args.allow_drift:
        problems = amendment_problems(layout, args.run_id)
        if problems:
            return _fail("analyze", problems)
        try:
            commit, drift = _check_drift(layout)
        except PreregistrationTagMissing as error:
            return _fail("analyze", [error])
        if drift:
            return _fail("analyze", [f"differs from pre-registration commit {commit}: {p}" for p in drift])
    try:
        ctx = _context(layout, args.run_id, _crosscheck_differences(layout))
        output = run_analysis(ctx, read_models_lock(layout.models_lock), settings)
    except (RunRecordsError, IncompleteInputs) as error:
        return _fail("analyze", [error])
    _write_outputs(layout, ctx, output, commit, settings)
    print(f"analyze: composite {output.composite.text}")
    return EXIT_OK


def _write_outputs(layout: AnalysisLayout, ctx: Context, output: AnalysisOutput, commit: str, settings: AnalysisSettings) -> None:
    provenance = _provenance(output, ctx, commit, settings)
    endpoints = endpoint_documents(output.endpoints)
    write_json(layout.decisions, {
        "endpoints": endpoints,
        "composite": output.composite.to_mapping(),
        "feasibility": {"items": [f.to_mapping() for f in output.feasibility], "verdict": output.feasibility_verdict},
        "preconditions": [p.to_mapping() for p in output.preconditions],
        "preregistration_commit": commit,
        "run_id": ctx.run.run_id,
    })
    write_json(layout.endpoints, {
        "endpoints": endpoints,
        "sensitivity": [row.to_mapping() for row in output.sensitivity],
        "settings": {"resamples": settings.resamples, "permutations": settings.permutations, "seed": settings.seed},
    })
    for table in (*output.tables, output.cost_table, output.timing_table, output.failure_table):
        write_csv(layout.results / f"{table.name}.csv", table.csv_rows())
    exploratory = layout.exploratory.read_text(encoding=ENCODING) if layout.exploratory.is_file() else None
    template = layout.report_template.read_text(encoding=ENCODING)
    write_text(layout.report, render_report(template, output, provenance, exploratory))


# ------------------------------------------------------------------ report-lint, precision-table


def report_lint_command(args: Namespace) -> int:
    layout = AnalysisLayout(args.root)
    report = args.report or layout.report
    findings = lint_report(report.read_text(encoding=ENCODING))
    if findings:
        return _fail("report-lint", findings)
    print("report-lint: OK")
    return EXIT_OK


def precision_table_command(args: Namespace) -> int:
    layout = AnalysisLayout(args.root)
    out = args.out or layout.precision_table
    write_csv(out, precision_table().csv_rows())
    print(f"precision-table: wrote {out}")
    return EXIT_OK

