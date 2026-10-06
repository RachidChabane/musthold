"""Verb implementations: wire adapters into use cases and report results."""

from __future__ import annotations

import json
import sys
from argparse import Namespace
from collections.abc import Sequence

from pilot.adapters.ast_edits import AstEditApplier
from pilot.adapters.plain_pytest import PlainPytestRunner
from pilot.adapters.prereg_files import (
    write_arm_p,
    DirectoryTestSourceReader,
    load_edit_sets,
    load_labels,
    load_refactor,
    read_label_matrix,
    write_label_matrix,
    write_lines,
)
from pilot.adapters.process import local_process_runner
from pilot.adapters.project_copy import ProjectWorkspace, temporary_work_root
from pilot.adapters.pytest_runner import PytestJunitRunner, WorkspaceVariantRunner
from pilot.adapters.shallnot import ShallnotTraceability
from pilot.application.assemble_arm_p import assemble_items
from pilot.application.isolation_table_stage import compute_isolation_table
from pilot.application.leak_check import leak_check
from pilot.application.runner_crosscheck import runner_crosscheck
from pilot.application.traceability_check import traceability_check
from pilot.application.traceability_stage import TraceabilityFailed
from pilot.application.verify_labels import LabelVerification, verify_labels
from pilot.cli.layout import ExperimentLayout
from pilot.domain.constants import (
    LABEL_MATRIX_FILENAME,
    VERIFY_LOG_FILENAME,
)
from pilot.domain.edits import EditSet
from pilot.domain.ground_truth_paths import ORACLE_FILE_GLOB
from pilot.domain.isolation_table import ISOLATION_TABLE_FILENAME
from pilot.domain.label_rules import ExpectedCounts
from pilot.domain.variants import VariantKind

EXIT_OK = 0
EXIT_FAILED = 1


def expected_counts(args: Namespace) -> ExpectedCounts:
    return ExpectedCounts(args.expected_tests, args.expected_violations, args.expected_planted, args.expected_criteria)


def _all_variants(layout: ExperimentLayout) -> dict[str, EditSet]:
    return {
        **load_edit_sets(layout.violations, VariantKind.VIOLATION),
        **load_edit_sets(layout.planted, VariantKind.PLANTED),
        **load_refactor(layout.refactor),
    }


def _violations_and_refactor(layout: ExperimentLayout) -> dict[str, EditSet]:
    return {
        **load_edit_sets(layout.violations, VariantKind.VIOLATION),
        **load_refactor(layout.refactor),
    }


def _report(title: str, problems: Sequence[object]) -> int:
    if not problems:
        print(f"{title}: OK")
        return EXIT_OK
    print(f"{title}: {len(problems)} failure(s)", file=sys.stderr)
    for problem in problems:
        print(f"  {problem}", file=sys.stderr)
    return EXIT_FAILED


def _verification_log(verification: LabelVerification) -> list[str]:
    lines = [f"tests: {len(verification.matrix.rows)}", f"variants: {len(verification.runs) - 1}"]
    for name in sorted(verification.runs):
        run = verification.runs[name]
        passed = sum(1 for o in run.outcomes.values() if o.is_pass)
        lines.append(f"{name}: {passed} passed, {len(run.outcomes) - passed} not passed")
    lines.append("RESULT: PASS" if verification.passed else f"RESULT: FAIL ({len(verification.failures)})")
    lines += [str(failure) for failure in verification.failures]
    return lines


def verify_labels_command(args: Namespace) -> int:
    layout = ExperimentLayout(args.root)
    out_dir = args.out_dir or layout.prereg
    oracle_files = sorted(layout.oracle.glob(ORACLE_FILE_GLOB))
    with temporary_work_root() as work_root:
        workspace = ProjectWorkspace(layout.project, work_root, AstEditApplier())
        runner = WorkspaceVariantRunner(workspace, PlainPytestRunner(local_process_runner(), oracle_files))
        verification = verify_labels(
            runner, load_labels(layout.labels), _all_variants(layout), expected_counts(args)
        )
    write_label_matrix(out_dir / LABEL_MATRIX_FILENAME, verification.matrix)
    write_lines(out_dir / VERIFY_LOG_FILENAME, _verification_log(verification))
    return _report("verify-labels", verification.failures)


def runner_crosscheck_command(args: Namespace) -> int:
    layout = ExperimentLayout(args.root)
    committed = read_label_matrix((args.out_dir or layout.prereg) / LABEL_MATRIX_FILENAME)
    with temporary_work_root() as work_root:
        workspace = ProjectWorkspace(layout.project, work_root, AstEditApplier())
        runner = WorkspaceVariantRunner(workspace, PytestJunitRunner(local_process_runner()).run)
        differences = runner_crosscheck(runner, committed, _violations_and_refactor(layout))
    return _report("runner-crosscheck", differences)


def leak_check_command(args: Namespace) -> int:
    layout = ExperimentLayout(args.root)
    findings = leak_check(
        DirectoryTestSourceReader(layout.tests_dir, layout.conftest), tuple(args.test_files), args.expected_tests
    )
    return _report("leak-check", findings)


def traceability_check_command(args: Namespace) -> int:
    layout = ExperimentLayout(args.root)
    process = local_process_runner()
    with temporary_work_root() as work_root:
        workspace = ProjectWorkspace(layout.project, work_root, AstEditApplier())
        port = ShallnotTraceability(workspace, PytestJunitRunner(process), process, args.out_dir)
        failures = traceability_check(port, args.expected_tests, args.expected_criteria)
    return _report("traceability-check", failures)


def assemble_arm_p_command(args: Namespace) -> int:
    layout = ExperimentLayout(args.root)
    items = assemble_items(
        load_edit_sets(layout.violations, VariantKind.VIOLATION),
        load_edit_sets(layout.planted, VariantKind.PLANTED),
    )
    write_arm_p(layout.arm_p_items, layout.arm_p_labels, items)
    print(f"assemble-arm-p: {len(items)} items written to {layout.arm_p_items}")
    return EXIT_OK


def isolation_table_command(args: Namespace) -> int:
    """Writes `prereg/isolation_table.json`: C_D8 and the violations another binding's test kills (no model call)."""
    layout = ExperimentLayout(args.root)
    process = local_process_runner()
    matrix = read_label_matrix(layout.prereg / LABEL_MATRIX_FILENAME)
    with temporary_work_root() as work_root:
        workspace = ProjectWorkspace(layout.project, work_root, AstEditApplier())
        port = ShallnotTraceability(workspace, PytestJunitRunner(process), process)
        try:
            table = compute_isolation_table(port, matrix, args.expected_tests, args.expected_criteria)
        except TraceabilityFailed as failure:
            return _report("isolation-table", [failure])
    out_dir = args.out_dir or layout.prereg
    out_dir.mkdir(parents=True, exist_ok=True)
    (out_dir / ISOLATION_TABLE_FILENAME).write_text(json.dumps(table.to_mapping(), indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print(f"isolation-table: C_D8 = {list(table.d8_criteria)} written to {out_dir / ISOLATION_TABLE_FILENAME}")
    return EXIT_OK
