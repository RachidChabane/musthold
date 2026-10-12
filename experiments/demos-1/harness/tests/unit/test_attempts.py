"""The attempt use case on scripted containers: stage order, outcomes, scans, replays, disk cap, cleanup, resume."""

from pathlib import Path, PurePosixPath

import pytest

from demos1.application.attempts import Target
from demos1.application.ports import HostRun, OperatorStopped
from demos1.domain.container import ContainerCommand
from demos1.domain.ecosystem import Ecosystem
from demos1.domain.layout import WorkLayout
from demos1.domain.lists import ListName, Lists, Stratum
from support.attempt_flow import SHORT, PickScript, a_entry, attempts_for, b_entry, root_of
from support.fakes import Played


@pytest.fixture
def work(tmp_path: Path) -> WorkLayout:
    return WorkLayout(PurePosixPath(tmp_path / "work"), PurePosixPath(tmp_path / "product"))


def test_a_pick_runs_every_stage_then_its_scans_and_replays(work: WorkLayout):
    attempts, engine, _ = attempts_for(work, PickScript(root_of(work)))
    record = attempts.run_one(Target(ListName(Stratum.B, Ecosystem.PYTEST), b_entry()))
    runs = [name for kind, name in engine.calls if kind == "run"]
    assert runs == [
        "B-pytest-01-clone-r1",
        "B-pytest-01-toolchain-r1",
        "B-pytest-01-install-r1",
        "B-pytest-01-clean-tree-1-r1",
        "B-pytest-01-suite-1-r1",
        "B-pytest-01-suite-2-r1",
        "B-pytest-01-clean-tree-2-r1",
        "B-pytest-01-change-r1",
        "B-pytest-01-change-replay-1-r1",
        "B-pytest-01-change-replay-2-r1",
        "B-pytest-01-quick-r1",
        "B-pytest-01-quick-replay-1-r1",
        "B-pytest-01-quick-replay-2-r1",
        "B-pytest-01-change-full-r1",
        "B-pytest-01-change-full-replay-1-r1",
        "B-pytest-01-change-full-replay-2-r1",
    ]
    assert record.outcome == "pick"
    toolchain = record.stage("toolchain").details
    assert (toolchain["python_request"], toolchain["python_resolved"]) == ("3.12", "3.12.11")
    clone = engine.commands[0]
    assert clone.network and clone.environment["GIT_ALLOW_PROTOCOL"] == "https"
    assert all(not command.network for command in engine.commands[3:])
    install = engine.commands[2]
    assert install.argv[-1].splitlines()[1] == "uv venv .venv --python 3.12"
    change = record.scan("change")
    assert change.outcome == "scan-ok"
    assert change.statement is not None and change.statement.sampled_green == 30
    assert change.headline is not None and change.headline["depth"] == "full"
    first, second = change.findings
    assert (first.replay_result, second.replay_result) == ("holds on replay", "no longer holds on replay")
    assert (first.pull_request_status, second.pull_request_status) == ("added", None)
    assert (first.site_file, first.site_line) == ("ledger/totals.py", 41)
    scans_dir = Path(work.attempt("B-pytest-01").root) / "scans" / "change" / "moved"
    assert (scans_dir / "scan.json").exists()
    assert engine.containers == {}
    assert record.cleanup is not None and record.cleanup["containers_listed"] == []


def test_the_end_of_an_attempt_deletes_the_scratch_and_keeps_the_records(work: WorkLayout):
    attempts, _, _ = attempts_for(work, PickScript(root_of(work)))
    attempts.run_one(Target(ListName(Stratum.B, Ecosystem.PYTEST), b_entry()))
    root = Path(work.attempt("B-pytest-01").root)
    for gone in ("clone", "cache", "home", "tmp"):
        assert not (root / gone).exists(), gone
    for kept in ("attempt.json", "logs", "suite-1.xml", "suite-2.xml", "scans/change/moved/1/finding.json"):
        assert (root / kept).exists(), kept
    assert not list((root / "scans").glob("*/r*/tmp"))
    assert list((root / "scans").glob("*/r1/run.stdout"))


def test_a_failed_install_ends_the_attempt_there(work: WorkLayout):
    script = PickScript(root_of(work))
    script.overrides["install"] = Played(exit_code=1)
    attempts, engine, _ = attempts_for(work, script)
    record = attempts.run_one(Target(ListName(Stratum.B, Ecosystem.PYTEST), b_entry()))
    assert record.outcome == "setup-install"
    assert record.outcome_log is not None and record.outcome_log.endswith("install-r1.log")
    assert [name for kind, name in engine.calls if kind == "run"][-1] == "B-pytest-01-install-r1"
    assert record.stages_reached() == ["clone", "toolchain", "install"]


def test_a_slow_second_run_and_a_memory_limit(work: WorkLayout):
    script = PickScript(root_of(work))
    script.overrides["suite-2"] = Played(exit_code=137, timed_out=True, wall_seconds=60.0)
    attempts, _, _ = attempts_for(work, script)
    assert attempts.run_one(Target(ListName(Stratum.B, Ecosystem.PYTEST), b_entry())).outcome == "setup-suite-too-slow"
    script = PickScript(root_of(work))
    script.overrides["suite-1"] = Played(exit_code=0, oom_killed=True)
    attempts, _, _ = attempts_for(work, script)
    record = attempts.run_one(Target(ListName(Stratum.B, Ecosystem.PYTEST), b_entry(rank=2)))
    assert record.outcome == "setup-oom"


