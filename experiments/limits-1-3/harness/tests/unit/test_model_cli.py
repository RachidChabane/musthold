from __future__ import annotations

import json
from pathlib import Path

import pytest

from pilot.adapters.model_cli import ClaudeCli, CodexCli
from pilot.adapters.process import ProcessResult
from pilot.domain.isolation import token_bound
from pilot.domain.model_call import (
    CanaryBaseline,
    Cli,
    Effort,
    MissingResult,
    ModelPin,
    ModelProvenance,
    ModelRequest,
    ModelResult,
)

SCHEMA = '{"type":"object"}'
CLAUDE_PIN = ModelPin("claude-sonnet-5-5", Cli.CLAUDE)
CODEX_PIN = ModelPin("gpt-5.6-terra", Cli.CODEX)


class FakeProcess:
    """Scripted ProcessRunner: each call pops the next behaviour and records its inputs."""

    def __init__(self, *behaviours):
        self.behaviours = list(behaviours)
        self.calls = []

    def run(self, argv, cwd, *, stdin_path=None, env=None, deadline_seconds=None):
        stdin = stdin_path.read_text() if stdin_path else None
        self.calls.append({"argv": list(argv), "cwd": cwd, "stdin": stdin, "env": env, "listing": sorted(p.name for p in cwd.iterdir())})
        return self.behaviours.pop(0)(cwd)


def request(pin, call_id="c1", item="hello world"):
    return ModelRequest(call_id, pin, Effort.MEDIUM, item, SCHEMA, "system text")


def claude_stdout(responses, model="claude-sonnet-5-5", **changes):
    data = json.loads((responses / "claude_ok.json").read_text())
    data["modelUsage"] = {model: data["modelUsage"]["claude-sonnet-5-5"]}
    data.update(changes)
    return json.dumps(data)


def claude_ok(responses, **changes):
    return lambda cwd: ProcessResult(0, claude_stdout(responses, **changes), "")


def codex_ok(responses, extra_events=()):
    def behave(cwd: Path):
        (cwd / "out.json").write_text((responses / "codex_out_ok.json").read_text())
        events = (responses / "codex_events_ok.jsonl").read_text()
        return ProcessResult(0, events + "".join(json.dumps(e) + "\n" for e in extra_events), "")

    return behave


def failing(returncode=1, stdout=""):
    return lambda cwd: ProcessResult(returncode, stdout, "boom")


def test_claude_argv_is_the_contracted_command(tmp_path, responses):
    process = FakeProcess(claude_ok(responses))
    ClaudeCli(process, tmp_path / "raw").call(request(CLAUDE_PIN), None)
    argv = process.calls[0]["argv"]
    assert argv == [
        "claude", "-p", "--model", "claude-sonnet-5-5", "--effort", "medium",
        "--system-prompt", "system text", "--tools", "", "--setting-sources", "",
        "--strict-mcp-config", "--disable-slash-commands", "--no-session-persistence",
        "--output-format", "json", "--json-schema", SCHEMA,
    ]


def test_claude_runs_in_an_empty_directory_with_stdin_from_a_file(tmp_path, responses):
    process = FakeProcess(claude_ok(responses))
    ClaudeCli(process, tmp_path / "raw").call(request(CLAUDE_PIN, item="the item"), None)
    call = process.calls[0]
    assert call["listing"] == []
    assert call["stdin"] == "the item"
    assert not str(call["cwd"]).startswith(str(tmp_path))
    assert not call["cwd"].exists()


def test_claude_nesting_variables_are_stripped(tmp_path, responses, monkeypatch):
    monkeypatch.setenv("CLAUDECODE", "1")
    monkeypatch.setenv("CLAUDE_CODE_ENTRYPOINT", "cli")
    monkeypatch.setenv("CLAUDE_CODE_OAUTH_TOKEN", "keep")
    process = FakeProcess(claude_ok(responses))
    ClaudeCli(process, tmp_path / "raw").call(request(CLAUDE_PIN), None)
    env = process.calls[0]["env"]
    assert "CLAUDECODE" not in env and "CLAUDE_CODE_ENTRYPOINT" not in env
    assert env["CLAUDE_CODE_OAUTH_TOKEN"] == "keep"


