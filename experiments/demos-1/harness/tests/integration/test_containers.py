"""PREREG 15.4 on two local fixture repositories, one pytest and one Go, in the engine's real machine.

Run through script/test-containers, which builds the product from HEAD and the image into a scratch
$DEMOS1_WORK and $DEMOS1_PRODUCT, and stops the machine whatever the outcome.
"""

from __future__ import annotations

import os
import shutil
import subprocess
from collections.abc import Callable, Iterator
from pathlib import Path, PurePosixPath

import pytest

from demos1.adapters.engine import ColimaMachine, DockerEngine
from demos1.adapters.host import LocalHostFacts
from demos1.adapters.local import DeviationFile, LocalFiles, SystemClock
from demos1.application.attempt_run import AttemptContext, AttemptVerb
from demos1.application.command_runner import CommandPlan, CommandRunner, PlannedRun, RefusalBudget
from demos1.application.environment import EnvironmentProbe
from demos1.application.machine_session import machine_running
from demos1.domain.container import ContainerCommand
from demos1.domain.ecosystem import Ecosystem
from demos1.domain.layout import WorkLayout
from demos1.domain.lists import AEntry, BEntry, Lists, PathEntry
from demos1.domain.machine import GIB, docker_socket
from demos1.domain.records import AttemptRecord, CommandRecord

pytestmark = pytest.mark.docker

FIXTURES = Path(__file__).resolve().parent.parent / "fixtures" / "repos"
REPO_ROOT = Path(__file__).resolve().parents[5]
GIT_IDENTITY = {
    "GIT_AUTHOR_NAME": "Tallyho Maintainers",
    "GIT_AUTHOR_EMAIL": "maintainers@tallyho.example",
    "GIT_COMMITTER_NAME": "Tallyho Maintainers",
    "GIT_COMMITTER_EMAIL": "maintainers@tallyho.example",
    "GIT_AUTHOR_DATE": "2026-09-30T10:00:00Z",
    "GIT_COMMITTER_DATE": "2026-09-30T10:00:00Z",
}
TEST_MIN_FREE_BYTES = 1 * GIB
TEST_DISK_CAP_BYTES = 1 << 20
OOM_MEMORY = "64m"


def git(*arguments: str, cwd: Path) -> str:
    environment = {**os.environ, **GIT_IDENTITY, "GIT_CONFIG_GLOBAL": "/dev/null", "GIT_CONFIG_SYSTEM": "/dev/null"}
    return subprocess.run(
        ["git", *arguments], cwd=cwd, env=environment, capture_output=True, text=True, check=True
    ).stdout.strip()


def commit_tree(source: Path, work: Path, message: str) -> str:
    shutil.copytree(source, work, dirs_exist_ok=True)
    git("add", "-A", cwd=work)
    git("commit", "-q", "-m", message, cwd=work)
    return git("rev-parse", "HEAD", cwd=work)


def bare_repository(name: str, scratch: Path) -> tuple[Path, str, str | None]:
    """The fixture's history as a bare repository: its base commit and, when it has one, its change."""
    work = scratch / f"{name}-work"
    work.mkdir(parents=True)
    git("init", "-q", "-b", "main", cwd=work)
    base = commit_tree(FIXTURES / name / "base", work, "Start the project")
    change = None
    if (FIXTURES / name / "change").is_dir():
        change = commit_tree(FIXTURES / name / "change", work, "Add the late fee")
    bare = scratch / f"{name}.git"
    git("clone", "-q", "--bare", str(work), str(bare), cwd=scratch)
    return bare, base, change


@pytest.fixture(scope="module")
def work() -> WorkLayout:
    if not os.environ.get("DEMOS1_WORK") or not os.environ.get("DEMOS1_PRODUCT"):
        pytest.fail("run through script/test-containers, which sets DEMOS1_WORK and DEMOS1_PRODUCT")
    return WorkLayout(
        PurePosixPath(Path(os.environ["DEMOS1_WORK"]).resolve()),
        PurePosixPath(Path(os.environ["DEMOS1_PRODUCT"]).resolve()),
    )


