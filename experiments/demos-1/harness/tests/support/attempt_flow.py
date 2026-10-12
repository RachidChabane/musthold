"""An attempt played to its end on scripted containers, for the use-case tests."""

from collections.abc import Callable
from pathlib import Path

from demos1.adapters.local import LocalFiles
from demos1.application.attempts import Attempts, AttemptSettings
from demos1.application.command_runner import CommandRunner, RefusalBudget
from demos1.domain.container import ContainerCommand
from demos1.domain.layout import WorkLayout
from demos1.domain.lists import AEntry, BEntry, PathEntry
from demos1.domain.machine import ATTEMPT_CAP_BYTES
from support.fakes import FakeClock, Played, RecordedDeviations, ScriptedEngine
from support.scans import BASE, COMMIT, HEADLINE, finding, statement

SHORT = COMMIT[:7]
JUNIT_PASSING = "<testsuite>" + "".join(f"<testcase name='t{index}'/>" for index in range(14)) + "</testsuite>"


def b_entry(rank: int = 1, name: str = "north/ledger") -> BEntry:
    return BEntry(
        rank=rank,
        full_name=name,
        stars=2400,
        licence="MIT",
        number=41,
        url=f"https://github.com/{name}/pull/41",
        signals=("copilot",),
        merged_at="2026-09-30T18:00:00Z",
        merge_commit=COMMIT,
        base=BASE,
        files=(
            PathEntry("ledger/totals.py", "modified", False),
            PathEntry("tests/test_totals.py", "added", True),
        ),
    )


def a_entry(rank: int = 1) -> AEntry:
    return AEntry(rank, "cedar/forms", 9000, "MIT", "main", COMMIT, "2026-09-30T11:00:00Z")


class PickScript:
    """Containers that play a pytest pick to its end, with overrides per stage."""

    def __init__(self, attempt_root: Callable[[ContainerCommand], Path]) -> None:
        self.root = attempt_root
        self.overrides: dict[str, Played] = {}
        self.replay_exit = {1: 0, 2: 1}

    def __call__(self, command: ContainerCommand) -> Played:
        root = self.root(command)
        stage = command.name.split("-", 3)[3].rsplit("-r", 1)[0]
        if stage in self.overrides:
            return self.overrides[stage]
        if "replay" in stage:
            return Played(exit_code=self.replay_exit[int(stage.rsplit("-", 1)[1])])
        play = self.plays(root).get(stage)
        return play() if play is not None else Played()

    def plays(self, root: Path) -> dict[str, Callable[[], Played]]:
        clone = root / "clone"
        scan = clone / ".spec-proof" / "scan" / SHORT

        def scanned() -> Played:
            return Played(
                stdout=HEADLINE.replace("1a2b3c4", SHORT),
                writes={
                    str(scan / "scan.json"): statement(),
                    str(scan / "report.md"): "## spec-proof scan\n",
                    str(scan / "1" / "finding.json"): finding("tests/test_totals.py::test_total_accepts_any_rate"),
                    str(scan / "2" / "finding.json"): finding("tests/test_rates.py::test_rate[high value]"),
                },
            )

        def suite(run: int) -> Callable[[], Played]:
            return lambda: Played(exit_code=0, writes={str(root / f"suite-{run}.xml"): JUNIT_PASSING})

        return {
            "clone": lambda: Played(
                writes={str(clone / "pytest.ini"): "[pytest]\n", str(clone / ".python-version"): "3.12\n"}
            ),
            "toolchain": lambda: Played(
                writes={str(root / "cache/uv/python/cpython-3.12.11-linux-aarch64-gnu/bin/python"): "x"}
            ),
            "suite-1": suite(1),
            "suite-2": suite(2),
            "change": scanned,
            "quick": scanned,
            "change-full": scanned,
            "full": scanned,
        }


def attempts_for(
    work: WorkLayout, script: Callable[[ContainerCommand], Played], attempt_cap_bytes: int = ATTEMPT_CAP_BYTES
) -> tuple[Attempts, ScriptedEngine, RecordedDeviations]:
    engine = ScriptedEngine(script)
    files = LocalFiles()
    clock = FakeClock()
    deviations = RecordedDeviations()
    refusals = RefusalBudget()

    def runner_of(persist: Callable[[], None]) -> CommandRunner:
        return CommandRunner(engine, files, clock, refusals, persist)

    configured = AttemptSettings(
        work=work,
        image="sha256:image",
        binaries_dir=str(work.product_bin),
        uid=502,
        gid=20,
        clone_url=lambda name: f"https://github.com/{name}.git",
        clone_protocol="https",
        uv_releases={"3.13": "3.13.7", "3.12": "3.12.11"},
        environment={"facts": {}},
        min_free_bytes=0,
        attempt_cap_bytes=attempt_cap_bytes,
    )
    return Attempts(configured, engine, files, clock, deviations, runner_of, refusals), engine, deviations


def root_of(work: WorkLayout) -> Callable[[ContainerCommand], Path]:
    return lambda command: Path(command.attempt_dir)