def test_claude_success_records_cost_turns_and_total_input_tokens(tmp_path, responses):
    result = ClaudeCli(FakeProcess(claude_ok(responses)), tmp_path / "raw").call(request(CLAUDE_PIN), None)
    assert isinstance(result, ModelResult)
    assert result.payload == {"answer": 4}
    assert (result.usage.input_tokens, result.usage.output_tokens) == (1174, 52)
    assert result.num_turns == 2 and result.cost_usd == pytest.approx(0.005212)
    assert result.attempt == 1 and result.provenance is ModelProvenance.REPORTED


def test_claude_wrong_or_extra_model_usage_key_is_an_isolation_violation(tmp_path, responses):
    raw = tmp_path / "raw"
    wrong = ClaudeCli(FakeProcess(claude_ok(responses, model="claude-opus-5-5"), claude_ok(responses, model="claude-opus-5-5")), raw)
    outcome = wrong.call(request(CLAUDE_PIN), None)
    assert isinstance(outcome, MissingResult)
    assert len(outcome.isolation_violations) == 2
    assert "claude-opus-5-5" in outcome.isolation_violations[0]


def test_claude_second_model_in_usage_is_rejected(tmp_path, responses):
    extra = {"claude-sonnet-5-5": {}, "claude-haiku-5": {}}
    process = FakeProcess(claude_ok(responses, modelUsage=extra), claude_ok(responses, modelUsage=extra))
    assert isinstance(ClaudeCli(process, tmp_path / "raw").call(request(CLAUDE_PIN), None), MissingResult)


def test_claude_token_bound_uses_the_canary_baseline(tmp_path, responses):
    baseline = CanaryBaseline(input_tokens=1174, num_turns=2)
    item = "x" * 250
    assert token_bound(baseline, len(item)) == 1174 + 100 + 500
    ok = ClaudeCli(FakeProcess(claude_ok(responses)), tmp_path / "ok").call(request(CLAUDE_PIN, item=item), baseline)
    assert isinstance(ok, ModelResult)
    too_small = CanaryBaseline(input_tokens=500, num_turns=2)
    bad = ClaudeCli(FakeProcess(claude_ok(responses), claude_ok(responses)), tmp_path / "bad").call(request(CLAUDE_PIN, item=item), too_small)
    assert isinstance(bad, MissingResult)
    assert "exceed bound" in bad.isolation_violations[0]


def test_claude_num_turns_must_match_the_canary(tmp_path, responses):
    baseline = CanaryBaseline(input_tokens=5000, num_turns=1)
    process = FakeProcess(claude_ok(responses), claude_ok(responses))
    outcome = ClaudeCli(process, tmp_path / "raw").call(request(CLAUDE_PIN), baseline)
    assert isinstance(outcome, MissingResult)
    assert "num_turns 2" in outcome.isolation_violations[0]


def test_claude_leftover_file_in_the_call_directory_is_a_violation(tmp_path, responses):
    def littering(cwd: Path):
        (cwd / "stray.txt").write_text("x")
        return claude_ok(responses)(cwd)

    outcome = ClaudeCli(FakeProcess(littering, littering), tmp_path / "raw").call(request(CLAUDE_PIN), None)
    assert isinstance(outcome, MissingResult)
    assert "not empty" in outcome.isolation_violations[0]


def test_failed_call_is_retried_once_then_recorded_missing(tmp_path, responses):
    process = FakeProcess(failing(), failing())
    outcome = ClaudeCli(process, tmp_path / "raw").call(request(CLAUDE_PIN), None)
    assert len(process.calls) == 2
    assert outcome == MissingResult(("claude exited 1", "claude exited 1"), ())