@pytest.fixture(scope="module")
def wiring(work: WorkLayout) -> Iterator[dict[str, object]]:
    clock = SystemClock()
    files = LocalFiles()
    engine = DockerEngine(docker_socket(PurePosixPath(Path.home())), PurePosixPath(REPO_ROOT / "script"), clock.now)
    machine = ColimaMachine()
    yield {"clock": clock, "files": files, "engine": engine, "machine": machine}
    machine.stop()


def place_origin(work: WorkLayout, attempt: str, bare: Path) -> str:
    """The bare repository copied, as plain files, into the attempt directory the clone container mounts."""
    target = Path(work.attempt(attempt).root) / "origin.git"
    shutil.copytree(bare, target)
    return str(target)


def run_attempts(
    work: WorkLayout, wiring: dict[str, object], lists: Lists, origins: dict[str, str], cap: int = 15 * GIB
) -> list[AttemptRecord]:
    files: LocalFiles = wiring["files"]  # type: ignore[assignment]
    engine: DockerEngine = wiring["engine"]  # type: ignore[assignment]
    clock: SystemClock = wiring["clock"]  # type: ignore[assignment]
    machine: ColimaMachine = wiring["machine"]  # type: ignore[assignment]
    refusals = RefusalBudget()

    def runner_of(persist: Callable[[], None]) -> CommandRunner:
        return CommandRunner(engine, files, clock, refusals, persist)

    files.write_json(work.lists, lists.to_json())
    if not files.exists(work.manifest):
        files.write_json(work.manifest, {"product_tag": "HEAD", "facts": {}})
    probe = EnvironmentProbe(files, LocalHostFacts(engine), machine, runner_of(lambda: None), os.getuid(), os.getgid())
    verb = AttemptVerb(files, engine, clock, DeviationFile(files, work.deviations), probe, runner_of, machine, refusals)
    context = AttemptContext(
        work,
        os.getuid(),
        os.getgid(),
        clone_url=lambda name: origins[name],
        clone_protocol="file",
        attempt_cap_bytes=cap,
        min_free_bytes=TEST_MIN_FREE_BYTES,
    )
    return verb.run(context)


def tallyho_entry(base: str, change: str) -> BEntry:
    return BEntry(
        rank=1,
        full_name="tallyho-shop/tallyho",
        stars=1200,
        licence="MIT",
        number=12,
        url="https://github.com/tallyho-shop/tallyho/pull/12",
        signals=("copilot",),
        merged_at="2026-09-30T10:00:00Z",
        merge_commit=change,
        base=base,
        files=(PathEntry("tallyho/late.py", "added", False), PathEntry("tests/test_late.py", "added", True)),
    )


def meter_entry(rank: int, commit: str) -> AEntry:
    return AEntry(rank, "meter-dev/meter", 3400, "MIT", "main", commit, "2026-09-30T10:00:00Z")


def machine_processes() -> list[str]:
    """Host processes that serve the profile's machine, found by name and never signalled."""
    found = subprocess.run(["pgrep", "-fl", "colima-spec-proof-demos"], capture_output=True, text=True, check=False)
    return [line for line in found.stdout.splitlines() if "pgrep" not in line]


def visible_in_machine(path: Path) -> bool:
    probe = subprocess.run(
        ["colima", "ssh", "--profile", "spec-proof-demos", "--", "test", "-e", str(path)],
        capture_output=True,
        text=True,
        check=False,
    )
    return probe.returncode == 0


def docker_context() -> str:
    return subprocess.run(["docker", "context", "show"], capture_output=True, text=True, check=False).stdout.strip()


def ssh_config_state() -> float | None:
    path = Path.home() / ".ssh" / "config"
    return path.stat().st_mtime if path.exists() else None


