from __future__ import annotations

import json
import math

import pytest
from hypothesis import given
from hypothesis import strategies as st

from specbench.domain.construction import (
    BASELINE_INPUT_TOKEN_CEILING,
    CHARS_PER_TOKEN,
    OPUS,
    BaselineKey,
    IsolationViolation,
    ModelRequest,
    Role,
    baseline_from_stdout,
    baseline_request,
    input_token_bound,
    response_from_stdout,
)

LEAK_TOKENS = 4_000
"""The smallest ambient context the check must always catch: a CLAUDE.md plus a memory file."""
baselines = st.integers(min_value=0, max_value=BASELINE_INPUT_TOKEN_CEILING)
sizes = st.integers(min_value=0, max_value=2_000_000)
SCHEMA = {"type": "object"}


def stdout_of(input_tokens: int) -> str:
    return json.dumps(
        {
            "is_error": False,
            "structured_output": {},
            "modelUsage": {OPUS: {}},
            "usage": {"input_tokens": input_tokens, "cache_creation_input_tokens": 0, "cache_read_input_tokens": 0},
        }
    )


def request_of_size(chars: int) -> ModelRequest:
    request = ModelRequest(Role.SPEC_AUTHOR, "k", "", "", SCHEMA)
    prompt = "x" * max(0, chars - request.request_chars)
    return ModelRequest(Role.SPEC_AUTHOR, "k", "", prompt, SCHEMA)


@given(baselines, sizes, st.integers(min_value=0, max_value=100_000))
def test_the_bound_never_shrinks_as_the_request_or_the_baseline_grows(baseline: int, chars: int, more: int) -> None:
    assert input_token_bound(baseline, chars) <= input_token_bound(baseline, chars + more)
    assert input_token_bound(baseline, chars) <= input_token_bound(baseline + more, chars)


@given(baselines, sizes)
def test_the_bound_covers_the_request_at_the_densest_tokenization(baseline: int, chars: int) -> None:
    assert input_token_bound(baseline, chars) >= baseline + chars / CHARS_PER_TOKEN
    assert input_token_bound(baseline, chars) - baseline - math.ceil(chars / CHARS_PER_TOKEN) == 500


@given(baselines, st.integers(min_value=500, max_value=1_000_000))
def test_an_ambient_context_of_four_thousand_tokens_is_always_caught(baseline: int, chars: int) -> None:
    request = request_of_size(chars)
    honest = baseline + math.ceil(request.request_chars / CHARS_PER_TOKEN)
    with pytest.raises(IsolationViolation, match="exceed the bound"):
        response_from_stdout(request, stdout_of(honest + LEAK_TOKENS), baseline)


@given(baselines, st.integers(min_value=500, max_value=1_000_000))
def test_a_request_at_its_densest_tokenization_is_never_flagged(baseline: int, chars: int) -> None:
    request = request_of_size(chars)
    honest = baseline + math.ceil(request.request_chars / CHARS_PER_TOKEN)
    assert response_from_stdout(request, stdout_of(honest), baseline).input_tokens == honest


@given(st.integers(min_value=BASELINE_INPUT_TOKEN_CEILING + 1, max_value=10_000_000))
def test_a_baseline_over_the_ceiling_is_always_refused(tokens: int) -> None:
    probe = baseline_request(BaselineKey(OPUS, "high"))
    with pytest.raises(IsolationViolation, match="ceiling"):
        baseline_from_stdout(probe, stdout_of(tokens))


@given(st.integers(min_value=0, max_value=BASELINE_INPUT_TOKEN_CEILING))
def test_a_baseline_up_to_the_ceiling_is_always_accepted(tokens: int) -> None:
    probe = baseline_request(BaselineKey(OPUS, "high"))
    assert baseline_from_stdout(probe, stdout_of(tokens)).input_tokens == tokens
