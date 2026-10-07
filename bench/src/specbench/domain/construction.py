"""Construction roles (ADR 0007): which model plays each, and the identity of one single-shot call.

Construction roles are not arms. Each call is isolated: no tools, no settings, no session, an empty working
directory. The role fixes the model; a call that reports another model is a violation, never a result.
"""

from __future__ import annotations

import json
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
CHARS_PER_TOKEN: Final = 3
INPUT_TOKEN_SLACK: Final = 2500
"""Input tokens beyond the request's own text that the harness itself adds; a larger overshoot means ambient context leaked."""


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


def input_token_bound(system_prompt: str, prompt: str, schema_text: str) -> int:
    return (len(system_prompt) + len(prompt) + len(schema_text)) // CHARS_PER_TOKEN + INPUT_TOKEN_SLACK


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


def response_from_stdout(request: ModelRequest, stdout: str) -> ModelResponse:
    """The structured result of a `claude -p --output-format json` run, after the isolation checks.

    Used on a live call and again when a call is replayed from its archive, so a recorded response is held to the
    same pin and token bound as a fresh one.
    """
    try:
        data = mapping(json.loads(stdout), "claude result")
    except (ValueError, TypeError) as error:
        raise ModelCallFailed("stdout is not a JSON result") from error
    if data.get("is_error") or "structured_output" not in data:
        raise ModelCallFailed(f"error result or no structured_output: {str(data.get('result'))[:200]}")
    models = tuple(sorted(mapping(data.get("modelUsage", {}), "modelUsage")))
    usage = mapping(data.get("usage", {}), "usage")
    input_tokens = sum(int(str(usage.get(key, 0))) for key in USAGE_INPUT_KEYS)
    bound = input_token_bound(request.system_prompt, request.prompt, canonical_json(request.schema))
    violation = pin_violation(models, request.model)
    if violation is None and input_tokens > bound:
        violation = f"input tokens {input_tokens} exceed the bound {bound}"
    if violation is not None:
        raise IsolationViolation(violation)
    return ModelResponse(
        payload=data["structured_output"],  # type: ignore[arg-type]
        reported_models=models,
        stdout=stdout,
        input_tokens=input_tokens,
        output_tokens=int(str(usage.get("output_tokens", 0))),
    )
