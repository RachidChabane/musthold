from __future__ import annotations

import io
import json
import tarfile
from collections.abc import Mapping, Sequence
from pathlib import Path

from specbench.adapters.area_environment import AreaEnvironment
from specbench.adapters.area_preparer import AreaPreparer
from specbench.adapters.ci_round_archive import CiRoundArchive
from specbench.adapters.clean_room_repository import CleanRoomRepository
from specbench.adapters.execution_counter import ClaudeExecutionCounter
from specbench.adapters.gate_session_driver import GateSessionDriver
from specbench.adapters.isolated_ci_round_trip import ARTIFACTS_DIRECTORY, ARTIFACTS_ENV, IsolatedCiRoundTrip
from specbench.adapters.isolated_host_config import IsolatedHostConfig
from specbench.adapters.product_cli import ProductCli
from specbench.adapters.session_state import SessionState, SessionStateFile
from specbench.adapters.shallnot_cli import ShallnotCli
from specbench.application.ports import EventSink, GateSessionEnd, GateSessionSpec, ProcessResult
from specbench.domain.arm_policy import policy_for
from specbench.domain.arms import Arm
from specbench.domain.canonical import JsonValue
from specbench.domain.ci_round import CiRoundRequest
from specbench.domain.clean_room import RoomArea
from specbench.domain.gate import GateStatus
from specbench.domain.manifest import CALL_EVENTS, RunLayout
from specbench.domain.product import PROOF_MODULES
from specbench.domain.sessions import ci_round_call_id
from support.credentials import StaticCredentialSource

BINARIES = Path("/tools/bin")
DP = PROOF_MODULES[0]
HEAD = "c" * 40
RUN_ID = "bench-0123456789abcdef"
SESSION_ID = "s-1"
SHIM = "/repo/bench/script/gh-shim"


class NoChecks:
    def problems(self, schema: JsonValue, document: JsonValue) -> list[str]:
        return []


class ProductRunner:
    """Answers the git and module commands of a round trip: a clean or dirty tree, and a push that publishes or not."""

    def __init__(self, dirty: bool = False, push_publishes: bool = True) -> None:
        self.dirty = dirty
        self.push_publishes = push_publishes
        self.pulls: list[tuple[list[str], Mapping[str, str] | None]] = []

    def run(self, argv: Sequence[str], cwd: Path | None = None, env: Mapping[str, str] | None = None) -> ProcessResult:
        words = list(argv)
        if words[0] == "git":
            return self._git(words[3:])
        if words[1:3] == ["host", "pull"]:
            self.pulls.append((words, env))
            return ProcessResult(0, "", "")
        if words[1:3] == ["host", "push"]:
            return ProcessResult(0 if self.push_publishes else 1, "", "")
        return ProcessResult(0, "", "")

    def _git(self, arguments: list[str]) -> ProcessResult:
        match arguments[0]:
            case "status":
                return ProcessResult(0, " M shallnot.yaml\n" if self.dirty else "", "")
            case "rev-parse":
                return ProcessResult(0, f"{HEAD}\n", "")
            case "archive":
                self._write_archive(Path(arguments[arguments.index("-o") + 1]))
                return ProcessResult(0, "", "")
            case _:
                return ProcessResult(0, "", "")

    def _write_archive(self, destination: Path) -> None:
        manifest = json.dumps({"headSha": HEAD, "responses": [{"requestId": "r1", "vote": 0}]}).encode()
        with tarfile.open(destination, "w") as archive:
            info = tarfile.TarInfo("manifest.json")
            info.size = len(manifest)
            archive.addfile(info, io.BytesIO(manifest))


class RecordingSession:
    def __init__(self, finishes: bool = True) -> None:
        self.finishes = finishes
        self.specs: list[GateSessionSpec] = []

    def run(self, spec: GateSessionSpec, on_event: EventSink) -> GateSessionEnd:
        self.specs.append(spec)
        init: dict[str, object] = {
            "type": "system",
            "subtype": "init",
            "session_id": SESSION_ID,
            "plugins": [],
            "mcp_servers": [],
        }
        plugins = [{"name": "spec-proof"}, {"name": DP.plugin}]
        on_event(json.dumps({**init, "plugins": plugins, "mcp_servers": [{"name": DP.server, "status": "connected"}]}))
        result = {"type": "result", "subtype": "success", "is_error": False, "session_id": SESSION_ID, "result": "done"}
        if self.finishes:
            on_event(json.dumps(result))
        return GateSessionEnd(0 if self.finishes else 1)


