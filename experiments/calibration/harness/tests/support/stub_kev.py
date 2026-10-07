"""A stub of the Kev server: `GET /v1/models` with the real card shape and `POST /v1/systemone` in Kev's form."""

from __future__ import annotations

import json
import time
from dataclasses import dataclass

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

OK = 200
UNPROCESSABLE = 422
NOT_FOUND = 404
MAX_STATE_TOKENS = 65536
PROBABILITY_DECIMALS = 4
STATE_TOO_LARGE_DETAIL = (
    "state is 70,000 tokens, over the 65,536-token limit (the <state> token included): shorten the document or "
    "split it across requests; or start the server with KEV_TRUNCATE_STATES=1 to truncate it"
)


@dataclass
class KevBehaviour:
    max_state_chars: int | None = None
    invalid_reply: bool = False
    hang_seconds: float = 0.0
    echo_model: str | None = None


def _choice_confidence(probabilities: list[float]) -> float:
    count = len(probabilities)
    if count == 1:
        return 1.0
    return (max(probabilities) - 1 / count) / (1 - 1 / count)


def _score_confidence(probabilities: list[float]) -> float:
    levels = len(probabilities)
    if levels == 1:
        return 1.0
    mode = max(range(levels), key=probabilities.__getitem__)
    deviation = sum(abs(index - (levels - 1) / 2) for index in range(levels)) / levels
    spread = sum(p * abs(index - mode) for index, p in enumerate(probabilities))
    return max(0.0, 1.0 - spread / deviation)


def _rounded(value: float) -> float:
    return round(value, PROBABILITY_DECIMALS)


def _answer(question: StubQuestion, probabilities: list[float]) -> dict[str, object]:
    shown = {key: _rounded(p) for key, p in zip(question.options, probabilities, strict=True)}
    if question.question_type == "choice":
        best = max(range(len(probabilities)), key=probabilities.__getitem__)
        return {
            "type": "choice",
            "choice": question.options[best],
            "confidence": _rounded(_choice_confidence(probabilities)),
            "probabilities": shown,
        }
    if question.question_type == "score":
        return {
            "type": "score",
            "score": _rounded(sum(index * p for index, p in enumerate(probabilities))),
            "legend": dict(question.legend),
            "probabilities": shown,
            "confidence": _rounded(_score_confidence(probabilities)),
        }
    return {"type": "noul", "noul": _rounded(probabilities[1])}


class StubKevServer(StubHttpServer):
    def __init__(
        self,
        host: str = "127.0.0.1",
        port: int = 0,
        *,
        run: str = "jaredpalmer/kev-0.8b@0123456789abcdef0123456789abcdef01234567",
        behaviour: KevBehaviour | None = None,
    ) -> None:
        super().__init__(host, port)
        self.run = run
        self.behaviour = behaviour or KevBehaviour()
        self.hits = 0
        self.misses = 0
        self._cached_states: set[str] = set()

    def respond(self, request: RecordedRequest) -> StubReply:
        route = (request.method, request.path)
        if route == ("GET", "/v1/models"):
            return json_reply(OK, self._models())
        if route == ("POST", "/v1/systemone"):
            return self._system_one(request)
        return json_reply(NOT_FOUND, {"detail": "Not Found"})

    def _models(self) -> dict[str, object]:
        card = {
            "name": "kev-latest",
            "description": f"Kev pointer head serving {self.run}",
            "release_date": "2026-05-01",
            "run": self.run,
            "base": "Qwen/Qwen3.5-0.8B",
            "lora": True,
            "device": "mps",
            "backend": "mlx",
            "dtype": "bfloat16",
            "temperature": 1.0,
            "max_state_tokens": MAX_STATE_TOKENS,
            "truncate_states": False,
            "cuda_graphs": None,
            "prefix_cache": {
                "size": 8,
                "min_state_tokens": 1024,
                "max_tokens": MAX_STATE_TOKENS,
                "hits": self.hits,
                "misses": self.misses,
                "cached_states": len(self._cached_states),
                "oom_retries": 0,
            },
            "batches": {"count": self.hits + self.misses, "requests": self.hits + self.misses, "queued": 0},
        }
        return {"models": [card]}

    def _system_one(self, request: RecordedRequest) -> StubReply:
        behaviour = self.behaviour
        state = state_text(request.body)
        if behaviour.invalid_reply:
            return StubReply(OK, b"{not json", behaviour.hang_seconds)
        if behaviour.max_state_chars is not None and len(state) > behaviour.max_state_chars:
            return json_reply(UNPROCESSABLE, {"detail": STATE_TOO_LARGE_DETAIL}, behaviour.hang_seconds)
        started = time.monotonic()
        if state in self._cached_states:
            self.hits += 1
        else:
            self.misses += 1
            self._cached_states.add(state)
        answers = {
            question.question_id: _answer(
                question, derived_probabilities(state, question.question_id, len(question.options))
            )
            for question in parse_questions(request.body)
        }
        model = json.loads(request.body).get("model", "kev-latest")
        reply = {
            "model": behaviour.echo_model or model,
            "answers": answers,
            "usage": {"input_tokens": len(state), "output_tokens": 1},
            "latency_ms": round((time.monotonic() - started) * 1000, 1),
        }
        return json_reply(OK, reply, behaviour.hang_seconds)
