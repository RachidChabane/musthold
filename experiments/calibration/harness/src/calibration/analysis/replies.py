"""Source (1): the recorded replies of one model, read the way the product reads them (§4.6)."""

from __future__ import annotations

import json
from collections.abc import Mapping
from dataclasses import dataclass

from calibration.analysis.model import (
    FALL_THROUGH_MISSING,
    FALL_THROUGH_RELAY_REFUSED,
    FALL_THROUGH_STATE_TOO_LARGE,
)
from calibration.analysis.run_data import RunData
from calibration.domain.constants import ENCODING
from calibration.domain.items import Item
from calibration.domain.models import ModelSpec
from calibration.domain.ports import DecodedAnswer, DecodeRequest, ProductReader
from calibration.domain.schedule import Pass
from calibration.domain.server_wire import (
    ReplyClass,
    ReplyView,
    classify_reply,
    question_specs,
    read_reply,
)

ANSWERED: str = "ANSWERED"


@dataclass(frozen=True)
class ReplyOutcome:
    """What one request became: a fall-through type, or the product's answers with the server's own reading."""

    request_id: str
    status: str
    answers: Mapping[str, DecodedAnswer]
    returned: ReplyView | None
    document: str | None
    cached_hit: bool | None
    model_reported: str | None = None

    @property
    def answered(self) -> bool:
        return self.status == ANSWERED


def _fall_through(request_id: str, status: str) -> ReplyOutcome:
    return ReplyOutcome(request_id, status, {}, None, None, None)


def read_outcomes(run: RunData, model: ModelSpec, which: Pass, reader: ProductReader) -> dict[str, ReplyOutcome]:
    """The outcome of every request of a pass: MISSING and STATE_TOO_LARGE from the bytes, RELAY_REFUSED and the
    answers from the product's own decoder."""
    outcomes: dict[str, ReplyOutcome] = {}
    pending: dict[str, tuple[Item, str, bytes, bool | None]] = {}
    for request_id in run.schedule.ids_of(which):
        item = run.items[request_id]
        exchange = run.exchange(model.model_id, which, request_id)
        if exchange is None:
            outcomes[request_id] = _fall_through(request_id, FALL_THROUGH_MISSING)
            continue
        specs = question_specs(item.closed)
        reply_class = classify_reply(model.flavour, specs, exchange.meta.http_status, exchange.response)
        if reply_class is ReplyClass.STATE_TOO_LARGE:
            outcomes[request_id] = _fall_through(request_id, FALL_THROUGH_STATE_TOO_LARGE)
        elif reply_class is ReplyClass.INVALID:
            outcomes[request_id] = _fall_through(request_id, FALL_THROUGH_MISSING)
        else:
            body = exchange.response
            pending[request_id] = (item, body.decode(ENCODING), body, exchange.meta.prefix_cache_hit)
    decoded = reader.decode(
        [DecodeRequest(request_id, json.dumps(item.closed), text) for request_id, (item, text, _, _) in pending.items()]
    )
    for outcome in decoded:
        item, _, body, hit = pending[outcome.key]
        if not outcome.accepted:
            outcomes[outcome.key] = _fall_through(outcome.key, FALL_THROUGH_RELAY_REFUSED)
            continue
        view = read_reply(question_specs(item.closed), body)
        answers = {answer.question_id: answer for answer in outcome.answers}
        outcomes[outcome.key] = ReplyOutcome(outcome.key, ANSWERED, answers, view, outcome.document, hit, outcome.model)
    return outcomes
