"""A stub of the Ollama API and of its `/v1/systemone` route, with the wire shapes of the recorded real server."""

from __future__ import annotations

import json
import math
from collections.abc import Callable
from dataclasses import dataclass
from enum import Enum

from support.stub_common import (
    RecordedRequest,
    StubHttpServer,
    StubQuestion,
    StubReply,
    derived_probabilities,
    json_reply,
    parse_questions,
    state_text,
)

OLLAMA_VERSION = "0.35.1"
DEFAULT_TAG = "tev1:0.8b"
DEFAULT_DIGEST = "33aa" * 16
CONTEXT_LENGTH = 2048
OK = 200
BAD_REQUEST = 400
PAYLOAD_TOO_LARGE = 413
NOT_FOUND = 404


class Oversize(Enum):
    """How the stub refuses a state longer than `max_state_chars`."""

    TOKENS = "tokens"
    CONTEXT = "context"
    PAYLOAD = "payload"


@dataclass
class OllamaBehaviour:
    """What a test switches on a stub; every field is read at the time of each request."""

    max_state_chars: int | None = None
    oversize: Oversize = Oversize.TOKENS
    invalid_reply: bool = False
    hang_seconds: float = 0.0
    echo_model: str | None = None
    context_length: int = CONTEXT_LENGTH


def _entropy_confidence(probabilities: list[float]) -> float:
    """1 - H(p) / ln K, written here independently of the harness."""
    count = len(probabilities)
    if count == 1:
        return 1.0
    entropy = -sum(p * math.log(p) for p in probabilities if p > 0.0)
    return 1.0 - entropy / math.log(count)


def _answer(question: StubQuestion, probabilities: list[float]) -> dict[str, object]:
    confidence = _entropy_confidence(probabilities)
    if question.question_type == "choice":
        best = max(range(len(probabilities)), key=probabilities.__getitem__)
        return {
            "type": "choice",
            "choice": question.options[best],
            "probabilities": dict(zip(question.options, probabilities, strict=True)),
            "confidence": confidence,
        }
    if question.question_type == "score":
        return {
            "type": "score",
            "score": sum(index * p for index, p in enumerate(probabilities)),
            "legend": dict(question.legend),
            "probabilities": dict(zip(question.options, probabilities, strict=True)),
            "confidence": confidence,
        }
    return {"type": "noul", "noul": probabilities[1]}


class StubOllamaServer(StubHttpServer):
    """`on_load(model)` and `on_unload(model)` are called when a model becomes resident or is unloaded."""

    def __init__(
        self,
        host: str = "127.0.0.1",
        port: int = 0,
        *,
        behaviour: OllamaBehaviour | None = None,
        models: dict[str, str] | None = None,
        on_load: Callable[[str], None] | None = None,
        on_unload: Callable[[str], None] | None = None,
    ) -> None:
        super().__init__(host, port)
        self.behaviour = behaviour or OllamaBehaviour()
        self.models = dict(models) if models is not None else {DEFAULT_TAG: DEFAULT_DIGEST}
        self.loaded: list[str] = []
        self._on_load = on_load
        self._on_unload = on_unload

    def respond(self, request: RecordedRequest) -> StubReply:
        routes: dict[tuple[str, str], Callable[[RecordedRequest], StubReply]] = {
            ("GET", "/api/version"): lambda _: json_reply(OK, {"version": OLLAMA_VERSION}),
            ("GET", "/api/ps"): lambda _: json_reply(OK, {"models": [self._resident(name) for name in self.loaded]}),
            ("GET", "/api/tags"): lambda _: json_reply(OK, {"models": [self._tag_entry(name) for name in self.models]}),
            ("POST", "/api/show"): self._show,
            ("POST", "/api/pull"): self._pull,
            ("POST", "/api/generate"): self._generate,
            ("POST", "/v1/systemone"): self._system_one,
        }
        handler = routes.get((request.method, request.path))
        return handler(request) if handler is not None else json_reply(NOT_FOUND, {"error": "not found"})

    def _tag_entry(self, name: str) -> dict[str, object]:
        return {"name": name, "model": name, "digest": self.models[name], "size": 1_000_000}

    def _resident(self, name: str) -> dict[str, object]:
        return {"name": name, "model": name, "context_length": self.behaviour.context_length}

    def _show(self, request: RecordedRequest) -> StubReply:
        name = request.json()["model"]
        if name not in self.models:
            return json_reply(NOT_FOUND, {"error": f"model '{name}' not found"})
        return json_reply(
            OK,
            {
                "parameters": 'num_ctx                        2048\ntemperature                    0\nstop                           "<|im_end|>"',
                "details": {
                    "format": "gguf",
                    "family": "qwen",
                    "parameter_size": "0.8B",
                    "quantization_level": "Q4_K_M",
                },
            },
        )

    def _pull(self, request: RecordedRequest) -> StubReply:
        name = request.json()["model"]
        self.models.setdefault(name, DEFAULT_DIGEST[:-4] + f"{len(self.models):04d}")
        return json_reply(OK, {"status": "success"})

    def _generate(self, request: RecordedRequest) -> StubReply:
        document = request.json()
        name = document["model"]
        if document.get("keep_alive") == 0 and name in self.loaded:
            self.loaded.remove(name)
            if self._on_unload is not None:
                self._on_unload(name)
        return json_reply(OK, {"model": name, "done": True, "done_reason": "unload"})

    def _system_one(self, request: RecordedRequest) -> StubReply:
        behaviour = self.behaviour
        document = request.json()
        state = state_text(request.body)
        delay = behaviour.hang_seconds
        if behaviour.invalid_reply:
            return StubReply(OK, b"{not json", delay)
        if behaviour.max_state_chars is not None and len(state) > behaviour.max_state_chars:
            return self._refusal(behaviour.oversize, delay)
        model = document["model"]
        if model not in self.loaded:
            self.loaded.append(model)
            if self._on_load is not None:
                self._on_load(model)
        answers = {
            question.question_id: _answer(
                question, derived_probabilities(state, question.question_id, len(question.options))
            )
            for question in parse_questions(request.body)
        }
        reply = {
            "model": behaviour.echo_model or model,
            "answers": answers,
            "usage": {"input_tokens": len(state), "output_tokens": 1},
        }
        return json_reply(OK, reply, delay)

    @staticmethod
    def _refusal(oversize: Oversize, delay: float) -> StubReply:
        if oversize is Oversize.PAYLOAD:
            return json_reply(PAYLOAD_TOO_LARGE, {"error": "request body too large"}, delay)
        if oversize is Oversize.CONTEXT:
            message = "prompt 0 requires 3000 context tokens for scoring; model has 2048"
        else:
            message = "prompt 0 has 5000 tokens; expected 1\u20132048 (input is never truncated)"
        return StubReply(BAD_REQUEST, json.dumps({"error": message}, ensure_ascii=False).encode("utf-8"), delay)