@pytest.fixture(scope="module")
def picks(work: WorkLayout, wiring: dict[str, object], tmp_path_factory: pytest.TempPathFactory) -> dict[str, object]:
    scratch = tmp_path_factory.mktemp("origins")
    tallyho, tallyho_base, tallyho_change = bare_repository("tallyho", scratch)
    meter, meter_base, _ = bare_repository("meter", scratch)
    assert tallyho_change is not None
    origins = {
        "tallyho-shop/tallyho": place_origin(work, "B-pytest-01", tallyho),
        "meter-dev/meter": place_origin(work, "A-go-01", meter),
    }
    lists = Lists(
        b={Ecosystem.PYTEST: [tallyho_entry(tallyho_base, tallyho_change)], Ecosystem.GO: []},
        a={Ecosystem.PYTEST: [], Ecosystem.GO: [meter_entry(1, meter_base)]},
    )
    context_before, ssh_before = docker_context(), ssh_config_state()
    records = {record.attempt: record for record in run_attempts(work, wiring, lists, origins)}
    machine: ColimaMachine = wiring["machine"]  # type: ignore[assignment]
    return {
        "records": records,
        "status_after": machine.status(),
        "processes_after": machine_processes(),
        "context": (context_before, docker_context()),
        "ssh": (ssh_before, ssh_config_state()),
        "meter_base": meter_base,
        "meter_bare": meter,
    }


def test_git_in_the_container_accepts_the_mounted_clone(picks: dict[str, object]):
    records: dict[str, AttemptRecord] = picks["records"]  # type: ignore[assignment]
    for attempt in ("B-pytest-01", "A-go-01"):
        record = records[attempt]
        assert record.stage("clone").outcome is None, record.stage("clone").command.runs
        assert record.stage("clean-tree-1").outcome is None
        assert record.stage("clean-tree-2").outcome is None
        assert record.outcome == "pick", (attempt, record.outcome, record.outcome_log)


def test_install_and_suites_ran_in_the_container(picks: dict[str, object]):
    records: dict[str, AttemptRecord] = picks["records"]  # type: ignore[assignment]
    pytest_record = records["B-pytest-01"]
    toolchain = pytest_record.stage("toolchain").details
    assert toolchain["python_request"] == "3.13"
    assert isinstance(toolchain["python_resolved"], str) and toolchain["python_resolved"].startswith("3.13.")
    counts = pytest_record.stage("suite-2").details["counts"]
    assert isinstance(counts, dict) and counts["passing"] == 20
    go_counts = records["A-go-01"].stage("suite-2").details["counts"]
    assert isinstance(go_counts, dict) and go_counts["passing"] == 13
    for record in records.values():
        install = record.stage("install").command.runs[0]
        assert install.network
        suite = record.stage("suite-2").command.runs[0]
        assert not suite.network
        assert suite.wall_seconds is not None and suite.wall_seconds < 60


def test_scans_and_replays_run_offline(picks: dict[str, object]):
    records: dict[str, AttemptRecord] = picks["records"]  # type: ignore[assignment]
    assert [scan.scan for scan in records["B-pytest-01"].scans] == ["change", "quick", "change-full"]
    assert [scan.scan for scan in records["A-go-01"].scans] == ["quick", "full"]
    replayed = 0
    for record in records.values():
        for scan in record.scans:
            assert scan.outcome == "scan-ok", (record.attempt, scan.scan, scan.stderr_tail)
            assert scan.statement is not None and scan.scan_json_sha256 is not None
            assert scan.headline is not None and scan.headline["no_model"] and scan.headline["promises_not_checked"]
            assert all(not run.network for run in scan.command.runs)
            for finding in scan.findings:
                assert finding.replay_result in ("holds on replay", "no longer holds on replay", "replay failed")
                assert all(not run.network for run in finding.replay.runs)
                replayed += 1
    assert replayed > 0


def test_the_wrapper_reports_memory_peak_and_wall_time(picks: dict[str, object]):
    records: dict[str, AttemptRecord] = picks["records"]  # type: ignore[assignment]
    for record in records.values():
        for run in record.runs():
            assert run.memory_peak_bytes is not None and run.memory_peak_bytes > 0, run.name
            assert run.wall_seconds is not None
            assert run.exit_code is not None and run.oom_killed is False


def test_the_end_of_each_attempt_leaves_no_container_and_only_the_records(picks: dict[str, object], work: WorkLayout):
    records: dict[str, AttemptRecord] = picks["records"]  # type: ignore[assignment]
    for attempt, record in records.items():
        assert record.cleanup is not None and record.cleanup["containers_listed_after"] == []
        root = Path(work.attempt(attempt).root)
        for gone in ("clone", "cache", "home", "tmp"):
            assert not (root / gone).exists(), (attempt, gone)
        assert (root / "attempt.json").exists()
        assert (root / "logs").is_dir()
        assert list(root.glob("suite-2.*"))
        assert (root / "scans").is_dir()
        assert list((root / "scans").glob("*/moved/scan.json"))
        assert not list((root / "scans").glob("*/r*/tmp"))


