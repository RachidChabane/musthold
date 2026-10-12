"""The `attempt` use case: setup stages, scans, replays and cleanup of each candidate (PREREG 8 to 10, 13)."""

from __future__ import annotations

from collections.abc import Callable, Mapping, Sequence
from dataclasses import dataclass
from datetime import datetime
from pathlib import PurePosixPath
from typing import Final

from demos1.application.command_runner import (
    POLL_SECONDS,
    SCAN_CAP_SECONDS,
    SCAN_SLOTS,
    SETUP_SLOTS,
    CommandPlan,
    CommandRunner,
    PlannedRun,
    RefusalBudget,
)
from demos1.application.ports import Clock, ContainerEngine, DeviationLog, Files
from demos1.domain import commands
from demos1.domain.commands import A_SCANS, B_SCANS, SUITE_FORMAT, ProjectFacts, ScanName
from demos1.domain.container import MEMORY, ContainerCommand
from demos1.domain.deviations import Deviation, DeviationKind
from demos1.domain.ecosystem import ECOSYSTEM_ORDER, Ecosystem
from demos1.domain.jsonvalue import JsonObject, JsonShapeError, JsonValue
from demos1.domain.layout import (
    SCAN_DIRECTORY,
    AttemptLayout,
    SetupStage,
    WorkLayout,
    attempt_id,
    container_prefix,
    gitconfig_text,
    replay_container,
    run_directories,
    run_prefix,
    scan_container,
    setup_container,
)
from demos1.domain.lists import BEntry, Entry, ListName, Lists, Stratum, attempt_lists
from demos1.domain.machine import ATTEMPT_CAP_BYTES, MIN_FREE_BYTES, enough_free, over_cap, scratch_paths
from demos1.domain.outcomes import (
    PICK,
    ScanOutcome,
    SetupOutcome,
    replay_result,
    scan_outcome,
    setup_failed,
    setup_terminal,
)
from demos1.domain.records import AttemptRecord, CommandRecord, FindingRecord, ScanRecord, StageRecord
from demos1.domain.scan_output import (
    file_of_test,
    finding_file,
    parse_headline,
    statement_facts,
)
from demos1.domain.suites import SecondRun, SuiteCounts, go_counts, junit_counts, second_run_outcome
from demos1.domain.toolchains import (
    GoVersionError,
    go_directives,
    go_exceeds_image,
    installed_cpython,
    python_request,
)

CLONE_LIMIT_SECONDS: Final = 15 * 60
INSTALL_LIMIT_SECONDS: Final = 20 * 60
FIRST_SUITE_LIMIT_SECONDS: Final = 15 * 60
SECOND_SUITE_LIMIT_SECONDS: Final = 60
STDERR_TAIL_LINES: Final = 40
SHORT_COMMIT: Final = 7
FINDING_RECORD: Final = "finding.json"
STATEMENT: Final = "scan.json"


class UndefinedStage(RuntimeError):
    """Something the pre-registration does not define happened: the verb logs it and stops for the owner."""


@dataclass(frozen=True)
class AttemptSettings:
    """What every container of the run shares."""

    work: WorkLayout
    image: str
    binaries_dir: str
    uid: int
    gid: int
    clone_url: Callable[[str], str]
    clone_protocol: str
    uv_releases: Mapping[str, str]
    environment: JsonObject
    memory: str = MEMORY
    attempt_cap_bytes: int = ATTEMPT_CAP_BYTES
    min_free_bytes: int = MIN_FREE_BYTES


