from __future__ import annotations

import json
import math
from dataclasses import replace
from datetime import datetime
from pathlib import Path

import pytest

from specbench.adapters.claude_cli import (
    ClaudeCliModel,
    claude_argv,
    isolated_environment,
    schema_argument,
)
from specbench.adapters.process import SubprocessPromptRunner
from specbench.adapters.run_store import FAILURES_FILE_NAME, FileCallStore, FileFailureLog
from specbench.application.paced_calls import CallGate, PacedConstructionModel
from specbench.domain.arms import Harness
from specbench.domain.canonical import canonical_json
from specbench.domain.construction import (
    BASELINE_INPUT_TOKEN_CEILING,
    BASELINE_PROMPT,
    HAIKU,
    OPUS,
    SONNET,
    IsolationViolation,
    ModelCallFailed,
    ModelRequest,
    QuotaExhausted,
    Role,
    model_matches_pin,
)
from specbench.domain.manifest import RunLayout
from specbench.domain.pacing import PacingPolicy, PauseReason, RunPaused, RunWindow
from specbench.domain.sessions import CallRecord
from support.fake_claude import FAKE_BASELINE_INPUT_TOKENS, install_fake_claude, recorded_baseline_calls, recorded_calls
from support.fakes import FixedClock, MemoryCalls, MemoryLedger

pytestmark = pytest.mark.integration

SCHEMA = {
    "$schema": "https://json-schema.org/draft/2020-12/schema",
    "type": "object",
    "required": ["ok"],
    "properties": {"ok": {"type": "boolean"}},
}
NIGHT = datetime(2026, 10, 8, 23, 0)
DAY = datetime(2026, 10, 8, 12, 0)
TONIGHT = NIGHT.date()
BASELINE = FAKE_BASELINE_INPUT_TOKENS


def request(role: Role = Role.SPEC_AUTHOR, key: str = "T:spec") -> ModelRequest:
    return ModelRequest(role, key, "You write specs.", "Write the spec.", SCHEMA)


def model(directory: Path, responses: list[dict[str, object]]) -> ClaudeCliModel:
    executable = install_fake_claude(directory, responses)  # type: ignore[arg-type]
    return ClaudeCliModel(SubprocessPromptRunner(lambda: 100), str(executable))


def test_each_role_is_pinned_to_its_model() -> None:
    assert request(Role.SPEC_AUTHOR).model == OPUS
    assert request(Role.REVIEWER).model == HAIKU
    assert request(Role.LEAK_PROBE).model == SONNET
    assert request(Role.H_WRITER).model == OPUS
    assert {request(role).model for role in (Role.ASSIGNER_OPUS, Role.ASSIGNER_HAIKU)} == {OPUS, HAIKU}


