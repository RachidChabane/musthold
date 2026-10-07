from __future__ import annotations

from collections.abc import Callable, Collection, Mapping, Sequence
from dataclasses import dataclass, field

from calibration.application.ports import MemoryReading
from calibration.application.safety_log import (
    EVENT_FIT,
    EVENT_GUARD,
    EVENT_PRESSURE_STOP,
    EVENT_SHUTDOWN,
    EVENT_WINDOW,
    SafetyLog,
)
from calibration.application.session import RunSession, SessionDeps
from calibration.domain.constants import (
    DEFAULT_RUN_WINDOW,
    DEVIATIONS_FILE,
    EXIT_FAILURE,
    EXIT_GUARD_INTERRUPTED,
    EXIT_GUARD_REFUSED,
    EXIT_OK,
    EXIT_PAUSED,
    MEMORY_DEFERRAL_LIMIT,
)
from calibration.domain.models import BLOCK_ORDER, MODEL_IDS, model_by_id
from calibration.domain.schedule import Pass
from calibration.domain.window import WINDOW_CLOSED_TEXT, RunWindow
from support.application_fakes import (
    EVENING,
    MORNING,
    ROOMY_MEMORY,
    TIGHT_MEMORY,
    UNCLEAN_SHUTDOWN,
    FakeMemoryProbe,
    FakeModelServer,
    FakeServerFactory,
    ManualClock,
    MemoryRunStore,
    RecordingLock,
    ScriptedLauncher,
)
from support.run_directory_seed import lock_record, request_ids, seed_responses, seed_run

RUN_WINDOW = RunWindow.parse(DEFAULT_RUN_WINDOW)
WINDOW_CLOSED_LINE = f"- pause | scope: none | {WINDOW_CLOSED_TEXT}"
ONLY_M3 = ("M1", "M2", "M4", "M5", "M6")


@dataclass
class Night:
    """One run directory over which sessions are held; each `session` call is a new session."""

    blocked: Collection[str] = ()
    store: MemoryRunStore = field(default_factory=MemoryRunStore)
    clock: ManualClock = field(default_factory=ManualClock)
    events: list[str] = field(default_factory=list[str])
    servers: FakeServerFactory = field(init=False)

    def __post_init__(self) -> None:
        self.servers = FakeServerFactory(self.events)
        seed_run(self.store, pass1=request_ids(2), pass2=[], lock=lock_record(blocked=self.blocked))

    def session(
        self,
        codes: Mapping[str, int] | None = None,
        readings: Sequence[MemoryReading] = (ROOMY_MEMORY,),
        window: RunWindow | None = None,
        after_launch: Callable[[str], None] | None = None,
    ) -> tuple[int, ScriptedLauncher, FakeMemoryProbe]:
        launcher = ScriptedLauncher(self.events, codes, after_launch)
        memory = FakeMemoryProbe(readings)
        deps = SessionDeps(
            store=self.store,
            clock=self.clock,
            memory=memory,
            launcher=launcher,
            servers=self.servers,
            lock=RecordingLock(self.events),
            window=window,
        )
        return RunSession(deps).run(), launcher, memory

    def deviation_lines(self) -> list[str]:
        return [line for line in self.store.text_of(DEVIATIONS_FILE).splitlines() if line.startswith("- ")]

    def safety(self, event: str) -> list[Mapping[str, object]]:
        return [e for e in SafetyLog(self.store).events() if e["event"] == event]


def test_the_blocks_run_in_the_order_m3_m2_m1_m4_m5_m6() -> None:
    night = Night()

    code, launcher, _ = night.session()

    assert code == EXIT_OK
    assert launcher.launched == ["M3", "M2", "M1", "M4", "M5", "M6"]
    assert tuple(launcher.launched) == BLOCK_ORDER


def test_a_model_the_lock_blocks_is_skipped() -> None:
    night = Night(blocked=["M2", "M5"])

    _, launcher, _ = night.session()

    assert launcher.launched == ["M3", "M1", "M4", "M6"]


def test_a_model_with_a_blocked_entry_is_skipped() -> None:
    night = Night()
    night.store.write_text(DEVIATIONS_FILE, "# Deviations\n\n- blocked | scope: M1, M4 | memory-pressure: 2 stops\n")

    _, launcher, _ = night.session()

    assert launcher.launched == ["M3", "M2", "M5", "M6"]


def test_a_finished_model_is_skipped() -> None:
    night = Night()
    first = request_ids(2)
    seed_responses(night.store, "M3", Pass.ONE, first)

    _, launcher, _ = night.session()

    assert launcher.launched == ["M2", "M1", "M4", "M5", "M6"]


def test_a_model_with_one_request_left_is_launched() -> None:
    night = Night()
    seed_responses(night.store, "M3", Pass.ONE, request_ids(2)[:1])

    _, launcher, _ = night.session()

    assert launcher.launched[0] == "M3"


