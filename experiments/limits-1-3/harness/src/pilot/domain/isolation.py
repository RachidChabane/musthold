"""Per-call isolation checks (PREREG 4.2 and 4.3), pure over parsed call evidence."""

from __future__ import annotations

import math
from collections.abc import Iterable

from pilot.domain.constants import (
    CODEX_ALLOWED_ITEM_TYPES,
    TOKEN_BOUND_SLACK,
    TOKEN_CHARS_PER_TOKEN,
)
from pilot.domain.model_call import CanaryBaseline


def token_bound(baseline: CanaryBaseline, item_chars: int) -> int:
    return baseline.input_tokens + math.ceil(item_chars / TOKEN_CHARS_PER_TOKEN) + TOKEN_BOUND_SLACK


def token_violation(
    total_input_tokens: int, baseline: CanaryBaseline | None, item_chars: int
) -> str | None:
    if baseline is None:
        return None
    bound = token_bound(baseline, item_chars)
    if total_input_tokens > bound:
        return f"input tokens {total_input_tokens} exceed bound {bound}"
    return None


def pinned_model_violation(model_usage_keys: Iterable[str], pinned_model: str) -> str | None:
    keys = sorted(model_usage_keys)
    if keys != [pinned_model]:
        return f"modelUsage keys {keys}, expected only {pinned_model!r}"
    return None


def num_turns_violation(num_turns: int | None, baseline: CanaryBaseline | None) -> str | None:
    if baseline is None or baseline.num_turns is None:
        return None
    if num_turns != baseline.num_turns:
        return f"num_turns {num_turns}, canary baseline {baseline.num_turns}"
    return None


def leftover_files_violation(leftovers: Iterable[str]) -> str | None:
    names = sorted(leftovers)
    if names:
        return f"call directory not empty afterwards: {names}"
    return None


def codex_item_violations(item_types: Iterable[str]) -> list[str]:
    return [
        f"forbidden codex item type {item_type!r}"
        for item_type in sorted(set(item_types) - set(CODEX_ALLOWED_ITEM_TYPES))
    ]