def test_retry_success_records_the_attempt_number(tmp_path, responses):
    process = FakeProcess(failing(), claude_ok(responses))
    outcome = ClaudeCli(process, tmp_path / "raw").call(request(CLAUDE_PIN), None)
    assert isinstance(outcome, ModelResult) and outcome.attempt == 2
    assert (tmp_path / "raw" / "c1" / "attempt-1" / "stderr.txt").read_text() == "boom"
    assert (tmp_path / "raw" / "c1" / "attempt-2").is_dir()


def test_non_json_or_error_results_are_failures(tmp_path, responses):
    garbage = lambda cwd: ProcessResult(0, "not json", "")  # noqa: E731
    error = claude_ok(responses, is_error=True)
    outcome = ClaudeCli(FakeProcess(garbage, error), tmp_path / "raw").call(request(CLAUDE_PIN), None)
    assert isinstance(outcome, MissingResult)
    assert outcome.failures == ("stdout is not JSON", "error result or no structured_output")


def test_raw_files_are_stored_verbatim(tmp_path, responses):
    stdout = claude_stdout(responses)
    ClaudeCli(FakeProcess(lambda cwd: ProcessResult(0, stdout, "warn")), tmp_path / "raw").call(request(CLAUDE_PIN, item="ITEM"), None)
    attempt = tmp_path / "raw" / "c1" / "attempt-1"
    assert (attempt / "request.txt").read_text() == "ITEM"
    assert (attempt / "stdout.txt").read_text() == stdout
    assert (attempt / "stderr.txt").read_text() == "warn"
    assert json.loads((attempt / "argv.json").read_text())[0] == "claude"
    assert (attempt / "schema.json").read_text() == SCHEMA


def test_a_recorded_call_is_not_repeated(tmp_path, responses):
    raw = tmp_path / "raw"
    first = ClaudeCli(FakeProcess(claude_ok(responses)), raw).call(request(CLAUDE_PIN), None)
    silent = FakeProcess()
    again = ClaudeCli(silent, raw).call(request(CLAUDE_PIN), None)
    assert again == first and silent.calls == []


def test_a_recorded_missing_result_is_not_repeated(tmp_path):
    raw = tmp_path / "raw"
    first = ClaudeCli(FakeProcess(failing(), failing()), raw).call(request(CLAUDE_PIN), None)
    silent = FakeProcess()
    assert ClaudeCli(silent, raw).call(request(CLAUDE_PIN), None) == first
    assert silent.calls == []


CODEX_FLAGS = [
    "--disable", "shell_tool", "--disable", "hooks", "--disable", "plugins", "--disable", "apps",
    "--disable", "browser_use", "--disable", "computer_use", "--disable", "in_app_browser",
    "--disable", "skill_search", "--disable", "tool_suggest", "--disable", "sleep_tool",
    "--disable", "shell_snapshot",
]


def test_codex_argv_is_the_contracted_command(tmp_path, responses):
    process = FakeProcess(codex_ok(responses))
    CodexCli(process, tmp_path / "raw").call(request(CODEX_PIN), None)
    call = process.calls[0]
    schema_path = str(tmp_path / "raw" / "c1" / "attempt-1" / "schema.json")
    assert call["argv"] == [
        "codex", "exec", "--ephemeral", "--skip-git-repo-check", "--ignore-user-config",
        "--ignore-rules", "-s", "read-only", "-C", str(call["cwd"]), "-m", "gpt-5.6-terra",
        "-c", "model_reasoning_effort=medium", "-c", "web_search=disabled",
        *CODEX_FLAGS,
        "--output-schema", schema_path, "-o", "out.json", "--json",
    ]


def test_codex_success_parses_events_and_records_usage(tmp_path, responses):
    outcome = CodexCli(FakeProcess(codex_ok(responses)), tmp_path / "raw").call(request(CODEX_PIN), None)
    assert isinstance(outcome, ModelResult)
    assert outcome.payload == {"answer": 4}
    assert (outcome.usage.input_tokens, outcome.usage.output_tokens) == (8627, 15)
    assert outcome.cost_usd is None and outcome.provenance is ModelProvenance.REQUESTED_ONLY
    attempt = tmp_path / "raw" / "c1" / "attempt-1"
    assert (attempt / "events.jsonl").read_text() == (responses / "codex_events_ok.jsonl").read_text()
    assert (attempt / "out.json").read_text() == '{"answer":4}'