@dataclass(frozen=True)
class Target:
    """The candidate an attempt checks out: its commit, and for a B pick its base."""

    name: ListName
    entry: Entry

    @property
    def attempt(self) -> str:
        return attempt_id(self.name, self.entry.rank)

    @property
    def ecosystem(self) -> Ecosystem:
        return self.name.ecosystem

    @property
    def commit(self) -> str:
        return self.entry.merge_commit if isinstance(self.entry, BEntry) else self.entry.commit

    @property
    def base(self) -> str | None:
        return self.entry.base if isinstance(self.entry, BEntry) else None

    @property
    def scans(self) -> tuple[ScanName, ...]:
        return B_SCANS if self.name.stratum is Stratum.B else A_SCANS

    def identity(self) -> JsonObject:
        entry = self.entry
        document: dict[str, JsonValue] = {
            "list": str(self.name),
            "rank": entry.rank,
            "full_name": entry.full_name,
            "stars": entry.stars,
            "licence": entry.licence,
        }
        if isinstance(entry, BEntry):
            document |= {
                "pull_request_url": entry.url,
                "signals": list(entry.signals),
                "merged_at": entry.merged_at,
                "merge_commit": entry.merge_commit,
                "base": entry.base,
                "files": [path.to_json() for path in entry.files],
            }
        else:
            document |= {"default_branch": entry.default_branch, "commit": entry.commit}
        return document


class AttemptStore:
    """attempt.json of each attempt, written after every container command."""

    def __init__(self, files: Files, work: WorkLayout) -> None:
        self._files = files
        self._work = work

    def load(self, attempt: str) -> AttemptRecord | None:
        document = self._files.read_json(self._work.attempt(attempt).record)
        if not isinstance(document, dict):
            return None
        return AttemptRecord.from_json(document)  # pyright: ignore[reportUnknownArgumentType]

    def save(self, record: AttemptRecord) -> None:
        self._files.write_json(self._work.attempt(record.attempt).record, record.to_json())


class Attempts:
    def __init__(
        self,
        settings: AttemptSettings,
        engine: ContainerEngine,
        files: Files,
        clock: Clock,
        deviations: DeviationLog,
        runner_of: Callable[[Callable[[], None]], CommandRunner],
        refusals: RefusalBudget,
    ) -> None:
        self._settings = settings
        self._engine = engine
        self._files = files
        self._clock = clock
        self._deviations = deviations
        self._runner_of = runner_of
        self._refusals = refusals
        self._store = AttemptStore(files, settings.work)
        self.current: str | None = None
        """The attempt the verb is working on, for the deviations it logs."""

    # The order of PREREG 8.1 -----------------------------------------------------------------------------

    def run_all(self, lists: Lists, picks_per_ecosystem: int) -> list[AttemptRecord]:
        records: list[AttemptRecord] = []
        for ecosystem in ECOSYSTEM_ORDER:
            picks = 0
            for name in attempt_lists(ecosystem):
                for entry in lists.entries(name):
                    if picks >= picks_per_ecosystem:
                        break
                    record = self.run_one(Target(name, entry))
                    records.append(record)
                    if record.outcome == PICK:
                        picks += 1
        return records

    def run_one(self, target: Target) -> AttemptRecord:
        self.current = target.attempt
        layout = self._settings.work.attempt(target.attempt)
        record = self._store.load(target.attempt) or AttemptRecord(
            attempt=target.attempt,
            list_name=str(target.name),
            rank=target.entry.rank,
            identity=target.identity(),
            environment=self._settings.environment,
        )
        if record.finished():
            return record
        self._wait_for_disk()
        record.environment = self._settings.environment
        attempt = _Attempt(self, target, layout, record)
        attempt.run()
        return record

    def _wait_for_disk(self) -> None:
        """PREREG 13: an attempt starts only with enough free space; the wait counts like a slot refusal."""
        root = self._settings.work.root
        self._files.ensure_directory(root)
        while not enough_free(self._files.free_bytes(root), self._settings.min_free_bytes):
            self._clock.sleep(POLL_SECONDS)
            self._refusals.spend(POLL_SECONDS)

    # Shared with one attempt -----------------------------------------------------------------------------

    @property
    def settings(self) -> AttemptSettings:
        return self._settings

    @property
    def engine(self) -> ContainerEngine:
        return self._engine

    @property
    def files(self) -> Files:
        return self._files

    def now(self) -> datetime:
        return self._clock.now()

    def save(self, record: AttemptRecord) -> None:
        self._store.save(record)

    def runner(self, record: AttemptRecord) -> CommandRunner:
        return self._runner_of(lambda: self.save(record))

    def deviate(self, kind: DeviationKind, attempt: str, says: str, happened: str) -> None:
        self._deviations.append(Deviation(self._clock.now(), kind, attempt, says, happened))


