"""Pipeline verbs: canary, relabel, run-core, run-secondary, leak-audit, log-deviation. Reaches no ground truth."""

from __future__ import annotations

import json
import sys
from datetime import datetime
from argparse import Namespace
from collections.abc import Callable, Sequence

from pilot.adapters.models_lock import DirectoryCanaryStore, JsonSchemaValidator
from pilot.adapters.process import SubprocessRunner
from pilot.adapters.run_directory import RunDirectory
from pilot.adapters.run_files import DirectoryImportSource
from pilot.application.blind_spot_stage import BlindSpotPreconditionFailed, run_blindspot
from pilot.application.canary import CanaryFailed, canary_roles, plan_canary, run_canary
from pilot.application.exploratory_calls import ExploratoryLedger, WindowNotOpen
from pilot.application.core_run import run_core
from pilot.application.import_stage import import_run, plan_from_source
from pilot.application.leak_audit_stage import LeakAuditMissing, run_leak_audit
from pilot.application.luna_stage import LunaDependencies, run_luna
from pilot.application.model_caller import MissingCanary, ModelCaller
from pilot.application.process_mutants import ReferenceUnstable
from pilot.application.provenance import ensure_manifest, log_deviation
from pilot.application.relabel_stage import run_relabel
from pilot.application.secondary_run import SecondaryDependencies, SecondaryPreconditionFailed, run_secondary
from pilot.application.stage_timer import StageTimer
from pilot.application.traceability_stage import TraceabilityFailed, ensure_traceability
from pilot.application.arm_p import ArmPIntegrityError
from pilot.adapters.git_drift import PreregistrationTagMissing
from pilot.cli.pipeline_layout import PipelineLayout
from pilot.cli.pipeline_wiring import (
    PipelineWorld,
    build_model_stack,
    core_dependencies,
    log_to_stderr,
    pipeline_world,
    prompt_library,
)
from pilot.domain.call_budget import CALL_BUDGET_PAUSE_TEXT, DEFAULT_CALL_BUDGET, CallBudgetReached
from pilot.domain.call_type import CallType
from pilot.domain.deviations import DeviationEntry, DeviationKind, InvalidDeviation, parse_scope
from pilot.domain.constants import MAX_CONCURRENT_MODEL_CALLS
from pilot.domain.import_manifest import ImportRefused, SOURCE_RUN_ID
from pilot.domain.model_slate import CANARY_PINS, LUNA_CANARY_PINS
from pilot.domain.pipeline_constants import EXIT_QUOTA_EXHAUSTED, LEAK_AUDIT_HINTS_FILENAME
from pilot.domain.quota import QuotaExhausted
from pilot.domain.run_window import RUN_WINDOW_PAUSE_TEXT, InvalidRunWindow, RunWindow, RunWindowClosed

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
    PreregistrationTagMissing,
    ArmPIntegrityError,
    MissingPreregFile,
    ImportRefused,
    InvalidRunWindow,
    BlindSpotPreconditionFailed,
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
    except WindowNotOpen as stop:
        print(f"{verb}: {stop}; rerun once the window has opened", file=sys.stderr)
        return EXIT_QUOTA_EXHAUSTED
    except RunWindowClosed as stop:
        if quota_log is not None:
            quota_log(RUN_WINDOW_PAUSE_TEXT)
        print(f"{verb}: {RUN_WINDOW_PAUSE_TEXT} ({stop}); rerun inside the window to resume", file=sys.stderr)
        return EXIT_QUOTA_EXHAUSTED
    except CallBudgetReached as stop:
        if quota_log is not None:
            quota_log(CALL_BUDGET_PAUSE_TEXT)
        print(f"{verb}: {CALL_BUDGET_PAUSE_TEXT} ({stop}); rerun the same command on another night to resume", file=sys.stderr)
        return EXIT_QUOTA_EXHAUSTED
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
    _require_open_window(args)
    process = SubprocessRunner()
    stack = build_model_stack(layout, layout.raw_dir(args.run_id), process)
    prompts = prompt_library(layout)
    tasks = plan_canary(
        canary_roles(stack.lock.roles, CANARY_PINS), prompts, args.models or (), [CallType(name) for name in args.call_types or ()]
    )
    if not tasks:
        print("canary: no (model, call type) pair selected", file=sys.stderr)
        return EXIT_FAILED
    store = RunDirectory(layout.run_dir(args.run_id))
    with StageTimer(store).stage("canary"):
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
    with core_dependencies(layout, args.run_id, args.expected_tests, args.expected_criteria, args.call_budget, _window(args)) as deps:
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
    layout = PipelineLayout(args.root)
    _require_prereg(layout)
    with core_dependencies(layout, args.run_id, args.expected_tests, args.expected_criteria, args.call_budget, _window(args)) as deps:
        run_core(deps)
    print("run-core: OK")
    return EXIT_OK