def test_codex_prepends_the_system_prompt_to_the_item(tmp_path, responses):
    process = FakeProcess(codex_ok(responses))
    CodexCli(process, tmp_path / "raw").call(request(CODEX_PIN, item="ITEM"), None)
    assert process.calls[0]["stdin"] == "system text\n\nITEM"


@pytest.mark.parametrize("item_type", ["command_execution", "file_change", "web_search", "mcp_tool_call"])
def test_codex_rejects_any_item_type_besides_message_and_reasoning(tmp_path, responses, item_type):
    event = {"type": "item.completed", "item": {"id": "i", "type": item_type}}
    process = FakeProcess(codex_ok(responses, [event]), codex_ok(responses, [event]))
    outcome = CodexCli(process, tmp_path / "raw").call(request(CODEX_PIN), None)
    assert isinstance(outcome, MissingResult)
    assert outcome.isolation_violations[0] == f"forbidden codex item type {item_type!r}"


def test_codex_reasoning_items_are_allowed(tmp_path, responses):
    event = {"type": "item.completed", "item": {"id": "r", "type": "reasoning", "text": "..."}}
    outcome = CodexCli(FakeProcess(codex_ok(responses, [event])), tmp_path / "raw").call(request(CODEX_PIN), None)
    assert isinstance(outcome, ModelResult)


def test_codex_token_bound_applies(tmp_path, responses):
    baseline = CanaryBaseline(input_tokens=1000, num_turns=None)
    process = FakeProcess(codex_ok(responses), codex_ok(responses))
    outcome = CodexCli(process, tmp_path / "raw").call(request(CODEX_PIN, item="x"), baseline)
    assert isinstance(outcome, MissingResult)
    assert "exceed bound" in outcome.isolation_violations[0]


def test_codex_missing_output_file_is_a_failure(tmp_path, responses):
    events = (responses / "codex_events_ok.jsonl").read_text()
    quiet = lambda cwd: ProcessResult(0, events, "")  # noqa: E731
    outcome = CodexCli(FakeProcess(quiet, quiet), tmp_path / "raw").call(request(CODEX_PIN), None)
    assert isinstance(outcome, MissingResult)
    assert outcome.failures == ("no output file", "no output file")


def test_a_crashed_call_without_a_result_file_never_overwrites_its_raw_attempt(tmp_path, responses):
    raw = tmp_path / "raw"
    first = ClaudeCli(FakeProcess(claude_ok(responses)), raw)
    first.call(request(CLAUDE_PIN), None)
    (raw / "c1" / "result.json").unlink()
    attempt_one = (raw / "c1" / "attempt-1" / "stdout.txt").read_text()
    again = ClaudeCli(FakeProcess(claude_ok(responses, result="changed")), raw).call(request(CLAUDE_PIN), None)
    assert again.attempt == 2
    assert (raw / "c1" / "attempt-1" / "stdout.txt").read_text() == attempt_one
    assert (raw / "c1" / "attempt-2" / "stdout.txt").is_file()


def quota_failure(cwd):
    return ProcessResult(1, "", "Error: 429 rate limit exceeded, usage limit reached")


def test_a_quota_failure_stops_cleanly_and_leaves_the_call_unrecorded(tmp_path, responses):
    from pilot.domain.quota import QuotaExhausted

    raw = tmp_path / "raw"
    with pytest.raises(QuotaExhausted):
        ClaudeCli(FakeProcess(quota_failure), raw).call(request(CLAUDE_PIN), None)
    assert not (raw / "c1" / "result.json").exists()
    resumed = ClaudeCli(FakeProcess(claude_ok(responses)), raw).call(request(CLAUDE_PIN), None)
    assert isinstance(resumed, ModelResult)