class Rig:
    def __init__(self, tmp_path: Path, runner: ProductRunner, session: RecordingSession, token: str | None) -> None:
        self.runner = runner
        self.session = session
        self.config = IsolatedHostConfig(Path("/repo"), BINARIES, Path("/u"), tmp_path / "rooms")
        self.layout = RunLayout(tmp_path / "run")
        self.area = RoomArea(tmp_path / "rooms" / "session")
        self.area.tree.mkdir(parents=True)
        self.area.ready_marker.write_text("ready\n")
        self.record = tmp_path / "rec"
        self.issued = tmp_path / "issued"
        (self.issued / DP.host_ref_name / "requests").mkdir(parents=True)
        (self.issued / DP.host_ref_name / "issued.json").write_text("{}")
        cli = ProductCli(runner, BINARIES)
        repository = CleanRoomRepository(runner)
        environment = AreaEnvironment(BINARIES, (), {"PATH": "/usr/bin"})
        preparer = AreaPreparer(cli, ShallnotCli(runner, BINARIES / "shallnot", NoChecks()), repository, environment)
        self.trip = IsolatedCiRoundTrip(
            self.config,
            self.layout,
            GateSessionDriver(session),
            preparer,
            environment,
            cli,
            repository,
            ClaudeExecutionCounter(),
            StaticCredentialSource(token),
            {"PATH": "/usr/bin"},
        )

    def request(self, number: int = 1) -> CiRoundRequest:
        return CiRoundRequest(
            self.area.root,
            Arm.DP,
            policy_for(Arm.DP),
            (DP,),
            (DP,),
            self.area,
            self.record,
            RUN_ID,
            self.issued,
            number,
        )


def rig(tmp_path: Path, runner: ProductRunner | None = None, finishes: bool = True, token: str | None = "tok") -> Rig:
    return Rig(tmp_path, runner or ProductRunner(), RecordingSession(finishes), token)


def test_a_round_without_a_credential_never_pulls_nor_starts_a_session(tmp_path: Path) -> None:
    r = rig(tmp_path, token=None)
    result = r.trip.run(r.request())
    assert result.status is GateStatus.HARNESS_MISCONFIGURED
    assert (r.runner.pulls, r.session.specs) == ([], [])


def test_a_dirty_tracked_session_tree_is_misconfigured_before_anything_is_pulled(tmp_path: Path) -> None:
    r = rig(tmp_path, ProductRunner(dirty=True))
    result = r.trip.run(r.request())
    assert result.status is GateStatus.HARNESS_MISCONFIGURED
    assert (r.runner.pulls, r.session.specs) == ([], [])
    assert CiRoundArchive(r.record).rounds() == 0


def test_the_pull_sees_the_staged_artifact_through_the_shim_and_the_session_never_does(tmp_path: Path) -> None:
    r = rig(tmp_path)
    r.trip.run(r.request())
    [(argv, pull_env)] = r.runner.pulls
    assert argv[3:5] == ["--ci-run", RUN_ID]
    assert pull_env is not None
    assert pull_env["PATH"].startswith(SHIM + ":")
    staged = Path(pull_env[ARTIFACTS_ENV]) / RUN_ID / "spec-proof-host-discpower"
    assert (staged / "issued.json").is_file()
    assert Path(pull_env[ARTIFACTS_ENV]) == r.area.root / ARTIFACTS_DIRECTORY
    [spec] = r.session.specs
    assert SHIM not in spec.env["PATH"]
    assert ARTIFACTS_ENV not in spec.env


def test_a_round_resumes_the_session_of_the_stop_and_archives_the_pushed_ref_as_its_own_round(tmp_path: Path) -> None:
    r = rig(tmp_path)
    SessionStateFile(r.area).write(SessionState("stop-session", finished=True))
    result = r.trip.run(r.request(number=2))
    [spec] = r.session.specs
    assert spec.argv[spec.argv.index("--resume") + 1] == "stop-session"
    assert result.status is GateStatus.COMPLETE
    assert result.session_id == SESSION_ID
    assert (r.record / "ci" / "2" / "discpower" / "manifest.json").is_file()
    events = r.layout.call(ci_round_call_id(Arm.DP, r.record, 2)) / CALL_EVENTS
    assert len(events.read_text().splitlines()) == 2
    assert SessionStateFile(r.area).read() == SessionState(SESSION_ID, finished=True)


def test_a_session_that_does_not_finish_archives_nothing(tmp_path: Path) -> None:
    r = rig(tmp_path, finishes=False)
    result = r.trip.run(r.request())
    assert result.status is GateStatus.PAUSED
    assert CiRoundArchive(r.record).rounds() == 0


def test_a_push_that_publishes_nothing_leaves_the_round_unarchived(tmp_path: Path) -> None:
    r = rig(tmp_path, ProductRunner(push_publishes=False))
    result = r.trip.run(r.request())
    assert result.status is GateStatus.PAUSED
    assert CiRoundArchive(r.record).rounds() == 0
