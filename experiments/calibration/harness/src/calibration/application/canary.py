"""The canary of one model (§5.4): the server's validity, the relay's reading, the confidence formulas and the
state-size probe, on the experiment's own server."""

from __future__ import annotations

import json
import math
from collections.abc import Mapping
from dataclasses import dataclass
from typing import Final

from calibration.application.build_ports import Builders
from calibration.application.exchange import Exchange, ExchangeResult
from calibration.application.ports import ModelServer
from calibration.domain.canonical import JsonValue
from calibration.domain.confidence_formulas import confidence_matches, expected_confidence
from calibration.domain.constants import CHARS_PER_TOKEN_PROBE, ENCODING
from calibration.domain.jsonread import json_object, mapping, text
from calibration.domain.models import ModelSpec
from calibration.domain.ports import DecodeOutcome, DecodeRequest, ProductReader
from calibration.domain.server_wire import (
    CHOICE,
    AnswerView,
    QuestionSpec,
    ReplyClass,
    question_specs,
    read_reply,
)

CANARY_KEY: Final = "canary"
PROBE_SEPARATOR: Final = " "


@dataclass(frozen=True)
class CanaryCheck:
    name: str
    passed: bool
    detail: str

    def to_record(self) -> dict[str, JsonValue]:
        return {"name": self.name, "passed": self.passed, "detail": self.detail}


@dataclass(frozen=True)
class CanaryResult:
    model_id: str
    checks: tuple[CanaryCheck, ...]
    request: str
    response: str
    probe_status: int | None
    probe_response: str

    @property
    def passed(self) -> bool:
        return all(check.passed for check in self.checks)

    def to_record(self) -> dict[str, JsonValue]:
        return {
            "model": self.model_id,
            "passed": self.passed,
            "checks": [check.to_record() for check in self.checks],
            "request": self.request,
            "response": self.response,
            "probe": {"status": self.probe_status, "response": self.probe_response},
        }


def _ordered_probabilities(spec: QuestionSpec, answer: AnswerView) -> list[float]:
    """The probabilities in option order, or in level order for a score keyed by index."""
    keys = [str(index) for index in range(len(spec.options))] if str(0) in answer.probabilities else list(spec.options)
    return [answer.probabilities[key] for key in keys]


class CanaryRunner:
    def __init__(
        self, builders: Builders, reader: ProductReader, exchange: Exchange, server: ModelServer, closed_text: str
    ) -> None:
        self._builders = builders
        self._reader = reader
        self._exchange = exchange
        self._server = server
        self._closed_text = closed_text
        self._specs = question_specs(json.loads(closed_text))

    def run(self, model: ModelSpec) -> CanaryResult:
        body = self._render(self._closed_text, model)
        exchanged = self._exchange.send(self._server.endpoint_url, body.encode(ENCODING), self._specs)
        response = exchanged.final.body.decode(ENCODING, errors="replace")
        checks = [self._validity_check(exchanged)]
        if exchanged.reply_class is ReplyClass.VALID:
            checks.extend(self._reply_checks(model, response, exchanged.final.body))
        probe_status, probe_response, probe_check = self._probe(model)
        return CanaryResult(model.model_id, (*checks, probe_check), body, response, probe_status, probe_response)

    def _render(self, closed_text: str, model: ModelSpec) -> str:
        rendered = self._builders.render_closed(closed_text, [model.identifier])
        return text(mapping(rendered.get("bodies"), "render.bodies").get(model.identifier), "rendered body")

    @staticmethod
    def _validity_check(exchanged: ExchangeResult) -> CanaryCheck:
        passed = exchanged.reply_class is ReplyClass.VALID
        detail = "the reply is server-valid" if passed else f"the reply is {exchanged.reply_class}"
        return CanaryCheck("server-valid", passed, detail)

    def _reply_checks(self, model: ModelSpec, response: str, body: bytes) -> list[CanaryCheck]:
        decoded = self._reader.decode([DecodeRequest(CANARY_KEY, self._closed_text, response)])[0]
        view = read_reply(self._specs, body)
        return [
            self._relay_check(decoded),
            self._document_check(decoded, view.answers),
            self._formula_check(model, view.answers),
        ]

    @staticmethod
    def _relay_check(decoded: DecodeOutcome) -> CanaryCheck:
        detail = "the relay accepts the reply" if decoded.accepted else f"{decoded.stage}: {decoded.error}"
        return CanaryCheck("relay-accepts", decoded.accepted, detail)

    def _document_check(self, decoded: DecodeOutcome, answers: Mapping[str, AnswerView]) -> CanaryCheck:
        """The answer document keeps, for the choice question, the server's probabilities and confidence."""
        kept = {answer.question_id: answer for answer in decoded.answers}
        for spec in self._specs:
            if spec.question_type != CHOICE:
                continue
            stated, document = answers[spec.question_id], kept.get(spec.question_id)
            if document is None or dict(document.probabilities) != dict(stated.probabilities):
                return CanaryCheck("document-keeps-probabilities", False, f"{spec.question_id}: probabilities differ")
            if document.confidence != stated.confidence:
                return CanaryCheck("document-keeps-probabilities", False, f"{spec.question_id}: confidence differs")
        return CanaryCheck("document-keeps-probabilities", True, "the choice answer keeps both")

    def _formula_check(self, model: ModelSpec, answers: Mapping[str, AnswerView]) -> CanaryCheck:
        for spec in self._specs:
            answer = answers[spec.question_id]
            if answer.confidence is None:
                return CanaryCheck("confidence-formula", False, f"{spec.question_id}: no confidence")
            recomputed = expected_confidence(model.flavour, spec.question_type, _ordered_probabilities(spec, answer))
            if not confidence_matches(answer.confidence, recomputed):
                detail = f"{spec.question_id}: stated {answer.confidence}, recomputed {recomputed}"
                return CanaryCheck("confidence-formula", False, detail)
        return CanaryCheck("confidence-formula", True, "every confidence is within the tolerance")

    def _probe(self, model: ModelSpec) -> tuple[int | None, str, CanaryCheck]:
        """A state of more than four characters per token of the loaded context must be refused as too large."""
        context = self._server.card().loaded_context_tokens
        if context is None:
            return None, "", CanaryCheck("state-size-probe", False, "the loaded context length is unknown")
        closed = json_object(json.loads(self._closed_text), "canary")
        state = text(closed.get("state"), "canary state")
        repeats = math.floor(CHARS_PER_TOKEN_PROBE * context / (len(state) + len(PROBE_SEPARATOR))) + 1
        oversized: dict[str, JsonValue] = {**closed, "state": PROBE_SEPARATOR.join([state] * repeats)}
        body = self._render(json.dumps(oversized), model)
        exchanged = self._exchange.send(self._server.endpoint_url, body.encode(ENCODING), self._specs)
        passed = exchanged.reply_class is ReplyClass.STATE_TOO_LARGE
        detail = f"state of {len(str(oversized['state']))} characters for {context} tokens: {exchanged.reply_class}"
        return (
            exchanged.final.status,
            exchanged.final.body.decode(ENCODING, errors="replace"),
            CanaryCheck("state-size-probe", passed, detail),
        )
