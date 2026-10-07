"""Data of a synthetic run directory for the application tests: the lock, the schedule, the items and the replies a
server gives. Nothing here talks to a model, a server or the network."""

from __future__ import annotations

import hashlib
import json
from collections.abc import Collection, Mapping, Sequence
from typing import Final

from calibration.application.layout import item_path, raw_response_path
from calibration.application.ports import RunStore
from calibration.domain.canonical import JsonValue, document_json
from calibration.domain.confidence_formulas import expected_confidence
from calibration.domain.constants import BYTES_PER_GIB, KEV_COMMIT, MODELS_LOCK_FILE, OLLAMA_VERSION, SCHEDULE_FILE
from calibration.domain.items import Item, KindFamily
from calibration.domain.lock import MEMORY_FIT_REASON, LockedModel, PinStatus
from calibration.domain.models import MODELS, Flavour, ModelSpec
from calibration.domain.ports import HttpReply, TransportFailure
from calibration.domain.schedule import Pass, mutant_request_id
from calibration.domain.server_wire import CHOICE, SCORE

TOTAL_MEMORY_BYTES: Final = 24 * BYTES_PER_GIB
FOOTPRINT_BYTES: Final = 2 * BYTES_PER_GIB
KEV_REVISION: Final = "0123456789abcdef0123456789abcdef01234567"
ITEM_QUESTION_ID: Final = "q1"
REQUEST_MEMBER: Final = "request"
HTTP_OK_STATUS: Final = 200
HTTP_SERVER_ERROR: Final = 500
HTTP_PAYLOAD_TOO_LARGE_STATUS: Final = 413
HTTP_UNPROCESSABLE_STATUS: Final = 422
KEV_STATE_TOO_LARGE_BODY: Final = b'{"detail":"state is 70,000 tokens, over the 65,536-token limit"}'
OLLAMA_PROMPT_TOO_LONG_BODY: Final = b"prompt 0 has 99999 tokens; expected 1\xe2\x80\x934096 (input is never truncated)"

CANARY_STATE: Final = "A thermostat is set to 19 degrees. Its sensor reads 23 degrees. The valve is closed."
CANARY_CHOICE_ID: Final = "valve"
CANARY_SCORE_ID: Final = "gap"
CANARY_CHOICE_OPTIONS: Final = ("OPEN", "CLOSED", "UNKNOWN")
CANARY_SCORE_LEVELS: Final = ("AT", "NEAR", "MODERATE", "FAR")
CANARY_CHOICE_PROBABILITIES: Final = (0.1, 0.8, 0.1)
CANARY_SCORE_PROBABILITIES: Final = (0.1, 0.2, 0.6, 0.1)


def _options(names: Sequence[str]) -> list[JsonValue]:
    return [{"name": name, "description": f"{name} option"} for name in names]


def _question(question_id: str, question_type: str, names: Sequence[str]) -> dict[str, JsonValue]:
    return {"id": question_id, "type": question_type, "instructions": f"{question_id}?", "options": _options(names)}


CANARY_CLOSED_TEXT: Final = json.dumps(
    {
        "state": CANARY_STATE,
        "questions": [
            _question(CANARY_CHOICE_ID, CHOICE, CANARY_CHOICE_OPTIONS),
            _question(CANARY_SCORE_ID, SCORE, CANARY_SCORE_LEVELS),
        ],
    }
)
ITEM_CLOSED: Final[dict[str, JsonValue]] = {
    "state": "A request state.",
    "questions": [_question(ITEM_QUESTION_ID, CHOICE, ("A", "B"))],
}
ITEM_REPLY: Final = HttpReply(
    HTTP_OK_STATUS,
    json.dumps(
        {
            "model": "served",
            "answers": {
                ITEM_QUESTION_ID: {
                    "type": CHOICE,
                    "choice": "A",
                    "probabilities": {"A": 0.75, "B": 0.25},
                    "confidence": 0.5,
                }
            },
        }
    ).encode(),
)
INVALID_REPLY: Final = HttpReply(HTTP_SERVER_ERROR, b"internal error")


def pinned_version(flavour: Flavour) -> str:
    return OLLAMA_VERSION if flavour is Flavour.OLLAMA else KEV_COMMIT


def digest_of(model_id: str) -> str:
    return f"digest-{model_id}"


def _locked(model: ModelSpec, blocked: Collection[str], footprints: Mapping[str, int]) -> dict[str, JsonValue]:
    is_blocked = model.model_id in blocked
    reference = None if model.is_ollama else f"{model.repository}@{KEV_REVISION}"
    return LockedModel(
        model.model_id,
        PinStatus.BLOCKED if is_blocked else PinStatus.PINNED,
        MEMORY_FIT_REASON if is_blocked else None,
        footprints.get(model.model_id, FOOTPRINT_BYTES),
        digest_of(model.model_id),
        reference,
    ).to_record()


