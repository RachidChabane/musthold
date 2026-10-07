"""Hand-built objects of a tiny world for the source-(1) and source-(2) decision builders."""

from __future__ import annotations

import json
from collections.abc import Mapping
from typing import Final

from calibration.analysis.replies import ANSWERED, ReplyOutcome
from calibration.analysis.truth import MatrixInputs, MutantMeta, PilotTruth, TestLabel
from calibration.analysis.votes import JudgeVote, ValidityVote
from calibration.domain.items import Item, KindFamily
from calibration.domain.ports import DecodedAnswer
from calibration.domain.schedule import alignment_request_id, mutant_request_id
from calibration.domain.server_wire import AnswerView, ReplyView

REF_ONE: Final = "PARK-1~1"
REF_TWO: Final = "PARK-2~1"
REF_THREE: Final = "PARK-3~1"
TOKEN_ONE: Final = "K-1"
TOKEN_TWO: Final = "K-2"
TOKEN_THREE: Final = "K-3"
NONE: Final = "NONE"
LEVELS: Final = ("NONE", "WEAK", "PARTIAL", "FULL")
CRITERION: Final = "criterion"
COVERAGE: Final = "coverage"
VIOLATES: Final = "VIOLATES"
CONSISTENT: Final = "CONSISTENT"
MODEL_NAME: Final = "tiny:model"
NODE_GOOD: Final = "tests/a_good.py::t_good"
NODE_MIS: Final = "tests/b_mis.py::t_mis"
NODE_VAC: Final = "tests/c_vac.py::t_vac"
MUTANT_T: Final = "mT"
MUTANT_P: Final = "mP"
MUTANT_R: Final = "mR"
MUTANT_F: Final = "mF"
MUTANT_N: Final = "mN"
CALL_T: Final = "gen-7"
CALL_F: Final = "gen-8"
TEMPLATE: Final = "template"
CLOSED_DIGEST: Final = "digest"
ALIGNMENT_BODY: Final = {"M": "body"}
CRITERION_OPTIONS: Final = (TOKEN_ONE, TOKEN_TWO, TOKEN_THREE, NONE)
SCREEN_QUESTIONS: Final = {"q1": REF_ONE, "q2": REF_TWO}


def label(nodeid: str, cites: str, label_class: str, checks: tuple[str, ...]) -> TestLabel:
    aligned = "ALIGNED" if label_class in ("GOOD", "GOOD_BROAD") else "NOT_ALIGNED"
    return TestLabel(nodeid, cites, label_class, checks, aligned)


LABELS: Final = (
    label(NODE_GOOD, REF_ONE, "GOOD", (REF_ONE,)),
    label(NODE_MIS, REF_TWO, "MIS_OTHER", (REF_THREE,)),
    label(NODE_VAC, REF_TWO, "VACUOUS", ()),
)


def _closed(questions: list[dict[str, object]]) -> dict[str, object]:
    return {"state": "state", "questions": questions}


def alignment_item(nodeid: str, declared: str) -> Item:
    closed = _closed(
        [
            {"id": CRITERION, "type": "choice", "options": [{"name": name} for name in CRITERION_OPTIONS]},
            {"id": COVERAGE, "type": "score", "options": [{"name": name} for name in LEVELS]},
        ]
    )
    return Item(
        request_id=alignment_request_id(nodeid),
        family=KindFamily.ALIGNMENT,
        unit=nodeid,
        closed=json.loads(json.dumps(closed)),
        closed_digest=CLOSED_DIGEST,
        template_digest=TEMPLATE,
        bodies=ALIGNMENT_BODY,
        nodeid=nodeid,
        declared=declared,
        token_to_ref={TOKEN_ONE: REF_ONE, TOKEN_TWO: REF_TWO, TOKEN_THREE: REF_THREE},
    )


def screen_item(mutant_id: str) -> Item:
    closed = _closed(
        [
            {"id": question, "type": "choice", "options": [{"name": VIOLATES}, {"name": CONSISTENT}]}
            for question in SCREEN_QUESTIONS
        ]
    )
    return Item(
        request_id=mutant_request_id(mutant_id),
        family=KindFamily.SCREEN,
        unit=mutant_id,
        closed=json.loads(json.dumps(closed)),
        closed_digest=CLOSED_DIGEST,
        template_digest=TEMPLATE,
        bodies=ALIGNMENT_BODY,
        mutant_id=mutant_id,
        question_to_ref=dict(SCREEN_QUESTIONS),
    )


