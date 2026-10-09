"""Construction roles (ADR 0007): which model plays each, and the identity of one single-shot call.

Construction roles are not arms. Each call is isolated: no tools, no settings, no session, an empty working
directory. The role fixes the model; a call that reports another model is a violation, never a result.
"""

from __future__ import annotations

import json
import math
import re
from dataclasses import dataclass
from enum import Enum
from typing import Final

from specbench.domain.canonical import JsonValue, canonical_json, sha256_hex
from specbench.domain.jsonread import mapping

OPUS: Final = "claude-opus-5-5"
HAIKU: Final = "claude-haiku-4-5"
SONNET: Final = "claude-sonnet-5-5"

CONSTRUCTION_CALL_PREFIX: Final = "build-"
CONSTRUCTION_CALL_DIGEST_LENGTH: Final = 20
DATED_MODEL_SUFFIX: Final = re.compile(r"-\d{8}$")
CHARS_PER_TOKEN: Final = 2.5
"""Characters per token of the densest text sent, a code diff; the request's own text never costs more tokens than this."""
INPUT_TOKEN_SLACK: Final = 500
"""Input tokens beyond the baseline and the request's own text that tokenization may add; a larger overshoot means ambient context leaked."""
BASELINE_INPUT_TOKEN_CEILING: Final = 3000
"""What a trivial isolated call may cost in input tokens; more means CLAUDE.md, memory or tool definitions loaded."""
BASELINE_KEY_PREFIX: Final = "baseline"
BASELINE_SYSTEM_PROMPT: Final = "You answer with the requested JSON."
BASELINE_PROMPT: Final = "Reply with ok set to true."
BASELINE_SCHEMA: Final[JsonValue] = {
    "type": "object",
    "required": ["ok"],
    "properties": {"ok": {"type": "boolean"}},
}


USAGE_INPUT_KEYS: Final = ("input_tokens", "cache_creation_input_tokens", "cache_read_input_tokens")


class ModelCallFailed(RuntimeError):
    """A call produced no usable result: a failed process, an error result, or a payload that is not structured."""


class IsolationViolation(ModelCallFailed):
    """A call that ran, but not in the isolation the construction requires; its result must not be used."""


class QuotaExhausted(ModelCallFailed):
    """The subscription's quota is spent; the run pauses and a rerun resumes."""


class Role(Enum):
    SPEC_AUTHOR = "spec-author"
    REVIEWER = "reviewer"
    H_WRITER = "h-writer"
    VIOLATION_AUTHOR = "violation-author"
    PLANTED_AUTHOR = "planted-author"
    ASSIGNER_OPUS = "assigner-opus"
    ASSIGNER_HAIKU = "assigner-haiku"
    LEAK_PROBE = "leak-probe"


ROLE_MODEL: Final = {
    Role.SPEC_AUTHOR: OPUS,
    Role.REVIEWER: HAIKU,
    Role.H_WRITER: OPUS,
    Role.VIOLATION_AUTHOR: OPUS,
    Role.PLANTED_AUTHOR: OPUS,
    Role.ASSIGNER_OPUS: OPUS,
    Role.ASSIGNER_HAIKU: HAIKU,
    Role.LEAK_PROBE: SONNET,
}
"""Opus writes the truth and the spec, Haiku reviews and cross-classifies, Sonnet probes what leaks to the coder."""

ROLE_EFFORT: Final[dict[Role, str | None]] = {
    Role.SPEC_AUTHOR: "high",
    Role.REVIEWER: None,
    Role.H_WRITER: "high",
    Role.VIOLATION_AUTHOR: "high",
    Role.PLANTED_AUTHOR: "high",
    Role.ASSIGNER_OPUS: "medium",
    Role.ASSIGNER_HAIKU: None,
    Role.LEAK_PROBE: "medium",
}
"""`--effort` per role; `None` leaves the flag off (Haiku's support for it is settled at the canary)."""


def model_matches_pin(reported: str, pinned: str) -> bool:
    """A reported model id equals the pin, or the pin followed by a release date (`claude-haiku-4-5-20251001`)."""
    return reported == pinned or DATED_MODEL_SUFFIX.sub("", reported) == pinned


def pin_violation(reported_models: tuple[str, ...], pinned: str) -> str | None:
    if len(reported_models) != 1 or not model_matches_pin(reported_models[0], pinned):
        return f"modelUsage keys {sorted(reported_models)}, expected only {pinned!r}"
    return None


def input_token_bound(baseline_input_tokens: int, request_chars: int) -> int:
    """The most input tokens a call may report: what an empty isolated call costs, its own text, and the slack."""
    return baseline_input_tokens + math.ceil(request_chars / CHARS_PER_TOKEN) + INPUT_TOKEN_SLACK