def test_a_dirty_tree_after_the_suites(work: WorkLayout):
    script = PickScript(root_of(work))
    script.overrides["clean-tree-2"] = Played(stdout=" M tests/snapshots/totals.txt\n")
    attempts, _, _ = attempts_for(work, script)
    record = attempts.run_one(Target(ListName(Stratum.B, Ecosystem.PYTEST), b_entry()))
    assert record.outcome == "setup-dirty-tree"
    assert record.stage("clean-tree-2").details["status_output"] == " M tests/snapshots/totals.txt\n"


def test_the_disk_cap_ends_a_setup_stage_as_setup_disk(work: WorkLayout):
    script = PickScript(root_of(work))
    script.overrides["install"] = Played(
        writes={str(Path(work.root) / "attempts/B-pytest-01/clone/.venv/blob"): b"0" * 300_000}
    )
    attempts, engine, _ = attempts_for(work, script, attempt_cap_bytes=200_000)
    record = attempts.run_one(Target(ListName(Stratum.B, Ecosystem.PYTEST), b_entry()))
    assert record.outcome == "setup-disk"
    assert record.stopped == "setup-disk at install"
    assert [name for kind, name in engine.calls if kind == "run"][-1] == "B-pytest-01-install-r1"
    assert not (Path(work.attempt("B-pytest-01").root) / "clone").exists()


def test_the_disk_cap_ends_a_scan_as_scan_disk_and_stops_the_attempt(work: WorkLayout):
    script = PickScript(root_of(work))
    big = str(Path(work.root) / "attempts/B-pytest-01/clone/.spec-proof/scan" / SHORT / "trace.bin")
    attempts, engine, _ = attempts_for(work, script, attempt_cap_bytes=400_000)
    original = script.__call__

    def with_big_change(command: ContainerCommand) -> Played:
        played = original(command)
        if command.name == "B-pytest-01-change-r1":
            played.writes[big] = b"0" * 500_000
        return played

    engine.script = with_big_change
    record = attempts.run_one(Target(ListName(Stratum.B, Ecosystem.PYTEST), b_entry()))
    assert record.outcome == "pick"
    assert record.scan("change").outcome == "scan-disk"
    assert record.stopped == "scan-disk at change"
    runs = [name for kind, name in engine.calls if kind == "run"]
    assert runs[-1] == "B-pytest-01-change-r1"
    assert all(scan.scan == "change" for scan in record.scans)


def test_an_interrupted_verb_resumes_at_the_command_it_stopped_at(work: WorkLayout):
    script = PickScript(root_of(work))
    attempts, engine, _ = attempts_for(work, script)

    def stop_at_quick(command: ContainerCommand) -> Played:
        if command.name == "B-pytest-01-quick-r1":
            raise OperatorStopped(HostRun(130, "", False, attempts.now(), attempts.now()))
        return script(command)

    engine.script = stop_at_quick
    with pytest.raises(OperatorStopped):
        attempts.run_one(Target(ListName(Stratum.B, Ecosystem.PYTEST), b_entry()))
    resumed, engine_again, _ = attempts_for(work, script)
    record = resumed.run_one(Target(ListName(Stratum.B, Ecosystem.PYTEST), b_entry()))
    runs = [name for kind, name in engine_again.calls if kind == "run"]
    assert runs[0] == "B-pytest-01-quick-r2"
    assert record.scan("quick").command.runs[0].stopped_by_operator
    assert record.outcome == "pick" and record.finished()


def test_attempts_stop_at_two_picks_and_reach_the_a_list_only_after_the_b_list(work: WorkLayout):
    lists = Lists(
        b={Ecosystem.PYTEST: [b_entry(1, "north/a"), b_entry(2, "north/b")], Ecosystem.GO: []},
        a={Ecosystem.PYTEST: [a_entry(1)], Ecosystem.GO: []},
    )
    script = PickScript(root_of(work))
    fail_first = {"B-pytest-01-install-r1"}

    def scripted(command: ContainerCommand) -> Played:
        if command.name in fail_first:
            return Played(exit_code=1)
        return script(command)

    attempts, _, _ = attempts_for(work, scripted)
    records = attempts.run_all(lists, picks_per_ecosystem=2)
    assert [(record.attempt, record.outcome) for record in records] == [
        ("B-pytest-01", "setup-install"),
        ("B-pytest-02", "pick"),
        ("A-pytest-01", "pick"),
    ]
    again, engine, _ = attempts_for(work, scripted)
    again.run_all(lists, picks_per_ecosystem=2)
    assert engine.calls == []


def test_a_go_pick_past_the_image_toolchain(work: WorkLayout):
    def script(command: ContainerCommand) -> Played:
        if command.name.endswith("clone-r1"):
            return Played(writes={str(Path(command.attempt_dir) / "clone/go.mod"): "module x\n\ngo 1.27\n"})
        return Played()

    attempts, engine, _ = attempts_for(work, script)
    record = attempts.run_one(Target(ListName(Stratum.A, Ecosystem.GO), a_entry()))
    assert record.outcome == "setup-toolchain"
    assert [name for kind, name in engine.calls if kind == "run"] == ["A-go-01-clone-r1"]
    assert "cat-file" not in engine.commands[0].argv[-1]