ALIGNMENT_ITEMS: Final = {
    item.request_id: item
    for item in (
        alignment_item(NODE_GOOD, REF_ONE),
        alignment_item(NODE_MIS, REF_TWO),
        alignment_item(NODE_VAC, REF_TWO),
    )
}
SCREEN_MUTANTS: Final = (MUTANT_F, MUTANT_N, MUTANT_P, MUTANT_R, MUTANT_T)
SCREEN_ITEMS: Final = {item.request_id: item for item in (screen_item(m) for m in SCREEN_MUTANTS)}


def fall_through(request_id: str, status: str) -> ReplyOutcome:
    return ReplyOutcome(request_id, status, {}, None, None, None)


def _level_probabilities(weights: tuple[float, ...]) -> dict[str, float]:
    return {str(index): weight for index, weight in enumerate(weights)}


def alignment_outcome(
    nodeid: str,
    criterion: tuple[str, float, dict[str, float]],
    coverage: tuple[str, float, tuple[float, ...]],
) -> ReplyOutcome:
    """An answered alignment request: (choice, confidence, probabilities) and (level, confidence, level weights)."""
    request_id = alignment_request_id(nodeid)
    choice, choice_confidence, choice_probabilities = criterion
    level, level_confidence, weights = coverage
    answers = {
        CRITERION: DecodedAnswer(CRITERION, choice, choice_confidence, choice_probabilities),
        COVERAGE: DecodedAnswer(COVERAGE, level, level_confidence, {}),
    }
    views = {
        CRITERION: AnswerView(CRITERION, "choice", choice, None, choice_confidence, choice_probabilities),
        COVERAGE: AnswerView(
            COVERAGE, "score", None, float(LEVELS.index(level)), level_confidence, _level_probabilities(weights)
        ),
    }
    return ReplyOutcome(request_id, ANSWERED, answers, ReplyView(MODEL_NAME, views, None), None, None)


def screen_document(answers: Mapping[str, tuple[str, float | None]]) -> str:
    """An answer document: per question the choice and, when given, the probability of VIOLATES."""
    entries: list[dict[str, object]] = []
    for question, (choice, violates) in answers.items():
        entry: dict[str, object] = {"id": question, "choice": choice}
        if violates is not None:
            entry["probabilities"] = {VIOLATES: violates, CONSISTENT: 1.0 - violates}
        entries.append(entry)
    return json.dumps({"answers": entries})


def screen_outcome(mutant_id: str, document: str) -> ReplyOutcome:
    return ReplyOutcome(mutant_request_id(mutant_id), ANSWERED, {}, None, document, None)


def meta(mutant_id: str, arm: str, target: str | None, call: str | None) -> MutantMeta:
    return MutantMeta(mutant_id, arm, target, call)


def truth(
    *,
    consensus: frozenset[str] = frozenset(),
    alignment_votes: Mapping[str, Mapping[int, JudgeVote]] | None = None,
    validity_votes: Mapping[str, Mapping[str, Mapping[str, ValidityVote]]] | None = None,
) -> PilotTruth:
    """The tiny pilot truth: three tests, five mutants, two bound tests and their kills."""
    return PilotTruth(
        labels=LABELS,
        consensus=consensus,
        arm_p_sources={MUTANT_P: 3, MUTANT_R: 1},
        bindings={"tA": REF_ONE, "tB": REF_TWO},
        processing=(),
        differential={MUTANT_T: {"status": "DIFFERS"}, MUTANT_F: {"status": "SKIPPED"}},
        mutants={
            MUTANT_T: meta(MUTANT_T, "T", REF_ONE, CALL_T),
            MUTANT_F: meta(MUTANT_F, "T", REF_TWO, CALL_F),
            MUTANT_P: meta(MUTANT_P, "P", REF_THREE, None),
            MUTANT_R: meta(MUTANT_R, "P", REF_ONE, None),
            MUTANT_N: meta(MUTANT_N, "T", REF_ONE, CALL_T),
        },
        oracle={
            MUTANT_T: frozenset({1}),
            MUTANT_F: frozenset({2}),
            MUTANT_P: frozenset[int](),
            MUTANT_R: frozenset[int](),
        },
        kills={MUTANT_T: {"tA": "killed", "tB": "survived"}},
        alignment_votes=alignment_votes or {},
        validity_votes=validity_votes or {},
        matrix=MatrixInputs((), ()),
    )