def lock_record(
    *,
    blocked: Collection[str] = (),
    footprints: Mapping[str, int] | None = None,
    total_memory_bytes: int = TOTAL_MEMORY_BYTES,
) -> dict[str, JsonValue]:
    """A `models.lock.json` that pins every model not named in `blocked` at the versions of the document."""
    sizes = footprints or {}
    ollama = {m.model_id: _locked(m, blocked, sizes) for m in MODELS if m.is_ollama}
    kev = {m.model_id: _locked(m, blocked, sizes) for m in MODELS if not m.is_ollama}
    return {
        "total_memory_bytes": total_memory_bytes,
        "ollama": {"version": OLLAMA_VERSION, "models": ollama},
        "kev": {"commit": KEV_COMMIT, "models": kev},
    }


def request_ids(count: int) -> list[str]:
    return [mutant_request_id(f"{index:016x}") for index in range(count)]


def item_body(identifier: str, request_id: str) -> str:
    return json.dumps({"model": identifier, REQUEST_MEMBER: request_id})


def item_for(request_id: str) -> Item:
    return Item(
        request_id=request_id,
        family=KindFamily.SCREEN,
        unit="unit",
        closed=ITEM_CLOSED,
        closed_digest=hashlib.sha256(request_id.encode()).hexdigest(),
        template_digest="template",
        bodies={model.identifier: item_body(model.identifier, request_id) for model in MODELS},
        mutant_id=request_id.split("|", 1)[1],
        question_to_ref={ITEM_QUESTION_ID: "ref"},
    )


def seed_run(
    store: RunStore,
    *,
    pass1: Sequence[str],
    pass2: Sequence[str] = (),
    lock: Mapping[str, JsonValue] | None = None,
) -> None:
    """Writes the lock, the schedule and one item per request id, the files a block and a session read."""
    store.write_text(MODELS_LOCK_FILE, document_json(lock if lock is not None else lock_record()))
    store.write_text(SCHEDULE_FILE, document_json({"pass1": list(pass1), "pass2": list(pass2)}))
    for request_id in dict.fromkeys([*pass1, *pass2]):
        store.write_text(item_path(request_id), document_json(item_for(request_id).to_record()))


def seed_responses(store: RunStore, model_id: str, which: Pass, ids: Sequence[str]) -> None:
    """Marks these requests of a model as answered: their `response.bin` exists."""
    for request_id in ids:
        store.write_bytes(raw_response_path(model_id, which, request_id), ITEM_REPLY.body)


def posted_request_ids(posts: Sequence[bytes]) -> list[str]:
    """The request ids of the item bodies among the posted bodies, in order."""
    documents = [json.loads(body) for body in posts]
    return [document[REQUEST_MEMBER] for document in documents if REQUEST_MEMBER in document]


def too_large_reply(flavour: Flavour) -> HttpReply:
    if flavour is Flavour.OLLAMA:
        return HttpReply(HTTP_PAYLOAD_TOO_LARGE_STATUS, b"")
    return HttpReply(HTTP_UNPROCESSABLE_STATUS, KEV_STATE_TOO_LARGE_BODY)


def _canary_answer(
    flavour: Flavour, question_type: str, keys: Sequence[str], values: Sequence[float], shift: float
) -> dict[str, JsonValue]:
    confidence = expected_confidence(flavour, question_type, values) + shift
    answer: dict[str, JsonValue] = {
        "type": question_type,
        "probabilities": dict(zip(keys, values, strict=True)),
        "confidence": confidence,
    }
    if question_type == CHOICE:
        answer[CHOICE] = CANARY_CHOICE_OPTIONS[1]
    else:
        answer[SCORE] = 2
    return answer


def canary_reply(flavour: Flavour, *, choice_shift: float = 0.0, score_shift: float = 0.0) -> HttpReply:
    """A server-valid canary reply whose confidences are the formula's, plus a shift."""
    score_keys = [str(index) for index in range(len(CANARY_SCORE_LEVELS))]
    answers = {
        CANARY_CHOICE_ID: _canary_answer(
            flavour, CHOICE, CANARY_CHOICE_OPTIONS, CANARY_CHOICE_PROBABILITIES, choice_shift
        ),
        CANARY_SCORE_ID: _canary_answer(flavour, SCORE, score_keys, CANARY_SCORE_PROBABILITIES, score_shift),
    }
    return HttpReply(HTTP_OK_STATUS, json.dumps({"model": "served", "answers": answers}).encode())


class ServerBehaviour:
    """Answers each posted body the way a conforming server does: the canary, its oversized probe and the items.

    A request id of `queued` is answered from its queue first, which is how a test scripts a failing request.
    """

    def __init__(
        self,
        flavour: Flavour,
        *,
        canary: HttpReply | None = None,
        probe: HttpReply | None = None,
        queued: Mapping[str, Sequence[HttpReply | TransportFailure]] | None = None,
    ) -> None:
        self.canary = canary or canary_reply(flavour)
        self._probe = probe or too_large_reply(flavour)
        self._queued = {request_id: list(replies) for request_id, replies in (queued or {}).items()}

    def __call__(self, body: bytes) -> HttpReply | TransportFailure:
        document = json.loads(body)
        if "state" in document:
            return self._probe if len(document["state"]) > len(CANARY_STATE) else self.canary
        queue = self._queued.get(document[REQUEST_MEMBER], [])
        return queue.pop(0) if queue else ITEM_REPLY