def test_quota_words_in_a_successful_answer_are_not_a_quota_failure(tmp_path, responses):
    outcome = ClaudeCli(FakeProcess(claude_ok(responses, result="rate limit")), tmp_path / "raw").call(request(CLAUDE_PIN), None)
    assert isinstance(outcome, ModelResult)


def test_an_ordinary_failure_is_still_recorded_as_missing(tmp_path):
    outcome = ClaudeCli(FakeProcess(failing(), failing()), tmp_path / "raw").call(request(CLAUDE_PIN), None)
    assert isinstance(outcome, MissingResult)


def test_every_raw_record_stores_the_cli_version(tmp_path, responses):
    ClaudeCli(FakeProcess(claude_ok(responses)), tmp_path / "raw", cli_version="2.1.292").call(request(CLAUDE_PIN), None)
    record = json.loads((tmp_path / "raw" / "c1" / "result.json").read_text())
    assert record["cli_version"] == "2.1.292" and record["status"] == "ok"
    ClaudeCli(FakeProcess(failing(), failing()), tmp_path / "raw", cli_version="2.1.292").call(request(CLAUDE_PIN, "c2"), None)
    assert json.loads((tmp_path / "raw" / "c2" / "result.json").read_text())["cli_version"] == "2.1.292"


def test_a_payload_failing_the_request_check_is_retried_once_then_missing(tmp_path, responses):
    checked = ModelRequest("c1", CLAUDE_PIN, Effort.MEDIUM, "item", SCHEMA, "system", validate=lambda payload: ["bad slots"])
    process = FakeProcess(claude_ok(responses), claude_ok(responses))
    outcome = ClaudeCli(process, tmp_path / "raw").call(checked, None)
    assert isinstance(outcome, MissingResult) and len(process.calls) == 2
    assert any("bad slots" in failure for failure in outcome.failures)


def test_a_payload_violating_the_schema_is_invalid(tmp_path, responses):
    from pilot.adapters.models_lock import JsonSchemaValidator

    strict = ModelRequest("c1", CLAUDE_PIN, Effort.MEDIUM, "item", '{"type":"array"}', "system")
    outcome = ClaudeCli(FakeProcess(claude_ok(responses), claude_ok(responses)), tmp_path / "raw", schema_validator=JsonSchemaValidator()).call(strict, None)
    assert isinstance(outcome, MissingResult)


def test_a_check_that_cannot_read_the_payload_counts_as_a_failure(tmp_path, responses):
    def reads_missing_key(payload):
        return payload["absent"]

    checked = ModelRequest("c1", CLAUDE_PIN, Effort.MEDIUM, "item", SCHEMA, "system", validate=reads_missing_key)
    outcome = ClaudeCli(FakeProcess(claude_ok(responses), claude_ok(responses)), tmp_path / "raw").call(checked, None)
    assert isinstance(outcome, MissingResult)


def test_claude_receives_the_schema_without_its_dialect_reference(tmp_path, responses):
    schema = json.dumps({"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object"})
    process = FakeProcess(claude_ok(responses))
    ClaudeCli(process, tmp_path / "raw").call(ModelRequest("c1", CLAUDE_PIN, Effort.MEDIUM, "i", schema, "s"), None)
    assert json.loads(process.calls[0]["argv"][-1]) == {"type": "object"}
    assert json.loads((tmp_path / "raw" / "c1" / "attempt-1" / "schema.json").read_text())["$schema"]


def test_codex_receives_the_schema_file_unchanged(tmp_path, responses):
    schema = json.dumps({"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object"})
    process = FakeProcess(codex_ok(responses))
    CodexCli(process, tmp_path / "raw").call(ModelRequest("c1", CODEX_PIN, Effort.MEDIUM, "i", schema, "s"), None)
    assert (tmp_path / "raw" / "c1" / "attempt-1" / "schema.json").read_text() == schema
