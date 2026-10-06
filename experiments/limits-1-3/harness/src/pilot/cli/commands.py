"""Verb implementations: wire adapters into use cases and report results."""

from __future__ import annotations

import sys
from argparse import Namespace
from collections.abc import Sequence
from pathlib import Path

from pilot.adapters.ast_edits import AstEditApplier
from pilot.adapters.model_cli import ClaudeCli, CodexCli
from pilot.adapters.models_lock import (
    DirectoryCanaryStore,
    InstalledCliVersions,
    JsonSchemaValidator,
    load_models_lock,
)
from pilot.adapters.plain_pytest import PlainPytestRunner
from pilot.adapters.prereg_files import (
    DirectoryTestSourceReader,
    load_edit_sets,
    load_labels,
    load_refactor,
    read_label_matrix,
    write_label_matrix,
    write_lines,
)
from pilot.adapters.process import SubprocessRunner
from pilot.adapters.project_copy import ProjectWorkspace, temporary_work_root
from pilot.adapters.pytest_runner import PytestJunitRunner, WorkspaceVariantRunner
from pilot.adapters.shallnot import ShallnotTraceability
from pilot.application.canary import CanaryFailed, CanarySpec, run_canary
from pilot.application.leak_check import leak_check
from pilot.application.model_router import RoutingModelPort
from pilot.application.runner_crosscheck import runner_crosscheck
from pilot.application.traceability_check import traceability_check
from pilot.application.verify_labels import LabelVerification, verify_labels
from pilot.cli.layout import ExperimentLayout
from pilot.domain.constants import (
    CANARY_ITEM,
    CANARY_SCHEMA_TEXT,
    LABEL_MATRIX_FILENAME,
    ORACLE_FILE_GLOB,
    VERIFY_LOG_FILENAME,
)
from pilot.domain.edits import EditSet
from pilot.domain.label_rules import ExpectedCounts
from pilot.domain.model_call import Cli, Effort
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
        runner = WorkspaceVariantRunner(workspace, PlainPytestRunner(SubprocessRunner(), oracle_files))
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
        runner = WorkspaceVariantRunner(workspace, PytestJunitRunner(SubprocessRunner()).run)
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
    process = SubprocessRunner()
    with temporary_work_root() as work_root:
        workspace = ProjectWorkspace(layout.project, work_root, AstEditApplier())
        port = ShallnotTraceability(workspace, PytestJunitRunner(process), process, args.out_dir)
        failures = traceability_check(port, args.expected_tests, args.expected_criteria)
    return _report("traceability-check", failures)


def canary_command(args: Namespace) -> int:
    layout = ExperimentLayout(args.root)
    if not layout.system_prompt.is_file():
        return _report("canary", [f"missing {layout.system_prompt}"])
    lock = load_models_lock(layout.models_lock)
    process = SubprocessRunner()
    raw_dir = layout.raw_dir(args.run_id)
    router = RoutingModelPort({Cli.CLAUDE: ClaudeCli(process, raw_dir), Cli.CODEX: CodexCli(process, raw_dir)})
    spec = CanarySpec(
        pins=sorted(set(lock.pins), key=lambda pin: pin.key),
        locked_cli_versions=lock.cli_versions,
        effort=Effort.MEDIUM,
        system_prompt=_read_text(layout.system_prompt),
        item=CANARY_ITEM,
        schema_text=CANARY_SCHEMA_TEXT,
    )
    store = DirectoryCanaryStore(layout.canary_dir(args.run_id))
    try:
        baselines = run_canary(router, InstalledCliVersions(process, args.root), JsonSchemaValidator(), store, spec)
    except CanaryFailed as failure:
        return _report("canary", [failure])
    return _report("canary", [] if baselines else ["no model pins in the lock"])


def _read_text(path: Path) -> str:
    return path.read_text(encoding="utf-8")

