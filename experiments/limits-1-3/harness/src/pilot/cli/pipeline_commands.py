"""Pipeline verbs: canary, relabel, run-core, run-secondary, leak-audit, log-deviation. Reaches no ground truth."""

from __future__ import annotations

import sys
from argparse import Namespace
from collections.abc import Callable, Sequence

from pilot.adapters.models_lock import DirectoryCanaryStore, JsonSchemaValidator
from pilot.adapters.process import SubprocessRunner
from pilot.adapters.run_directory import RunDirectory
from pilot.application.canary import CanaryFailed, plan_canary, run_canary
from pilot.application.core_run import run_core
from pilot.application.leak_audit_stage import LeakAuditMissing, run_leak_audit
from pilot.application.model_caller import MissingCanary, ModelCaller
from pilot.application.process_mutants import ReferenceUnstable
from pilot.application.provenance import ensure_manifest, log_deviation
from pilot.application.relabel_stage import run_relabel
from pilot.application.secondary_run import SecondaryPreconditionFailed, run_secondary
from pilot.application.traceability_stage import TraceabilityFailed, ensure_traceability
from pilot.application.arm_p import ArmPIntegrityError
from pilot.adapters.provenance import PreregNotCommitted
from pilot.cli.pipeline_layout import PipelineLayout
from pilot.cli.pipeline_wiring import (
    build_model_stack,
    core_dependencies,
    log_to_stderr,
    prompt_library,
)
from pilot.domain.call_type import CallType
from pilot.domain.deviations import DeviationEntry, DeviationKind, InvalidDeviation, parse_scope
from pilot.domain.constants import MAX_CONCURRENT_MODEL_CALLS
from pilot.domain.pipeline_constants import EXIT_QUOTA_EXHAUSTED, LEAK_AUDIT_HINTS_FILENAME
from pilot.domain.quota import QuotaExhausted

EXIT_OK = 0
EXIT_FAILED = 1
LEAK_AUDIT_RAW_DIRNAME = "raw"
AUDIT_IGNORED_PARTS: Sequence[str] = ("__pycache__", ".pytest_cache", ".hypothesis", ".venv")
AUDIT_SUFFIXES: Sequence[str] = (".md", ".py", ".ini", ".toml")
class MissingPreregFile(Exception):
    pass


ABORTS = (
    CanaryFailed,
    MissingCanary,
    TraceabilityFailed,
    ReferenceUnstable,
    SecondaryPreconditionFailed,
    LeakAuditMissing,
    PreregNotCommitted,
    ArmPIntegrityError,
    MissingPreregFile,
)


def _require_prereg(layout: PipelineLayout) -> None:
    for path in (layout.system_prompt, layout.models_lock, layout.prompts_dir, layout.schemas_dir):
        if not path.exists():
            raise MissingPreregFile(f"missing {path}")


QuotaLog = Callable[[str], None]


def _run_quota_log(args: Namespace) -> QuotaLog:
    """Logs a quota pause of a run verb as a `quota-pause` entry of the run's DEVIATIONS.md."""
    store = RunDirectory(PipelineLayout(args.root).run_dir(args.run_id))
    return lambda text: log_deviation(store, DeviationEntry.quota_pause(text))


def _guarded(verb: str, action: Callable[[], int], quota_log: QuotaLog | None = None) -> int:
    try:
        return action()
    except QuotaExhausted as stop:
        if quota_log is not None:
            quota_log(f"{verb} stopped at a quota or rate limit ({stop}); a rerun of {verb} resumes it")
        print(f"{verb}: stopped at quota or rate limit ({stop}); rerun the same command to resume", file=sys.stderr)
        return EXIT_QUOTA_EXHAUSTED
    except ABORTS as failure:
        print(f"{verb}: {type(failure).__name__}: {failure}", file=sys.stderr)
        return EXIT_FAILED


def canary_command(args: Namespace) -> int:
    return _guarded("canary", lambda: _canary(args), _run_quota_log(args))