def test_the_call_is_isolated_as_the_pilot_adapter_isolates_it(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("CLAUDECODE", "1")
    monkeypatch.setenv("CLAUDE_CODE_ENTRYPOINT", "cli")
    claude = model(tmp_path, [{"payload": {"ok": True}}])
    response = claude.call(request(), BASELINE)
    assert response.payload == {"ok": True} and response.reported_models == (OPUS,)
    (call,) = recorded_calls(tmp_path)
    argv = call["argv"]
    for flag in ("-p", "--strict-mcp-config", "--disable-slash-commands", "--no-session-persistence"):
        assert flag in argv
    assert argv[argv.index("--tools") + 1] == "" and argv[argv.index("--setting-sources") + 1] == ""
    assert argv[argv.index("--output-format") + 1] == "json"
    assert argv[argv.index("--model") + 1] == OPUS and argv[argv.index("--effort") + 1] == "high"
    assert "$schema" not in argv[argv.index("--json-schema") + 1]
    assert call["stdin"] == "Write the spec."
    assert call["cwd_entries"] == [] and call["env_has_nesting"] is False


def test_haiku_runs_without_an_effort_flag() -> None:
    assert "--effort" not in claude_argv(request(Role.REVIEWER))


def test_the_schema_argument_drops_only_the_dialect_key() -> None:
    assert json.loads(schema_argument(SCHEMA)) == {key: value for key, value in SCHEMA.items() if key != "$schema"}
    assert "CLAUDECODE" not in isolated_environment({"CLAUDECODE": "1", "PATH": "/bin"})


@pytest.mark.parametrize(
    ("reported", "pinned", "accepted"),
    [
        (OPUS, OPUS, True),
        ("claude-haiku-4-5-20251001", HAIKU, True),
        (SONNET, OPUS, False),
        ("claude-opus-5-50", OPUS, False),
    ],
)
def test_a_dated_release_of_the_pinned_model_is_accepted_and_nothing_else(
    reported: str, pinned: str, accepted: bool
) -> None:
    assert model_matches_pin(reported, pinned) is accepted


def test_a_call_that_reports_another_model_or_two_models_is_an_isolation_violation(tmp_path: Path) -> None:
    other = model(tmp_path / "other", [{"payload": {"ok": True}, "models": [SONNET]}])
    with pytest.raises(IsolationViolation, match="expected only"):
        other.call(request(), BASELINE)
    two = model(tmp_path / "two", [{"payload": {"ok": True}, "models": [OPUS, HAIKU]}])
    with pytest.raises(IsolationViolation):
        two.call(request(), BASELINE)


def test_input_tokens_far_above_the_request_size_reveal_ambient_context(tmp_path: Path) -> None:
    loud = model(tmp_path, [{"payload": {"ok": True}, "input_tokens": 9000}])
    with pytest.raises(IsolationViolation, match="exceed the bound"):
        loud.call(request(), BASELINE)


def test_a_file_left_in_the_call_directory_is_an_isolation_violation(tmp_path: Path) -> None:
    messy = model(tmp_path, [{"payload": {"ok": True}, "leave_file": "notes.txt"}])
    with pytest.raises(IsolationViolation, match="not empty"):
        messy.call(request(), BASELINE)


def test_failures_and_quota_are_told_apart(tmp_path: Path) -> None:
    with pytest.raises(ModelCallFailed, match="exited 2"):
        model(tmp_path / "a", [{"exit": 2, "stderr": "boom"}]).call(request(), BASELINE)
    with pytest.raises(QuotaExhausted):
        model(tmp_path / "b", [{"exit": 1, "stderr": "You have hit your limit"}]).call(request(), BASELINE)
    with pytest.raises(ModelCallFailed, match="structured_output"):
        model(tmp_path / "c", [{"is_error": True}]).call(request(), BASELINE)


class Failures:
    def __init__(self) -> None:
        self.entries: list[tuple[str, str, str]] = []

    def append(self, call_id: str, key: str, message: str) -> None:
        self.entries.append((call_id, key, message))


class Run:
    """The state a construction run leaves behind (archive, ledger, failures) and the model built over it."""

    def __init__(self) -> None:
        self.calls, self.ledger, self.failures = MemoryCalls(), MemoryLedger(), Failures()

    def model(
        self,
        directory: Path,
        responses: list[dict[str, object]],
        baseline: dict[str, object] | None = None,
        moment: datetime = NIGHT,
        budget: int = 250,
    ) -> PacedConstructionModel:
        executable = install_fake_claude(directory, responses, baseline)  # type: ignore[arg-type]
        gate = CallGate(
            Harness.CLAUDE_CODE,
            PacingPolicy(RunWindow.parse("21:00-07:30"), budget),
            self.ledger,
            FixedClock(moment),
        )
        return PacedConstructionModel(
            ClaudeCliModel(SubprocessPromptRunner(lambda: 100), str(executable)), gate, self.calls, self.failures
        )

    def charged(self) -> int:
        return self.ledger.ledger.calls_on(TONIGHT, Harness.CLAUDE_CODE)


def paced(tmp_path: Path, responses: list[dict[str, object]], moment: datetime = NIGHT, budget: int = 250):
    run = Run()
    return run.model(tmp_path, responses, moment=moment, budget=budget), run.calls, run.ledger, run.failures.entries


def archived_call(calls: MemoryCalls, key: str) -> CallRecord:
    return next(record for record in calls.records.values() if record.argv[2] == key)


def option(argv: list[str], flag: str) -> str:
    return argv[argv.index(flag) + 1]


def test_a_recorded_call_is_replayed_for_free_and_a_new_call_is_charged(tmp_path: Path) -> None:
    pacing, calls, ledger, _ = paced(tmp_path, [{"payload": {"ok": True}}])
    first = pacing.call(request())
    again = pacing.call(request())
    assert first.payload == again.payload == {"ok": True}
    assert len(recorded_calls(tmp_path)) == 1 and len(calls.records) == 2
    assert ledger.ledger.calls_on(TONIGHT, Harness.CLAUDE_CODE) == 2
    record = archived_call(calls, "T:spec")
    assert record.stdin == "Write the spec." and record.argv[0] == "spec-author"


def test_the_baseline_is_one_trivial_call_with_the_flags_of_the_calls_it_bounds(tmp_path: Path) -> None:
    pacing, calls, *_ = paced(tmp_path, [{"payload": {"ok": True}}])
    pacing.call(request())
    (probe,) = recorded_baseline_calls(tmp_path)
    (real,) = recorded_calls(tmp_path)
    assert probe["stdin"] == BASELINE_PROMPT and probe["cwd_entries"] == []
    for flag in ("--tools", "--setting-sources", "--model", "--effort", "--output-format"):
        assert option(probe["argv"], flag) == option(real["argv"], flag)
    for flag in ("--strict-mcp-config", "--disable-slash-commands", "--no-session-persistence", "--json-schema"):
        assert flag in probe["argv"]
    assert archived_call(calls, f"baseline:{OPUS}:high").stdin == BASELINE_PROMPT


def test_one_baseline_serves_every_role_at_the_same_model_and_effort(tmp_path: Path) -> None:
    pacing, _, ledger, _ = paced(tmp_path, [{"payload": {"ok": True}}])
    for role in (Role.SPEC_AUTHOR, Role.H_WRITER, Role.VIOLATION_AUTHOR):
        pacing.call(request(role, f"T:{role.value}"))
    assert len(recorded_baseline_calls(tmp_path)) == 1
    pacing.call(request(Role.REVIEWER, "T:review"))
    pacing.call(request(Role.ASSIGNER_OPUS, "T:assign"))
    assert len(recorded_baseline_calls(tmp_path)) == 3
    assert ledger.ledger.calls_on(TONIGHT, Harness.CLAUDE_CODE) == 3 + 5


def test_a_resumed_run_reuses_the_archived_baseline_without_a_call_or_a_charge(tmp_path: Path) -> None:
    run = Run()
    run.model(tmp_path / "first", [{"payload": {"ok": True}}]).call(request(key="one"))
    run.model(tmp_path / "resumed", [{"payload": {"ok": True}}]).call(request(key="two"))
    assert recorded_baseline_calls(tmp_path / "resumed") == []
    assert run.charged() == 3


def test_a_baseline_over_the_ceiling_stops_the_run_and_is_not_archived(tmp_path: Path) -> None:
    run = Run()
    leaky = run.model(
        tmp_path, [{"payload": {"ok": True}}], baseline={"input_tokens": BASELINE_INPUT_TOKEN_CEILING + 1}
    )
    with pytest.raises(IsolationViolation, match=r"baseline input tokens 3001 .* ceiling 3000"):
        leaky.call(request())
    assert recorded_calls(tmp_path) == [] and not run.calls.records
    assert run.charged() == 1 and len(run.failures.entries) == 1


def test_a_baseline_at_the_ceiling_is_accepted(tmp_path: Path) -> None:
    at_ceiling = Run().model(
        tmp_path, [{"payload": {"ok": True}}], baseline={"input_tokens": BASELINE_INPUT_TOKEN_CEILING}
    )
    assert at_ceiling.call(request()).payload == {"ok": True}


def test_a_replayed_baseline_over_the_ceiling_is_refused(tmp_path: Path) -> None:
    run = Run()
    run.model(tmp_path / "first", [{"payload": {"ok": True}}]).call(request())
    record = archived_call(run.calls, f"baseline:{OPUS}:high")
    inflated = json.loads(record.stdout)
    inflated["usage"]["input_tokens"] = BASELINE_INPUT_TOKEN_CEILING + 1
    run.calls.records[record.call_id] = replace(record, stdout=json.dumps(inflated))
    with pytest.raises(IsolationViolation, match="ceiling"):
        run.model(tmp_path / "second", [{"payload": {"ok": True}}]).call(request(key="other"))


def test_a_large_diff_request_passes_at_its_measured_input_tokens(tmp_path: Path) -> None:
    system = "You write specs."
    overhead = len(system) + len(canonical_json(SCHEMA))
    diff = ModelRequest(Role.SPEC_AUTHOR, "USESTRIX-STRIX-1270:spec-draft", system, "x" * (34_863 - overhead), SCHEMA)
    assert diff.request_chars == 34_863
    pacing, *_ = paced(tmp_path, [{"payload": {"ok": True}, "input_tokens": 14_853}])
    assert pacing.call(diff).input_tokens == 14_853


def test_ambient_context_on_top_of_a_large_request_is_still_caught(tmp_path: Path) -> None:
    large = ModelRequest(Role.SPEC_AUTHOR, "T:spec", "You write specs.", "x" * 34_000, SCHEMA)
    own_tokens = math.ceil(large.request_chars / 2.5)
    pacing, *_ = paced(tmp_path, [{"payload": {"ok": True}, "input_tokens": BASELINE + own_tokens + 4_000}])
    with pytest.raises(IsolationViolation, match="exceed the bound"):
        pacing.call(large)


def test_the_window_and_the_budget_pause_a_new_call_but_never_a_recorded_one(tmp_path: Path) -> None:
    closed, *_ = paced(tmp_path / "day", [{"payload": {"ok": True}}], moment=DAY)
    with pytest.raises(RunPaused) as window:
        closed.call(request())
    assert window.value.reason is PauseReason.WINDOW_CLOSED
    assert recorded_baseline_calls(tmp_path / "day") == []
    spent, *_ = paced(tmp_path / "budget", [{"payload": {"ok": True}}], budget=2)
    spent.call(request(key="one"))
    with pytest.raises(RunPaused) as budget:
        spent.call(request(key="two"))
    assert budget.value.reason is PauseReason.BUDGET_REACHED
    assert spent.call(request(key="one")).payload == {"ok": True}


def test_quota_pauses_and_a_failed_call_is_logged_charged_and_never_archived(tmp_path: Path) -> None:
    quota, *_ = paced(tmp_path / "q", [{"exit": 1, "stderr": "usage limit reached"}])
    with pytest.raises(RunPaused) as paused:
        quota.call(request())
    assert paused.value.reason is PauseReason.QUOTA_EXHAUSTED
    broken, calls, ledger, failures = paced(tmp_path / "f", [{"exit": 2}, {"payload": {"ok": True}}])
    with pytest.raises(ModelCallFailed):
        broken.call(request())
    assert {record.argv[2] for record in calls.records.values()} == {f"baseline:{OPUS}:high"}
    assert len(failures) == 1 and failures[0][1] == "T:spec"
    assert ledger.ledger.calls_on(TONIGHT, Harness.CLAUDE_CODE) == 2
    assert broken.call(request()).payload == {"ok": True}


def test_a_recorded_failure_does_not_block_a_rerun_of_that_call(tmp_path: Path) -> None:
    layout = RunLayout(tmp_path / "construction")
    calls, ledger = FileCallStore(layout), MemoryLedger()

    def build(directory: Path, responses: list[dict[str, object]]) -> PacedConstructionModel:
        executable = install_fake_claude(directory, responses)  # type: ignore[arg-type]
        policy = PacingPolicy(RunWindow.parse("21:00-07:30"), 250)
        gate = CallGate(Harness.CLAUDE_CODE, policy, ledger, FixedClock(NIGHT))
        return PacedConstructionModel(
            ClaudeCliModel(SubprocessPromptRunner(lambda: 100), str(executable)), gate, calls, FileFailureLog(layout)
        )

    with pytest.raises(IsolationViolation):
        build(tmp_path / "first", [{"payload": {"ok": True}, "input_tokens": 90_000}]).call(request())
    failures = (layout.root / FAILURES_FILE_NAME).read_text().splitlines()
    assert len(failures) == 1 and json.loads(failures[0])["call_id"] == request().call_id
    assert build(tmp_path / "second", [{"payload": {"ok": True}}]).call(request()).payload == {"ok": True}
    assert recorded_baseline_calls(tmp_path / "second") == [] and len(recorded_calls(tmp_path / "second")) == 1


def test_a_replayed_call_is_held_to_the_same_pin_as_a_fresh_one(tmp_path: Path) -> None:
    pacing, calls, *_ = paced(tmp_path, [{"payload": {"ok": True}}])
    pacing.call(request())
    record = archived_call(calls, "T:spec")
    tampered = json.loads(record.stdout)
    tampered["modelUsage"] = {SONNET: {}}
    calls.records[record.call_id] = replace(record, stdout=json.dumps(tampered))
    with pytest.raises(IsolationViolation):
        pacing.call(request())
