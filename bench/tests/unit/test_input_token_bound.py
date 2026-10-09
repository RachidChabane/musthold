from __future__ import annotations

import json
import math

import pytest

from specbench.domain.canonical import canonical_json
from specbench.domain.construction import (
    BASELINE_INPUT_TOKEN_CEILING,
    BASELINE_PROMPT,
    CHARS_PER_TOKEN,
    HAIKU,
    INPUT_TOKEN_SLACK,
    OPUS,
    ROLE_EFFORT,
    ROLE_MODEL,
    BaselineKey,
    IsolationViolation,
    ModelRequest,
    Role,
    baseline_from_stdout,
    baseline_request,
    baseline_role,
    input_token_bound,
    response_from_stdout,
)

SCHEMA = {"type": "object"}
BASELINE = 1100


def result(model: str = OPUS, input_tokens: int = 2, creation: int = 0, read: int = 0) -> str:
    return json.dumps(
        {
            "type": "result",
            "is_error": False,
            "structured_output": {"ok": True},
            "modelUsage": {model: {}},
            "usage": {
                "input_tokens": input_tokens,
                "cache_creation_input_tokens": creation,
                "cache_read_input_tokens": read,
                "output_tokens": 5,
            },
        }
    )


def test_the_bound_is_the_baseline_plus_the_densest_tokenization_of_the_text_plus_the_slack() -> None:
    assert CHARS_PER_TOKEN == 2.5 and INPUT_TOKEN_SLACK == 500
    assert input_token_bound(1100, 0) == 1600
    assert input_token_bound(1100, 1000) == 1100 + 400 + 500
    assert input_token_bound(1100, 1001) == 1100 + 401 + 500
    assert input_token_bound(1100, 34_863) == 1100 + math.ceil(34_863 / 2.5) + 500


def test_the_request_that_stopped_the_first_run_fits_its_bound() -> None:
    assert input_token_bound(BASELINE, 34_863) >= 14_853


def test_the_request_text_counts_the_system_prompt_the_prompt_and_the_schema() -> None:
    request = ModelRequest(Role.SPEC_AUTHOR, "k", "abc", "defgh", {"a": 1})
    assert request.request_chars == 3 + 5 + len(canonical_json({"a": 1}))


def test_input_tokens_are_the_sum_of_the_three_usage_input_keys() -> None:
    request = ModelRequest(Role.SPEC_AUTHOR, "k", "s", "p", SCHEMA)
    bound = input_token_bound(BASELINE, request.request_chars)
    assert (
        response_from_stdout(request, result(input_tokens=2, creation=bound - 12, read=10), BASELINE).input_tokens
        == bound
    )
    with pytest.raises(IsolationViolation, match=f"exceed the bound {bound}"):
        response_from_stdout(request, result(input_tokens=2, creation=bound - 11, read=10), BASELINE)
    with pytest.raises(IsolationViolation, match="exceed the bound"):
        response_from_stdout(request, result(input_tokens=1, read=bound), BASELINE)


def test_the_baseline_ceiling_is_inclusive() -> None:
    probe = baseline_request(BaselineKey(OPUS, "high"))
    at_ceiling = baseline_from_stdout(probe, result(input_tokens=2, creation=BASELINE_INPUT_TOKEN_CEILING - 2))
    assert at_ceiling.input_tokens == BASELINE_INPUT_TOKEN_CEILING


def test_a_baseline_over_the_ceiling_names_the_baseline_and_the_model() -> None:
    probe = baseline_request(BaselineKey(OPUS, "high"))
    with pytest.raises(IsolationViolation, match=rf"baseline input tokens 3001 for {OPUS} exceed the ceiling 3000"):
        baseline_from_stdout(probe, result(input_tokens=1, read=BASELINE_INPUT_TOKEN_CEILING))


def test_a_baseline_from_another_model_is_refused_before_its_tokens_are_read() -> None:
    probe = baseline_request(BaselineKey(OPUS, "high"))
    with pytest.raises(IsolationViolation, match="expected only"):
        baseline_from_stdout(probe, result(model=HAIKU))


def test_a_baseline_request_runs_at_the_key_and_is_the_same_whichever_role_asks() -> None:
    for role in Role:
        key = BaselineKey(ROLE_MODEL[role], ROLE_EFFORT[role])
        probe = baseline_request(key)
        assert (probe.model, probe.effort) == (key.model, key.effort)
        assert probe.prompt == BASELINE_PROMPT and probe.baseline_key == key
        assert probe.call_id == baseline_request(key).call_id


def test_roles_sharing_a_model_and_effort_share_one_baseline_call() -> None:
    assert baseline_role(BaselineKey(OPUS, "high")) is Role.SPEC_AUTHOR
    same = {
        baseline_request(ModelRequest(role, "k", "s", "p", SCHEMA).baseline_key).call_id
        for role in (Role.SPEC_AUTHOR, Role.H_WRITER, Role.VIOLATION_AUTHOR, Role.PLANTED_AUTHOR)
    }
    assert len(same) == 1
    assert baseline_request(BaselineKey(OPUS, "medium")).call_id != baseline_request(BaselineKey(OPUS, "high")).call_id