def test_the_machine_stops_and_the_host_docker_and_ssh_configuration_stay_as_found(picks: dict[str, object]):
    assert picks["status_after"] == "Stopped"
    assert picks["processes_after"] == []
    before, after = picks["context"]  # type: ignore[misc]
    assert before == after
    ssh_before, ssh_after = picks["ssh"]  # type: ignore[misc]
    assert ssh_before == ssh_after


def test_the_disk_cap_ends_the_stage_on_a_lowered_cap(
    picks: dict[str, object], work: WorkLayout, wiring: dict[str, object]
):
    bare: Path = picks["meter_bare"]  # type: ignore[assignment]
    origins = {"meter-dev/meter": place_origin(work, "A-go-02", bare)}
    lists = Lists(
        b={Ecosystem.PYTEST: [], Ecosystem.GO: []},
        a={Ecosystem.PYTEST: [], Ecosystem.GO: [meter_entry(2, str(picks["meter_base"]))]},
    )
    (record,) = run_attempts(work, wiring, lists, origins, cap=TEST_DISK_CAP_BYTES)
    assert record.outcome == "setup-disk"
    assert record.stopped is not None and record.stopped.startswith("setup-disk at ")
    root = Path(work.attempt("A-go-02").root)
    assert not (root / "clone").exists() and not (root / "cache").exists()
    assert (root / "attempt.json").exists()


def test_oom_killed_and_the_wall_time_limit_in_a_real_container(work: WorkLayout, wiring: dict[str, object]):
    files: LocalFiles = wiring["files"]  # type: ignore[assignment]
    engine: DockerEngine = wiring["engine"]  # type: ignore[assignment]
    clock: SystemClock = wiring["clock"]  # type: ignore[assignment]
    machine: ColimaMachine = wiring["machine"]  # type: ignore[assignment]
    image = files.read_json(work.image_build)
    assert isinstance(image, dict)
    directory = Path(work.root) / "limits"
    directory.mkdir(parents=True, exist_ok=True)

    def plan(name: str, argv: list[str], memory: str, limit: int | None) -> CommandPlan:
        def planned(run: int) -> PlannedRun:
            prefix = PurePosixPath(directory / f"{name}-r{run}")
            command = ContainerCommand(
                name=f"demos-1-limits-{name}-r{run}",
                image=str(image["image_id"]),
                attempt_dir=str(directory),
                binaries_dir=str(work.product_bin),
                workdir=str(directory),
                network=False,
                environment={"PATH": "/usr/bin:/bin", "TMPDIR": str(directory)},
                argv=argv,
                record_prefix=str(prefix),
                limit_seconds=limit,
                uid=os.getuid(),
                gid=os.getgid(),
                memory=memory,
            )
            return PlannedRun(command, (), prefix)

        return CommandPlan(1, None, planned)

    runner = CommandRunner(engine, files, clock, RefusalBudget(), lambda: None)
    oom, slow = CommandRecord(), CommandRecord()
    with machine_running(machine, work.mounts()):
        assert visible_in_machine(Path(work.root))
        assert visible_in_machine(Path(work.product_bin))
        assert not visible_in_machine(REPO_ROOT)
        assert not visible_in_machine(Path.home() / ".ssh")
        runner.settle(plan("oom", ["/bin/sh", "-c", "head -c 400M /dev/zero | tail"], OOM_MEMORY, None), oom)
        runner.settle(plan("slow", ["sleep", "5"], "2560m", 1), slow)
        left = engine.names_with_prefix("demos-1-limits-")
    (oom_run,) = oom.runs
    assert oom_run.oom_killed is True
    assert oom_run.memory_peak_bytes is not None and oom_run.memory_peak_bytes > 32 << 20
    (slow_run,) = slow.runs
    assert slow_run.timed_out is True and slow_run.exit_code == 137
    assert left == []
    assert machine.status() == "Stopped"
    assert machine_processes() == []
