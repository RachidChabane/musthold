from __future__ import annotations

import json
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
from specbench.application.paced_calls import CallGate, PacedConstructionModel
from specbench.domain.arms import Harness
from specbench.domain.construction import (
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
from specbench.domain.pacing import PacingPolicy, PauseReason, RunPaused, RunWindow
from support.fake_claude import install_fake_claude, recorded_calls
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
    response = claude.call(request())
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
        other.call(request())
    two = model(tmp_path / "two", [{"payload": {"ok": True}, "models": [OPUS, HAIKU]}])
    with pytest.raises(IsolationViolation):
        two.call(request())


def test_input_tokens_far_above_the_request_size_reveal_ambient_context(tmp_path: Path) -> None:
    loud = model(tmp_path, [{"payload": {"ok": True}, "input_tokens": 9000}])
    with pytest.raises(IsolationViolation, match="exceed the bound"):
        loud.call(request())


def test_a_file_left_in_the_call_directory_is_an_isolation_violation(tmp_path: Path) -> None:
    messy = model(tmp_path, [{"payload": {"ok": True}, "leave_file": "notes.txt"}])
    with pytest.raises(IsolationViolation, match="not empty"):
        messy.call(request())


def test_failures_and_quota_are_told_apart(tmp_path: Path) -> None:
    with pytest.raises(ModelCallFailed, match="exited 2"):
        model(tmp_path / "a", [{"exit": 2, "stderr": "boom"}]).call(request())
    with pytest.raises(QuotaExhausted):
        model(tmp_path / "b", [{"exit": 1, "stderr": "You have hit your limit"}]).call(request())
    with pytest.raises(ModelCallFailed, match="structured_output"):
        model(tmp_path / "c", [{"is_error": True}]).call(request())


def paced(tmp_path: Path, responses: list[dict[str, object]], moment: datetime = NIGHT, budget: int = 250):
    calls, ledger, clock = MemoryCalls(), MemoryLedger(), FixedClock(moment)
    failures: list[tuple[str, str, str]] = []

    class Failures:
        def append(self, call_id: str, key: str, message: str) -> None:
            failures.append((call_id, key, message))

    gate = CallGate(Harness.CLAUDE_CODE, PacingPolicy(RunWindow.parse("21:00-07:30"), budget), ledger, clock)
    return PacedConstructionModel(model(tmp_path, responses), gate, calls, Failures()), calls, ledger, failures


def test_a_recorded_call_is_replayed_for_free_and_a_new_call_is_charged(tmp_path: Path) -> None:
    pacing, calls, ledger, _ = paced(tmp_path, [{"payload": {"ok": True}}])
    first = pacing.call(request())
    again = pacing.call(request())
    assert first.payload == again.payload == {"ok": True}
    assert len(recorded_calls(tmp_path)) == 1 and len(calls.records) == 1
    assert ledger.ledger.calls_on(datetime(2026, 10, 8).date(), Harness.CLAUDE_CODE) == 1
    record = next(iter(calls.records.values()))
    assert record.stdin == "Write the spec." and record.argv[0] == "spec-author"


def test_the_window_and_the_budget_pause_a_new_call_but_never_a_recorded_one(tmp_path: Path) -> None:
    closed, *_ = paced(tmp_path / "day", [{"payload": {"ok": True}}], moment=DAY)
    with pytest.raises(RunPaused) as window:
        closed.call(request())
    assert window.value.reason is PauseReason.WINDOW_CLOSED
    spent, *_ = paced(tmp_path / "budget", [{"payload": {"ok": True}}], budget=1)
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
    assert not calls.records and len(failures) == 1 and failures[0][1] == "T:spec"
    assert ledger.ledger.calls_on(datetime(2026, 10, 8).date(), Harness.CLAUDE_CODE) == 1
    assert broken.call(request()).payload == {"ok": True}


def test_a_replayed_call_is_held_to_the_same_pin_as_a_fresh_one(tmp_path: Path) -> None:
    pacing, calls, *_ = paced(tmp_path, [{"payload": {"ok": True}}])
    pacing.call(request())
    (record,) = calls.records.values()
    tampered = json.loads(record.stdout)
    tampered["modelUsage"] = {SONNET: {}}
    calls.records[record.call_id] = type(record)(
        record.call_id, record.argv, record.stdin, json.dumps(tampered), (), 0, record.session_id, None
    )
    with pytest.raises(IsolationViolation):
        pacing.call(request())
