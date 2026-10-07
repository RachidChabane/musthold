"""The verbs that prepare and run the measurement: `snapshot`, `build-items`, `pin`, `canary`, `run`, `run-session` and
`log-deviation`. No module they reach names a truth or judgment file of the pilot."""

from __future__ import annotations

import json
from argparse import ArgumentParser, Namespace
from typing import Final

from calibration.adapters.shallnot_build import ShallnotUnavailable
from calibration.application.block import BlockOutcome, ModelBlock
from calibration.application.build_items import BuildItemsSettings, BuildItemsUseCase
from calibration.application.deviation_log import DeviationLog
from calibration.application.pin import PinError, PinUseCase
from calibration.application.session import RunSession, SessionDeps
from calibration.cli.arguments import Subparsers
from calibration.cli.layout import HARNESS_PATHS, Layout
from calibration.cli.wiring import (
    GuardedBlockLauncher,
    RunServices,
    SessionBusy,
    exclusive_session_lock,
    run_window,
)
from calibration.domain.constants import (
    DEFAULT_RUN_ID,
    EXIT_FAILURE,
    EXIT_OK,
    EXIT_PAUSED,
    EXIT_PRECONDITION,
    SESSION_LOCK_FILE,
)
from calibration.domain.deviations import DeviationKind, InvalidDeviation, parse_scope
from calibration.domain.freeze import PreconditionFailed
from calibration.domain.models import MODEL_IDS, model_by_id

GO_PROCS: Final = 2
OUTCOME_EXIT: Final = {
    BlockOutcome.COMPLETE: EXIT_OK,
    BlockOutcome.BLOCKED: EXIT_OK,
    BlockOutcome.WINDOW_CLOSED: EXIT_PAUSED,
    BlockOutcome.HEAVY_WORK: EXIT_PAUSED,
    BlockOutcome.CANARY_FAILED: EXIT_FAILURE,
}


def add_common(parser: ArgumentParser) -> None:
    parser.add_argument("--run-id", default=DEFAULT_RUN_ID, help="the run directory below runs/")


def add_model(parser: ArgumentParser) -> None:
    parser.add_argument("--model", required=True, choices=MODEL_IDS, help="the model of the block")


def register(subparsers: Subparsers) -> None:
    """Adds the run-side verbs to the `argparse` sub-parsers; each sets `handler`."""
    add = subparsers.add_parser
    snapshot = add("snapshot", help="unpack the pilot snapshot of the a1 tag")
    add_common(snapshot)
    snapshot.set_defaults(handler=snapshot_command)
    build = add("build-items", help="product export, Go builders and every item")
    add_common(build)
    build.add_argument("--go-procs", type=int, default=GO_PROCS, help="go build -p (PREREG 4.1 states 2)")
    build.add_argument("--allow-dirty-harness", action="store_true", help="log a protocol entry instead of refusing")
    build.set_defaults(handler=build_items_command)
    pin = add("pin", help="pin the models and apply the static memory rule")
    add_common(pin)
    pin.set_defaults(handler=pin_command)
    canary = add("canary", help="the canary of one model against its own server")
    add_common(canary)
    add_model(canary)
    canary.set_defaults(handler=canary_command)
    run = add("run", help="one model block")
    add_common(run)
    add_model(run)
    run.set_defaults(handler=run_command)
    session = add("run-session", help="one nightly session")
    add_common(session)
    session.set_defaults(handler=run_session_command)
    deviation = add("log-deviation", help="append a blocked or protocol entry")
    add_common(deviation)
    deviation.add_argument("--kind", required=True, choices=[DeviationKind.BLOCKED.value, DeviationKind.PROTOCOL.value])
    deviation.add_argument("--scope", required=True, help="none, all or identifiers such as K4,M3")
    deviation.add_argument("--text", required=True)
    deviation.set_defaults(handler=log_deviation_command)


def _report(payload: object) -> None:
    print(json.dumps(payload, sort_keys=True, ensure_ascii=False))


def _precondition(failure: PreconditionFailed) -> int:
    print(f"calibration: {failure}")
    return EXIT_PRECONDITION


def snapshot_command(args: Namespace) -> int:
    services = RunServices(Layout.discover(), args.run_id)
    try:
        result = services.snapshot_use_case().run()
    except PreconditionFailed as failure:
        return _precondition(failure)
    _report({"pilot_commit": result.pilot_commit, "files": len(result.files)})
    return EXIT_OK


def build_items_command(args: Namespace) -> int:
    layout = Layout.discover()
    services = RunServices(layout, args.run_id)
    settings = BuildItemsSettings(
        run_id=args.run_id,
        cache_dir=layout.cache_dir,
        builder_dir=layout.builder_dir,
        go_procs=args.go_procs,
        allow_dirty_harness=args.allow_dirty_harness,
        harness_paths=HARNESS_PATHS,
        freeze_receipt=layout.freeze_receipt,
    )
    try:
        report = BuildItemsUseCase(settings, services.build_items_deps(), services.store).run()
    except (PreconditionFailed, ShallnotUnavailable) as failure:
        print(f"calibration: {failure}")
        return EXIT_PRECONDITION
    _report(
        {
            "alignment_items": report.alignment_items,
            "masker_rejections": report.masker_rejections,
            "screen_items": report.screen_items,
            "witness_unmapped": report.witness_unmapped,
            "screen_blocked": report.screen_blocked,
        }
    )
    return EXIT_OK


def pin_command(args: Namespace) -> int:
    services = RunServices(Layout.discover(), args.run_id)
    use_case = PinUseCase(
        services.store,
        DeviationLog(services.store),
        services.memory_probe(),
        services.registry(),
        services.hub(),
        services.kev_environment(),
        services.provisioner(),
    )
    try:
        report = use_case.run()
    except PinError as failure:
        print(f"calibration: {failure}")
        return EXIT_FAILURE
    _report(
        {"pinned": list(report.pinned), "blocked": list(report.blocked), "already_pinned": list(report.already_pinned)}
    )
    return EXIT_OK


def _block(args: Namespace, *, canary_only: bool) -> int:
    services = RunServices(Layout.discover(), args.run_id)
    outcome = ModelBlock(services.block_deps()).run(model_by_id(args.model), canary_only=canary_only)
    _report({"model": args.model, "outcome": outcome.value})
    return OUTCOME_EXIT[outcome]


def canary_command(args: Namespace) -> int:
    return _block(args, canary_only=True)


def run_command(args: Namespace) -> int:
    return _block(args, canary_only=False)


def run_session_command(args: Namespace) -> int:
    layout = Layout.discover()
    services = RunServices(layout, args.run_id)
    deps = SessionDeps(
        store=services.store,
        clock=services.clock,
        memory=services.memory_probe(),
        launcher=GuardedBlockLauncher(services.runner, layout, args.run_id),
        servers=services.server_factory(),
        lock=exclusive_session_lock(layout.run_dir(args.run_id) / SESSION_LOCK_FILE),
        window=run_window(),
    )
    try:
        code = RunSession(deps).run()
    except SessionBusy as busy:
        print(f"calibration: {busy}")
        return EXIT_PRECONDITION
    _report({"exit_code": code})
    return code


def log_deviation_command(args: Namespace) -> int:
    services = RunServices(Layout.discover(), args.run_id)
    try:
        entry = DeviationLog(services.store).record_operator(
            DeviationKind(args.kind), parse_scope(args.scope), args.text
        )
    except InvalidDeviation as invalid:
        print(f"calibration: {invalid}")
        return EXIT_PRECONDITION
    print(entry.render())
    return EXIT_OK