def test_a_session_over_only_finished_or_blocked_models_launches_nothing_and_succeeds() -> None:
    night = Night(blocked=["M1", "M2", "M4", "M5", "M6"])
    seed_responses(night.store, "M3", Pass.ONE, request_ids(2))

    code, launcher, _ = night.session()

    assert (code, launcher.launched) == (EXIT_OK, [])


def test_the_lock_is_held_from_before_the_first_launch_to_after_the_last_cleanup() -> None:
    night = Night(blocked=ONLY_M3)

    night.session()

    assert night.events == ["lock-enter", "launch:M3", "stop:M3", "lock-exit"]


def test_the_lock_is_held_across_every_block_of_the_session() -> None:
    night = Night()

    night.session()

    assert night.events[0] == "lock-enter"
    assert night.events[-1] == "lock-exit"
    assert night.events.count("lock-enter") == night.events.count("lock-exit") == 1


def test_each_launch_is_followed_by_the_cleanup_of_that_models_server() -> None:
    night = Night()

    night.session()

    expected = [event for model_id in BLOCK_ORDER for event in (f"launch:{model_id}", f"stop:{model_id}")]
    assert night.events[1:-1] == expected
    assert night.servers.requested == list(BLOCK_ORDER)


def test_the_cleanup_stops_the_server_of_the_model_after_a_guard_stop_too() -> None:
    night = Night(blocked=ONLY_M3)

    night.session(codes={"M3": EXIT_GUARD_INTERRUPTED})

    assert night.servers.servers["M3"].stops == 1


def test_the_cleanup_is_recorded_as_the_sessions_shutdown() -> None:
    night = Night(blocked=ONLY_M3)

    night.session()

    (event,) = night.safety(EVENT_SHUTDOWN)
    assert (event["by"], event["model"], event["port_free"]) == ("session", "M3", True)
    assert not night.store.exists(DEVIATIONS_FILE)


def test_a_cleanup_that_finds_the_server_not_stopped_is_a_protocol_entry_for_the_model() -> None:
    night = Night(blocked=ONLY_M3)
    night.servers.install(FakeModelServer("M3", model_by_id("M3").flavour, night.events, shutdown=UNCLEAN_SHUTDOWN))

    night.session()

    assert night.deviation_lines() == [
        "- protocol | scope: M3 | the cleanup after the block found the server not stopped"
    ]


def test_a_session_outside_the_window_launches_nothing_and_pauses() -> None:
    night = Night(clock=ManualClock(MORNING))

    code, launcher, _ = night.session(window=RUN_WINDOW)

    assert (code, launcher.launched) == (EXIT_PAUSED, [])
    assert night.deviation_lines() == [WINDOW_CLOSED_LINE]
    assert [event["open"] for event in night.safety(EVENT_WINDOW)] == [False]


def test_a_session_stops_at_the_window_close_between_two_blocks() -> None:
    night = Night()

    def close_after_m2(model_id: str) -> None:
        if model_id == "M2":
            night.clock.current = MORNING

    code, launcher, _ = night.session(window=RUN_WINDOW, after_launch=close_after_m2)

    assert code == EXIT_PAUSED
    assert launcher.launched == ["M3", "M2"]
    assert night.deviation_lines() == [WINDOW_CLOSED_LINE]


def test_a_session_inside_the_window_runs_every_block() -> None:
    night = Night(clock=ManualClock(EVENING))

    code, launcher, _ = night.session(window=RUN_WINDOW)

    assert (code, len(launcher.launched)) == (EXIT_OK, len(MODEL_IDS))


def test_enough_free_memory_loads_the_model_and_records_the_fit_check() -> None:
    night = Night(blocked=ONLY_M3)

    night.session()

    (check,) = night.safety(EVENT_FIT)
    assert check["decision"] == "load"
    assert check["model"] == "M3"
    assert check["free_percent"] == ROOMY_MEMORY.free_percent
    assert check["deferrals"] == 0


def test_too_little_free_memory_defers_the_block_to_the_next_session() -> None:
    night = Night(blocked=ONLY_M3)

    code, launcher, _ = night.session(readings=[TIGHT_MEMORY])

    assert launcher.launched == []
    assert code == EXIT_OK
    assert night.deviation_lines() == ["- pause | scope: none | M3 deferred: free memory is below F + 0.15 T"]
    (check,) = night.safety(EVENT_FIT)
    assert (check["decision"], check["deferrals"]) == ("defer", 1)


def test_a_deferred_model_does_not_hold_back_the_others() -> None:
    night = Night(blocked=["M1", "M2", "M4", "M5"])

    _, launcher, memory = night.session(readings=[TIGHT_MEMORY, ROOMY_MEMORY])

    assert launcher.launched == ["M6"]
    assert memory.calls == 2


def test_the_deferrals_of_a_model_accumulate_across_sessions() -> None:
    night = Night(blocked=ONLY_M3)

    for _ in range(MEMORY_DEFERRAL_LIMIT - 1):
        night.session(readings=[TIGHT_MEMORY])

    assert [check["deferrals"] for check in night.safety(EVENT_FIT)] == [1, 2, 3, 4]
    assert not any(line.startswith("- blocked") for line in night.deviation_lines())