def _canary(args: Namespace) -> int:
    layout = PipelineLayout(args.root)
    _require_prereg(layout)
    process = SubprocessRunner()
    stack = build_model_stack(layout, layout.raw_dir(args.run_id), process)
    prompts = prompt_library(layout)
    tasks = plan_canary(
        stack.lock.roles, prompts, args.models or (), [CallType(name) for name in args.call_types or ()]
    )
    if not tasks:
        print("canary: no (model, call type) pair selected", file=sys.stderr)
        return EXIT_FAILED
    report = run_canary(
        stack.router, stack.versions, JsonSchemaValidator(), DirectoryCanaryStore(layout.canary_dir(args.run_id)),
        tasks, prompts.system_prompt(),
    )
    print(f"canary: OK ({len(report.baselines)} baselines)")
    return EXIT_OK


def relabel_command(args: Namespace) -> int:
    return _guarded("relabel", lambda: _relabel(args), _run_quota_log(args))


def _relabel(args: Namespace) -> int:
    layout = PipelineLayout(args.root)
    _require_prereg(layout)
    with core_dependencies(layout, args.run_id, args.expected_tests, args.expected_criteria) as deps:
        ensure_manifest(deps.ctx.store, deps.provenance, deps.versions, deps.manifest_inputs)
        bindings = ensure_traceability(deps.ctx.store, deps.traceability, args.expected_tests, args.expected_criteria)
        with deps.timer.stage("relabel"):
            unanswered = run_relabel(deps.ctx, bindings)
    if unanswered:
        print(f"relabel: no answer from {[r.value for r in unanswered]}", file=sys.stderr)
        return EXIT_FAILED
    print("relabel: OK")
    return EXIT_OK


def run_core_command(args: Namespace) -> int:
    return _guarded("run-core", lambda: _run_core(args), _run_quota_log(args))


def _run_core(args: Namespace) -> int:
    _require_prereg(PipelineLayout(args.root))
    with core_dependencies(PipelineLayout(args.root), args.run_id, args.expected_tests, args.expected_criteria) as deps:
        run_core(deps)
    print("run-core: OK")
    return EXIT_OK


def run_secondary_command(args: Namespace) -> int:
    return _guarded("run-secondary", lambda: _run_secondary(args), _run_quota_log(args))


def _run_secondary(args: Namespace) -> int:
    _require_prereg(PipelineLayout(args.root))
    with core_dependencies(PipelineLayout(args.root), args.run_id, args.expected_tests, args.expected_criteria) as deps:
        run_secondary(deps)
    print("run-secondary: OK")
    return EXIT_OK


def leak_audit_command(args: Namespace) -> int:
    return _guarded("leak-audit", lambda: _leak_audit(args))


def _leak_audit(args: Namespace) -> int:
    layout = PipelineLayout(args.root)
    _require_prereg(layout)
    process = SubprocessRunner()
    raw_dir = layout.leak_audit_dir / LEAK_AUDIT_RAW_DIRNAME
    stack = build_model_stack(layout, raw_dir, process)
    prompts = prompt_library(layout)
    caller = ModelCaller(stack.router, {}, prompts.system_prompt(), log_to_stderr, MAX_CONCURRENT_MODEL_CALLS, require_baseline=False)
    store = RunDirectory(layout.leak_audit_dir)
    hints = run_leak_audit(
        caller,
        prompts.template(CallType.LEAK_AUDIT),
        prompts.schema(CallType.LEAK_AUDIT),
        _leak_audit_files(layout),
        lambda payload: store.write_json(LEAK_AUDIT_HINTS_FILENAME, payload),
    )
    print(f"leak-audit: {hints} hint(s) written to {layout.leak_audit_dir}")
    return EXIT_OK


def _leak_audit_files(layout: PipelineLayout) -> dict[str, str]:
    return {
        path.relative_to(layout.project).as_posix(): path.read_text(encoding="utf-8")
        for path in sorted(layout.project.rglob("*"))
        if path.is_file() and not any(part in AUDIT_IGNORED_PARTS for part in path.parts) and path.suffix in AUDIT_SUFFIXES
    }


def log_deviation_command(args: Namespace) -> int:
    """Appends one operator entry (canary fix, rerun, blocked arm, other deviation) to the run's DEVIATIONS.md."""
    try:
        entry = DeviationEntry.create(DeviationKind(args.kind), parse_scope(args.endpoints), args.text)
    except InvalidDeviation as invalid:
        print(f"log-deviation: {invalid}", file=sys.stderr)
        return EXIT_FAILED
    log_deviation(RunDirectory(PipelineLayout(args.root).run_dir(args.run_id)), entry)
    print(f"log-deviation: {entry.render()}")
    return EXIT_OK
