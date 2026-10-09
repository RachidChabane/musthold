"""The shape of a real `claude -p --output-format json` result, from results recorded by an earlier canary."""

from __future__ import annotations

from pathlib import Path

import pytest

from specbench.domain.construction import (
    IsolationViolation,
    ModelRequest,
    Role,
    baseline_from_stdout,
    baseline_request,
    response_from_stdout,
)

FIXTURES = Path(__file__).parent.parent / "fixtures"
SCHEMA = {"type": "object"}
BASELINE = 1100


def request(role: Role) -> ModelRequest:
    return ModelRequest(role, "k", "You are a careful engineer.", "x" * 2000, SCHEMA)


def test_a_recorded_opus_result_is_read_for_the_opus_roles() -> None:
    stdout = (FIXTURES / "claude_result_opus.json").read_text()
    response = response_from_stdout(request(Role.SPEC_AUTHOR), stdout, BASELINE)
    assert response.reported_models == ("claude-opus-5-5",)
    assert isinstance(response.payload, dict) and "tests" in response.payload
    assert response.input_tokens > 0 and response.output_tokens > 0


def test_a_recorded_sonnet_result_is_read_for_the_probe() -> None:
    stdout = (FIXTURES / "claude_result_sonnet.json").read_text()
    assert response_from_stdout(request(Role.LEAK_PROBE), stdout, BASELINE).reported_models == ("claude-sonnet-5-5",)


@pytest.mark.parametrize("role", [Role.REVIEWER, Role.LEAK_PROBE, Role.ASSIGNER_HAIKU])
def test_a_recorded_opus_result_is_refused_for_any_other_pinned_model(role: Role) -> None:
    stdout = (FIXTURES / "claude_result_opus.json").read_text()
    with pytest.raises(IsolationViolation, match="expected only"):
        response_from_stdout(request(role), stdout, BASELINE)


@pytest.mark.parametrize("name", ["opus", "sonnet"])
def test_a_recorded_result_of_a_trivial_call_is_a_baseline_under_the_ceiling(name: str) -> None:
    stdout = (FIXTURES / f"claude_result_{name}.json").read_text()
    role = Role.SPEC_AUTHOR if name == "opus" else Role.LEAK_PROBE
    probe = baseline_request(request(role).baseline_key)
    assert baseline_from_stdout(probe, stdout).input_tokens < 3000