def test_the_fifth_deferral_blocks_the_model_for_memory_fit() -> None:
    night = Night(blocked=ONLY_M3)

    for _ in range(MEMORY_DEFERRAL_LIMIT):
        night.session(readings=[TIGHT_MEMORY])

    assert night.deviation_lines()[-1] == "- blocked | scope: M3 | memory-fit: deferred in 5 sessions"
    assert night.safety(EVENT_FIT)[-1]["decision"] == "block-memory-fit"


def test_a_model_blocked_for_memory_fit_is_skipped_by_the_next_session() -> None:
    night = Night(blocked=ONLY_M3)
    for _ in range(MEMORY_DEFERRAL_LIMIT):
        night.session(readings=[TIGHT_MEMORY])

    _, launcher, memory = night.session()

    assert launcher.launched == []
    assert memory.calls == 0


def test_a_launch_between_deferrals_does_not_reset_the_count() -> None:
    night = Night(blocked=ONLY_M3)
    for _ in range(MEMORY_DEFERRAL_LIMIT - 1):
        night.session(readings=[TIGHT_MEMORY])

    night.session(readings=[ROOMY_MEMORY], codes={"M3": EXIT_PAUSED})
    night.session(readings=[TIGHT_MEMORY])

    assert night.deviation_lines()[-1].startswith("- blocked | scope: M3 | memory-fit")


def test_a_launch_that_ends_with_exit_3_pauses_the_session_without_launching_the_rest() -> None:
    night = Night()

    code, launcher, _ = night.session(codes={"M2": EXIT_PAUSED})

    assert code == EXIT_PAUSED
    assert launcher.launched == ["M3", "M2"]


def test_a_guard_refusal_is_logged_and_pauses_the_session() -> None:
    night = Night()

    code, launcher, _ = night.session(codes={"M3": EXIT_GUARD_REFUSED}, readings=[ROOMY_MEMORY])

    assert (code, launcher.launched) == (EXIT_PAUSED, ["M3"])
    (reading,) = night.safety(EVENT_GUARD)
    assert (reading["model"], reading["exit_code"]) == ("M3", EXIT_GUARD_REFUSED)
    assert reading["pressure_normal"] is True
    assert night.deviation_lines() == ["- pause | scope: none | the block of M3 stopped with exit code 4"]


def test_a_guard_refusal_does_not_block_the_model() -> None:
    night = Night(blocked=ONLY_M3)

    night.session(codes={"M3": EXIT_GUARD_REFUSED})
    _, launcher, _ = night.session()

    assert launcher.launched == ["M3"]


def test_a_first_memory_pressure_stop_pauses_the_session_without_blocking_the_model() -> None:
    night = Night()

    code, launcher, _ = night.session(codes={"M3": EXIT_GUARD_INTERRUPTED})

    assert (code, launcher.launched) == (EXIT_PAUSED, ["M3"])
    (stop,) = night.safety(EVENT_PRESSURE_STOP)
    assert (stop["model"], stop["stops"]) == ("M3", 1)
    assert night.deviation_lines() == ["- pause | scope: none | memory pressure stopped the block of M3"]


def test_a_second_memory_pressure_stop_of_the_same_model_blocks_it() -> None:
    night = Night(blocked=ONLY_M3)

    night.session(codes={"M3": EXIT_GUARD_INTERRUPTED})
    code, _, _ = night.session(codes={"M3": EXIT_GUARD_INTERRUPTED})

    assert code == EXIT_PAUSED
    assert [stop["stops"] for stop in night.safety(EVENT_PRESSURE_STOP)] == [1, 2]
    assert night.deviation_lines()[-1] == "- blocked | scope: M3 | memory-pressure: 2 stops"


def test_a_model_blocked_by_pressure_is_not_launched_again() -> None:
    night = Night(blocked=ONLY_M3)
    night.session(codes={"M3": EXIT_GUARD_INTERRUPTED})
    night.session(codes={"M3": EXIT_GUARD_INTERRUPTED})

    _, launcher, _ = night.session()

    assert launcher.launched == []


def test_pressure_stops_of_different_models_are_counted_apart() -> None:
    night = Night(blocked=["M1", "M4", "M5", "M6"])

    night.session(codes={"M3": EXIT_GUARD_INTERRUPTED})
    night.session(codes={"M3": EXIT_OK, "M2": EXIT_GUARD_INTERRUPTED})

    assert not any(line.startswith("- blocked") for line in night.deviation_lines())


def test_any_other_failing_exit_code_lets_the_session_go_on_and_ends_it_in_failure() -> None:
    night = Night()

    code, launcher, _ = night.session(codes={"M2": EXIT_FAILURE})

    assert code == EXIT_FAILURE
    assert launcher.launched == list(BLOCK_ORDER)
    assert not night.store.exists(DEVIATIONS_FILE)