@dataclass(frozen=True, order=True)
class BaselineKey:
    """What an empty isolated call's input cost depends on: the model and the effort flag."""

    model: str
    effort: str | None


@dataclass(frozen=True)
class ModelRequest:
    """One isolated single-shot call. `key` names the call within the construction (task, stage, attempt)."""

    role: Role
    key: str
    system_prompt: str
    prompt: str
    schema: JsonValue

    @property
    def model(self) -> str:
        return ROLE_MODEL[self.role]

    @property
    def effort(self) -> str | None:
        return ROLE_EFFORT[self.role]

    def identity(self) -> dict[str, JsonValue]:
        return {
            "effort": self.effort,
            "key": self.key,
            "model": self.model,
            "prompt": self.prompt,
            "role": self.role.value,
            "schema": self.schema,
            "system": self.system_prompt,
        }

    @property
    def request_chars(self) -> int:
        return len(self.system_prompt) + len(self.prompt) + len(canonical_json(self.schema))

    @property
    def baseline_key(self) -> BaselineKey:
        return BaselineKey(self.model, self.effort)

    @property
    def call_id(self) -> str:
        digest = sha256_hex(canonical_json(self.identity()).encode())
        return CONSTRUCTION_CALL_PREFIX + digest[:CONSTRUCTION_CALL_DIGEST_LENGTH]


@dataclass(frozen=True)
class ModelResponse:
    """What an isolated call returned: the structured payload, the models it ran and the raw stdout."""

    payload: JsonValue
    reported_models: tuple[str, ...]
    stdout: str
    input_tokens: int
    output_tokens: int


def baseline_role(key: BaselineKey) -> Role:
    """The first role that runs at the key's model and effort; its requests carry the key's baseline call."""
    return next(role for role in Role if (ROLE_MODEL[role], ROLE_EFFORT[role]) == (key.model, key.effort))


def baseline_request(key: BaselineKey) -> ModelRequest:
    """The trivial call, with the flags of every call at this model and effort, whose input tokens are the baseline."""
    name = f"{BASELINE_KEY_PREFIX}:{key.model}:{key.effort or 'default'}"
    return ModelRequest(baseline_role(key), name, BASELINE_SYSTEM_PROMPT, BASELINE_PROMPT, BASELINE_SCHEMA)


def _read_result(request: ModelRequest, stdout: str) -> tuple[ModelResponse, str | None]:
    """The response of a result and its pin violation, if any; the token checks are the callers' own."""
    try:
        data = mapping(json.loads(stdout), "claude result")
    except (ValueError, TypeError) as error:
        raise ModelCallFailed("stdout is not a JSON result") from error
    if data.get("is_error") or "structured_output" not in data:
        raise ModelCallFailed(f"error result or no structured_output: {str(data.get('result'))[:200]}")
    models = tuple(sorted(mapping(data.get("modelUsage", {}), "modelUsage")))
    usage = mapping(data.get("usage", {}), "usage")
    response = ModelResponse(
        payload=data["structured_output"],  # type: ignore[arg-type]
        reported_models=models,
        stdout=stdout,
        input_tokens=sum(int(str(usage.get(key, 0))) for key in USAGE_INPUT_KEYS),
        output_tokens=int(str(usage.get("output_tokens", 0))),
    )
    return response, pin_violation(models, request.model)


def response_from_stdout(request: ModelRequest, stdout: str, baseline_input_tokens: int) -> ModelResponse:
    """The structured result of a `claude -p --output-format json` run, after the isolation checks.

    Used on a live call and again when a call is replayed from its archive, so a recorded response is held to the
    same pin and token bound as a fresh one.
    """
    response, violation = _read_result(request, stdout)
    bound = input_token_bound(baseline_input_tokens, request.request_chars)
    if violation is None and response.input_tokens > bound:
        violation = f"input tokens {response.input_tokens} exceed the bound {bound}"
    if violation is not None:
        raise IsolationViolation(violation)
    return response


def baseline_from_stdout(request: ModelRequest, stdout: str) -> ModelResponse:
    """The result of a baseline call; its input tokens must stay under the ceiling that proves no ambient context loaded."""
    response, violation = _read_result(request, stdout)
    if violation is None and response.input_tokens > BASELINE_INPUT_TOKEN_CEILING:
        violation = (
            f"baseline input tokens {response.input_tokens} for {request.model} "
            f"exceed the ceiling {BASELINE_INPUT_TOKEN_CEILING}: ambient context loaded"
        )
    if violation is not None:
        raise IsolationViolation(violation)
    return response
