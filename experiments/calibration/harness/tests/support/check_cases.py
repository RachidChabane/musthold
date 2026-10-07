"""Hand-built decisions, cells, items and replies shared by the analysis unit and property tests."""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from typing import Final

from calibration.analysis.model import CellData, Decision, Kind
from calibration.analysis.replies import ANSWERED, ReplyOutcome
from calibration.domain.canonical import JsonValue
from calibration.domain.items import Item, KindFamily
from calibration.domain.ports import DecodedAnswer
from calibration.domain.server_wire import AnswerView, ReplyView
from calibration.domain.sources import SourceId

CRITERION_QUESTION: Final = "criterion"
COVERAGE_QUESTION: Final = "coverage"
CHOICE_OPTIONS: Final = ("K-0001", "K-0002", "NONE")
LEVELS: Final = ("NONE", "WEAK", "PARTIAL", "FULL")
SCREEN_QUESTION: Final = "q1"
SCREEN_OPTIONS: Final = ("VIOLATES", "CONSISTENT")
SCREEN_REF: Final = "PARK-1~1"
MODEL_ID: Final = "M1"
DIGEST: Final = "0" * 64


def decision(
    unit: str,
    cluster: str,
    confidence: float,
    correct: bool,
    *,
    event: float | None = None,
    truth: bool | None = None,
    predicted: bool | None = None,
    has_prediction: bool = True,
    alt: float | None = None,
    alt_event: float | None = None,
    lenient: bool | None = None,
    consensus: bool = True,
    tags: Mapping[str, str] | None = None,
) -> Decision:
    return Decision(
        unit=unit,
        cluster=cluster,
        confidence=confidence,
        correct=correct,
        event_probability=event,
        truth_positive=truth,
        predicted_positive=predicted,
        has_prediction=has_prediction,
        alt_confidence=alt,
        alt_event_probability=alt_event,
        lenient_correct=lenient,
        consensus=consensus,
        tags=dict(tags or {}),
    )


def cell(
    kind: Kind,
    decisions: Sequence[Decision],
    *,
    source: SourceId = SourceId.S1,
    model: str = MODEL_ID,
    units: int | None = None,
    blocked_reason: str | None = None,
) -> CellData:
    return CellData(
        kind=kind,
        source=source,
        model=model,
        confidence_source=None,
        decisions=tuple(decisions),
        units=len(decisions) if units is None else units,
        unit_clusters=tuple(d.cluster for d in decisions),
        blocked_reason=blocked_reason,
    )


def _closed() -> dict[str, JsonValue]:
    def options(names: Sequence[str]) -> list[JsonValue]:
        return [{"name": name, "description": name} for name in names]

    return {
        "state": "state",
        "questions": [
            {"id": CRITERION_QUESTION, "type": "choice", "instructions": "i", "options": options(CHOICE_OPTIONS)},
            {"id": COVERAGE_QUESTION, "type": "score", "instructions": "i", "options": options(LEVELS)},
        ],
    }


def alignment_item(request_id: str = "A|t") -> Item:
    return Item(
        request_id, KindFamily.ALIGNMENT, "tests/t.py::t", _closed(), DIGEST, DIGEST, {}, nodeid="tests/t.py::t"
    )


def screen_item(request_id: str = "M|m") -> Item:
    closed: dict[str, JsonValue] = {
        "state": "state",
        "questions": [
            {
                "id": SCREEN_QUESTION,
                "type": "choice",
                "instructions": "i",
                "options": [{"name": name, "description": name} for name in SCREEN_OPTIONS],
            }
        ],
    }
    return Item(
        request_id,
        KindFamily.SCREEN,
        "m",
        closed,
        DIGEST,
        DIGEST,
        {},
        mutant_id="m",
        question_to_ref={SCREEN_QUESTION: SCREEN_REF},
    )


def choice_view(question: str, choice: str, confidence: float | None, probabilities: Mapping[str, float]) -> AnswerView:
    return AnswerView(question, "choice", choice, None, confidence, dict(probabilities))


def score_view(question: str, score: float, confidence: float | None, probabilities: Mapping[str, float]) -> AnswerView:
    return AnswerView(question, "score", None, score, confidence, dict(probabilities))


def outcome(
    request_id: str, views: Sequence[AnswerView], answers: Mapping[str, DecodedAnswer] | None = None
) -> ReplyOutcome:
    reply = ReplyView("m", {view.question_id: view for view in views}, None)
    return ReplyOutcome(request_id, ANSWERED, dict(answers or {}), reply, "{}", None)