def run_blindspot_command(args: Namespace) -> int:
    return _guarded("run-blindspot", lambda: _run_blindspot(args), _run_quota_log(args))


def _run_blindspot(args: Namespace) -> int:
    layout = PipelineLayout(args.root)
    _require_prereg(layout)
    with pipeline_world(layout, args.run_id, args.expected_tests, args.expected_criteria, args.call_budget, _window(args)) as world:
        run_blindspot(world.blind_spot)
    print("run-blindspot: OK")
    return EXIT_OK


def run_secondary_command(args: Namespace) -> int:
    return _guarded("run-secondary", lambda: _run_secondary(args), _run_quota_log(args))


def _run_secondary(args: Namespace) -> int:
    layout = PipelineLayout(args.root)
    _require_prereg(layout)
    with core_dependencies(layout, args.run_id, args.expected_tests, args.expected_criteria, args.call_budget, _window(args)) as deps:
        run_secondary(SecondaryDependencies(deps, ExploratoryLedger(deps.ctx.store)))
    print("run-secondary: OK")
    return EXIT_OK


def run_luna_command(args: Namespace) -> int:
    return _guarded("run-luna", lambda: _run_luna(args), _run_quota_log(args))


def _run_luna(args: Namespace) -> int:
    layout = PipelineLayout(args.root)
    _require_prereg(layout)
    with pipeline_world(layout, args.run_id, args.expected_tests, args.expected_criteria, args.call_budget, _window(args)) as world:
        store = world.core.ctx.store
        canary = _luna_canary(layout, args.run_id, world, lambda: _require_open_window(args))
        run_luna(LunaDependencies(world.blind_spot, ExploratoryLedger(store), canary))
    print("run-luna: OK")
    return EXIT_OK


def _luna_canary(
    layout: PipelineLayout, run_id: str, world: PipelineWorld, require_window: Callable[[], None]
) -> Callable[[], None]:
    """Measures the luna (model, call type) pairs that have no baseline yet; the baselines join the caller's book."""

    def run() -> None:
        prompts = world.core.ctx.prompts
        tasks = [
            task
            for task in plan_canary(canary_roles(world.stack.lock.roles, LUNA_CANARY_PINS), prompts)
            if (task.pin.key, task.call_type) not in world.baselines
        ]
        if not tasks:
            return
        require_window()
        report = run_canary(
            world.stack.router, world.stack.versions, JsonSchemaValidator(), DirectoryCanaryStore(layout.canary_dir(run_id)),
            tasks, prompts.system_prompt(),
        )
        world.baselines.update(report.baselines)

    return run


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


def import_run_01_command(args: Namespace) -> int:
    return _guarded("import-run-01", lambda: _import_run_01(args))


def _import_run_01(args: Namespace) -> int:
    """Carries the sealed run's completed calls into the decisional run; `--plan-only` prints their digests."""
    layout = PipelineLayout(args.root)
    source = DirectoryImportSource(layout.run_dir(SOURCE_RUN_ID))
    if args.plan_only:
        plan, _ = plan_from_source(source)
        print(json.dumps({"import_digests": {entry.source_path: entry.digest for entry in plan.entries}}, indent=2, sort_keys=True))
        return EXIT_OK
    store = RunDirectory(layout.run_dir(args.run_id))
    with StageTimer(store).stage("import-run-01"):
        plan = import_run(source, store)
    print(f"import-run-01: {len(plan.entries)} files imported from {SOURCE_RUN_ID}, {len(plan.retired)} call(s) retired")
    return EXIT_OK


def _require_open_window(args: Namespace) -> None:
    window = _window(args)
    if window is not None and not window.is_open(datetime.now()):
        raise RunWindowClosed(args.run_window)


def _window(args: Namespace) -> RunWindow | None:
    return RunWindow.parse(args.run_window)


def run_window_command(args: Namespace) -> int:
    """Exit 0 inside the run window, 3 outside it; `script/run-pilot` asks before every model-calling step."""
    window = _window(args)
    if window is None or window.is_open(datetime.now()):
        return EXIT_OK
    print(f"run-window: outside {args.run_window} local time", file=sys.stderr)
    return EXIT_QUOTA_EXHAUSTED