class _Attempt:
    """One attempt, resumable from its record at the first command it has not settled."""

    def __init__(self, owner: Attempts, target: Target, layout: AttemptLayout, record: AttemptRecord) -> None:
        self.owner = owner
        self.target = target
        self.layout = layout
        self.record = record
        self.files = owner.files
        self.settings = owner.settings
        self.runner = owner.runner(record)

    def run(self) -> None:
        self.files.ensure_directory(PurePosixPath(self.layout.root))
        if self.record.outcome is None and not self.files.exists(self.layout.gitconfig):
            self.files.ensure_directory(self.layout.home)
            self.files.write_text(self.layout.gitconfig, gitconfig_text(self.layout.clone))
        if self.record.outcome is None:
            self._setup()
        if self.record.outcome == PICK:
            for scan in self.target.scans:
                if self.record.stopped is not None:
                    break
                self._scan(scan)
        self._cleanup()
        self._delete_scratch()
        self.owner.save(self.record)

    def _over_cap(self, details: dict[str, JsonValue]) -> bool:
        """PREREG 13: the size of the attempt directory after a container command, against the cap."""
        size = self.files.tree_size(self.layout.root)
        details["attempt_bytes_after"] = size
        return over_cap(size, self.settings.attempt_cap_bytes)

    # Setup ----------------------------------------------------------------------------------------------

    def _setup(self) -> None:
        handlers: dict[SetupStage, Callable[[StageRecord], SetupOutcome | None]] = {
            SetupStage.CLONE: self._clone,
            SetupStage.TOOLCHAIN: self._toolchain,
            SetupStage.INSTALL: self._install,
            SetupStage.CLEAN_TREE_AFTER_INSTALL: self._clean_tree,
            SetupStage.SUITE_FIRST: self._suite_first,
            SetupStage.SUITE_SECOND: self._suite_second,
            SetupStage.CLEAN_TREE_AFTER_SUITE: self._clean_tree,
        }
        for stage, handler in handlers.items():
            stage_record = self.record.stage(stage.value)
            if not stage_record.done:
                outcome = handler(stage_record)
                if stage_record.command.runs and self._over_cap(stage_record.details):
                    outcome = SetupOutcome.DISK
                    self.record.stopped = f"{SetupOutcome.DISK.value} at {stage.value}"
                stage_record.outcome = outcome
                stage_record.done = True
                self.owner.save(self.record)
            if stage_record.outcome is not None:
                self.record.outcome = stage_record.outcome
                final = stage_record.command.final()
                self.record.outcome_log = final.log if final is not None else None
                self.owner.save(self.record)
                return
        self.record.outcome = PICK
        self.owner.save(self.record)

    def _container(
        self,
        name: str,
        argv: list[str],
        prefix: PurePosixPath,
        tmpdir: PurePosixPath,
        *,
        network: bool,
        limit: int | None,
        workdir: PurePosixPath,
        clone: bool = False,
        xdg_run: PurePosixPath | None = None,
    ) -> ContainerCommand:
        environment = self.layout.environment(tmpdir)
        xdg = run_directories(xdg_run)[1] if xdg_run is not None else None
        variables = environment.variables(
            offline=not network, clone_protocol=self.settings.clone_protocol if clone else None, xdg=xdg
        )
        return ContainerCommand(
            name=name,
            image=self.settings.image,
            attempt_dir=str(self.layout.root),
            binaries_dir=self.settings.binaries_dir,
            workdir=str(workdir),
            network=network,
            environment=variables,
            argv=argv,
            record_prefix=str(prefix),
            limit_seconds=limit,
            uid=self.settings.uid,
            gid=self.settings.gid,
            memory=self.settings.memory,
        )

    def _setup_plan(
        self,
        stage: SetupStage,
        argv: list[str],
        *,
        network: bool,
        limit: int | None,
        workdir: PurePosixPath,
        clone: bool = False,
        cleared: Sequence[PurePosixPath] = (),
    ) -> CommandPlan:
        def planned(run: int) -> PlannedRun:
            prefix = self.layout.setup_prefix(stage, run)
            tmpdir = self.layout.setup_tmp(stage, run)
            command = self._container(
                setup_container(self.target.attempt, stage, run),
                argv,
                prefix,
                tmpdir,
                network=network,
                limit=limit,
                workdir=workdir,
                clone=clone,
            )
            return PlannedRun(command, (tmpdir, self.layout.logs), prefix, cleared)

        return CommandPlan(SETUP_SLOTS, None, planned)

    def _settle(self, plan: CommandPlan, command: CommandRecord) -> None:
        self.files.ensure_directory(self.layout.logs)
        self.runner.settle(plan, command)

    def _clone(self, stage: StageRecord) -> SetupOutcome | None:
        argv = commands.clone_command(
            self.settings.clone_url(self.target.entry.full_name),
            str(self.layout.clone),
            self.target.commit,
            self.target.base,
        )
        plan = self._setup_plan(
            SetupStage.CLONE, argv, network=True, limit=CLONE_LIMIT_SECONDS, workdir=self.layout.root, clone=True
        )
        self._settle(plan, stage.command)
        return setup_failed(stage.command.end(), SetupOutcome.CLONE)

    def _root_text(self, name: str) -> str | None:
        return self.files.read_text(self.layout.clone / name)

    def _toolchain(self, stage: StageRecord) -> SetupOutcome | None:
        if self.target.ecosystem is Ecosystem.GO:
            go_mod = self._root_text("go.mod")
            directives = go_directives(go_mod or "")
            stage.details |= {
                "go_mod_present": go_mod is not None,
                "go": directives.go,
                "toolchain": directives.toolchain,
            }
            try:
                return SetupOutcome.TOOLCHAIN if go_exceeds_image(directives) else None
            except GoVersionError as error:
                raise UndefinedStage(f"the go.mod directives cannot be compared with 1.26.4: {error}") from error
        request = python_request(
            self._root_text(".python-version"),
            self._root_text("pyproject.toml"),
            self._root_text("setup.cfg"),
            self.settings.uv_releases,
        )
        stage.details |= {
            "python_rule": request.rule.value,
            "python_request": request.request,
            "python_specifier": request.specifier,
        }
        if request.unreadable is not None:
            self.owner.deviate(
                DeviationKind.UNDEFINED_STAGE,
                self.target.attempt,
                "PREREG 8.2 reads the version request from the root files of the checkout.",
                f"{request.unreadable} could not be parsed; the next rule decided.",
            )
        if request.request is None:
            return SetupOutcome.TOOLCHAIN
        plan = self._setup_plan(
            SetupStage.TOOLCHAIN,
            commands.python_install_command(request.request),
            network=True,
            limit=None,
            workdir=self.layout.clone,
        )
        self._settle(plan, stage.command)
        failed = setup_failed(stage.command.end(), SetupOutcome.TOOLCHAIN)
        resolved = installed_cpython(self.files.list_names(self.layout.uv_python))
        stage.details["python_resolved"] = resolved[-1] if len(resolved) == 1 else (resolved or None)
        return failed

    def _python_request(self) -> str | None:
        value = self.record.stage(SetupStage.TOOLCHAIN.value).details.get("python_request")
        return value if isinstance(value, str) else None

    def _install(self, stage: StageRecord) -> SetupOutcome | None:
        present = [
            name for name in commands.recipe_reads(self.target.ecosystem) if self.files.exists(self.layout.clone / name)
        ]
        pyproject = self._root_text("pyproject.toml") if "pyproject.toml" in present else None
        facts = ProjectFacts(frozenset(present), pyproject)
        if self.target.ecosystem is Ecosystem.PYTEST and commands.pyproject_tables(pyproject).unreadable:
            self.owner.deviate(
                DeviationKind.UNDEFINED_STAGE,
                self.target.attempt,
                "PREREG 8.3 reads the extras and dependency groups of the root pyproject.toml.",
                "pyproject.toml could not be parsed; the recipe installed no extra and no group.",
            )
        argv = commands.install_command(self.target.ecosystem, facts, self._python_request())
        stage.details |= {"recipe_files": sorted(present), "recipe": argv}
        plan = self._setup_plan(
            SetupStage.INSTALL,
            argv,
            network=True,
            limit=INSTALL_LIMIT_SECONDS,
            workdir=self.layout.clone,
            cleared=(self.layout.clone / commands.VENV_DIRECTORY,),
        )
        self._settle(plan, stage.command)
        return setup_failed(stage.command.end(), SetupOutcome.INSTALL)

    def _clean_tree(self, stage: StageRecord) -> SetupOutcome | None:
        setup_stage = SetupStage(stage.stage)
        plan = self._setup_plan(
            setup_stage,
            commands.clean_tree_command(str(self.layout.clone)),
            network=False,
            limit=None,
            workdir=self.layout.clone,
        )
        self._settle(plan, stage.command)
        end = stage.command.end()
        terminal = setup_terminal(end)
        if terminal is not None:
            return terminal
        final = stage.command.final()
        output = self.files.read_text(PurePosixPath(final.stdout)) if final is not None else None
        stage.details["status_output"] = output
        if output:
            return SetupOutcome.DIRTY_TREE
        if end.exit_code != 0:
            raise UndefinedStage(f"git status exited {end.exit_code} without output in stage {stage.stage}")
        return None

    def _suite(self, stage: StageRecord, setup_stage: SetupStage, run: int, limit: int) -> tuple[SuiteCounts, bool]:
        output = self.layout.suite_output(run, SUITE_FORMAT[self.target.ecosystem].value)
        plan = self._setup_plan(
            setup_stage,
            commands.suite_command(self.target.ecosystem, str(output)),
            network=False,
            limit=limit,
            workdir=self.layout.clone,
        )
        self._settle(plan, stage.command)
        report = self.files.read_text(output)
        final = stage.command.final()
        if self.target.ecosystem is Ecosystem.PYTEST:
            counts, named_pass = junit_counts(report), False
        else:
            suite = go_counts(report)
            counts, named_pass = suite.counts, suite.named_pass
        stage.details |= {
            "output": str(output),
            "counts": counts.to_json(),
            "named_pass": named_pass,
            "exit_status": final.exit_code if final is not None else None,
            "wall_seconds": final.wall_seconds if final is not None else None,
        }
        return counts, named_pass

    def _suite_first(self, stage: StageRecord) -> SetupOutcome | None:
        self._suite(stage, SetupStage.SUITE_FIRST, 1, FIRST_SUITE_LIMIT_SECONDS)
        end = stage.command.end()
        terminal = setup_terminal(end)
        if terminal is not None:
            return terminal
        return SetupOutcome.SUITE_FIRST_RUN_TIMEOUT if end.timed_out else None

    def _suite_second(self, stage: StageRecord) -> SetupOutcome | None:
        counts, named_pass = self._suite(stage, SetupStage.SUITE_SECOND, 2, SECOND_SUITE_LIMIT_SECONDS)
        end = stage.command.end()
        terminal = setup_terminal(end)
        if terminal is not None:
            return terminal
        stage.details["suite_runtime_seconds"] = stage.details.get("wall_seconds")
        return second_run_outcome(
            self.target.ecosystem,
            SecondRun(
                exit_status=end.exit_code if end.exit_code is not None else -1,
                timed_out=end.timed_out,
                counts=counts,
                named_pass=named_pass,
            ),
        )

    # Scans ----------------------------------------------------------------------------------------------

    def _scan_plan(self, scan: ScanName) -> CommandPlan:
        clone = self.layout.clone

        def planned(run: int) -> PlannedRun:
            run_dir = self.layout.scan_run(scan.value, run)
            tmpdir, xdg = run_directories(run_dir)
            command = self._container(
                scan_container(self.target.attempt, scan.value, run),
                commands.scan_command(str(clone), scan, self.target.base),
                run_prefix(run_dir),
                tmpdir,
                network=False,
                limit=None,
                workdir=clone,
                xdg_run=run_dir,
            )
            return PlannedRun(
                command,
                (run_dir, tmpdir, PurePosixPath(xdg.cache_home), PurePosixPath(xdg.state_home)),
                run_prefix(run_dir),
            )

        return CommandPlan(SCAN_SLOTS, SCAN_CAP_SECONDS, planned)

    def _replay_plan(self, scan: ScanName, finding: int, folder: str) -> CommandPlan:
        clone = self.layout.clone

        def planned(run: int) -> PlannedRun:
            run_dir = self.layout.replay_run(scan.value, finding, run)
            tmpdir, xdg = run_directories(run_dir)
            command = self._container(
                replay_container(self.target.attempt, scan.value, finding, run),
                commands.replay_command(str(clone), folder),
                run_prefix(run_dir),
                tmpdir,
                network=False,
                limit=None,
                workdir=clone,
                xdg_run=run_dir,
            )
            return PlannedRun(
                command,
                (run_dir, tmpdir, PurePosixPath(xdg.cache_home), PurePosixPath(xdg.state_home)),
                run_prefix(run_dir),
            )

        return CommandPlan(SCAN_SLOTS, SCAN_CAP_SECONDS, planned)

    def _scan(self, scan: ScanName) -> None:
        record = self.record.scan(scan.value)
        if record.done:
            return
        if not record.command.settled:
            self.files.ensure_directory(self.layout.scan_dir(scan.value))
            self.runner.settle(self._scan_plan(scan), record.command)
            self._after_scan(record)
            if self._over_cap(record.measures):
                self._stop_on_disk(record)
            self.owner.save(self.record)
        if not record.findings_listed:
            self._list_findings(record)
            self.owner.save(self.record)
        for finding in sorted(record.findings, key=lambda item: item.rank):
            if self.record.stopped is not None:
                break
            if finding.replay_result is None:
                self.runner.settle(self._replay_plan(scan, finding.rank, finding.folder), finding.replay)
                finding.replay_result = replay_result(finding.replay.end()).value
                if self._over_cap(record.measures):
                    self._stop_on_disk(record)
                self.owner.save(self.record)
        record.done = True
        self.owner.save(self.record)

    def _stop_on_disk(self, record: ScanRecord) -> None:
        """PREREG 13: above the cap, the scan's stage ends as scan-disk and the attempt stops there."""
        record.outcome = ScanOutcome.DISK.value
        self.record.stopped = f"{ScanOutcome.DISK.value} at {record.scan}"

    def _after_scan(self, record: ScanRecord) -> None:
        record.outcome = scan_outcome(record.command.end()).value
        final = record.command.final()
        if final is not None:
            stdout = self.files.read_text(PurePosixPath(final.stdout))
            record.headline_text = stdout
            headline = parse_headline(stdout or "")
            record.headline = headline.to_json()
            stderr = self.files.read_text(PurePosixPath(final.stderr)) or ""
            record.stderr_tail = "\n".join(stderr.splitlines()[-STDERR_TAIL_LINES:])
            if stdout and not (headline.no_model and headline.promises_not_checked):
                self.owner.deviate(
                    DeviationKind.DEPARTURE,
                    self.target.attempt,
                    'PREREG 2: each scan prints "no model used" and "promises not checked".',
                    f"the output of scan {record.scan} lacks one of them.",
                )
        self._move(record)

    def _move(self, record: ScanRecord) -> None:
        """PREREG 9.2: `.spec-proof/scan/<commit>/` leaves the clone, as plain files, for the scan's directory."""
        scans = self.layout.clone / SCAN_DIRECTORY
        short = self.target.commit[:SHORT_COMMIT]
        source = scans / short
        if not self.files.is_directory(source):
            return
        target = self.layout.moved(record.scan)
        self.files.move(source, target)
        record.moved = str(target)
        statement = target / STATEMENT
        record.scan_json_sha256 = self.files.sha256(statement)
        document = self.files.read_json(statement)
        if document is not None:
            try:
                record.statement = statement_facts(document)
            except (JsonShapeError, ValueError) as error:
                raise UndefinedStage(f"scan.json of {record.scan} cannot be read: {error}") from error

    def _list_findings(self, record: ScanRecord) -> None:
        if record.moved is None:
            record.findings_listed = True
            return
        moved = PurePosixPath(record.moved)
        headline = parse_headline(record.headline_text or "")
        by_rank = {finding.rank: finding for finding in headline.findings}
        folders = sorted(int(name) for name in self.files.list_names(moved) if name.isdigit())
        for number in folders:
            folder = moved / str(number)
            document = self.files.read_json(folder / FINDING_RECORD)
            try:
                found = finding_file(document) if document is not None else None
            except (JsonShapeError, ValueError):
                found = None
            printed = by_rank.get(number)
            test = found.test if found is not None else (printed.test if printed is not None else "")
            record.findings.append(
                FindingRecord(
                    rank=number,
                    test=test,
                    sentence=printed.sentence if printed is not None else None,
                    site_file=printed.site_file if printed is not None else None,
                    site_line=printed.site_line if printed is not None else None,
                    route=found.route if found is not None else None,
                    folder=str(folder),
                    pull_request_status=self._pull_request_status(test),
                )
            )
        record.findings_listed = True

    def _pull_request_status(self, test: str) -> str | None:
        if not isinstance(self.target.entry, BEntry) or not test:
            return None
        test_file = file_of_test(test)
        for path in self.target.entry.files:
            if path.test and path.path == test_file and path.status in ("added", "modified", "renamed"):
                return path.status
        return None

    # Cleanup --------------------------------------------------------------------------------------------

    def _cleanup(self) -> None:
        """PREREG 13: at the end of each attempt, `docker ps -a --filter name=<id>-` lists nothing."""
        prefix = container_prefix(self.target.attempt)
        left = list(self.owner.engine.names_with_prefix(prefix))
        for name in left:
            if self.owner.engine.running(name):
                self.owner.engine.stop(name)
            self.owner.engine.inspect(name)
            self.owner.engine.remove(name)
        if left:
            self.owner.deviate(
                DeviationKind.DEPARTURE,
                self.target.attempt,
                "PREREG 13: at the end of each attempt, docker ps -a --filter name=<id>- lists nothing.",
                f"containers left at the end of the attempt, then inspected and removed: {', '.join(left)}",
            )
        self.record.cleanup = {
            "containers_listed": left,
            "containers_listed_after": list(self.owner.engine.names_with_prefix(prefix)),
            "checked_at": self.owner.now().isoformat(),
        }

    def _delete_scratch(self) -> None:
        """PREREG 13: once the records and the replays are written, the clone, cache, home and TMPDIRs go."""
        layout = self.layout
        tmpdirs = [*self.record.tmpdirs(), str(layout.tmp_root)]
        paths = scratch_paths(layout.root, layout.clone, layout.cache, layout.home, tmpdirs)
        deleted = [str(path) for path in paths if self.files.exists(path)]
        for path in paths:
            self.files.remove_tree(path)
        cleanup = dict(self.record.cleanup or {})
        cleanup["deleted"] = deleted
        cleanup["attempt_bytes_kept"] = self.files.tree_size(layout.root)
        self.record.cleanup = cleanup
