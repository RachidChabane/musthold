"""The items of the synthetic run: closed question sets, the request bodies of every model and the truth plans."""

from __future__ import annotations

import json
from dataclasses import dataclass
from typing import Final

from calibration.domain.canonical import JsonValue, sha256_hex
from calibration.domain.items import Item, KindFamily
from calibration.domain.models import identifiers
from calibration.domain.pilot_records import criterion_number
from calibration.domain.schedule import alignment_request_id, mutant_request_id
from support.world_model import (
    CHOICE_CONSISTENT,
    CHOICE_VIOLATES,
    COVERAGE_LEVELS,
    CRITERION_REFS,
    ITEM_TESTS,
    MEASURED_MUTANTS,
    NONE_OPTION,
    SCREEN_OPTIONS,
    TOKEN_HEX_DIGITS,
    MutantSpec,
    TestSpec,
)
from support.world_pilot import host_k1_answer
from support.world_wire import QuestionPlan

TEMPLATE_DIGEST: Final = sha256_hex("synthetic decide.closed template")
SEPARATORS: Final = (",", ":")
CRITERION_QUESTION: Final = "criterion"
COVERAGE_QUESTION: Final = "coverage"
CRITERION_INSTRUCTIONS: Final = "Which requirement do the assertions of this test verify, if any?"
COVERAGE_INSTRUCTIONS: Final = "How completely do the assertions cover that requirement?"
SCREEN_INSTRUCTIONS: Final = "Does the change violate the requirement?"
WITNESS_MAPPED: Final = "mapped"
LEVEL_DESCRIPTIONS: Final = {
    "NONE": "the requirement is not checked",
    "WEAK": "the requirement is checked by a property the requirement does not need",
    "PARTIAL": "part of the requirement is checked",
    "FULL": "the requirement is checked completely",
}
SCREEN_DESCRIPTIONS: Final = {
    CHOICE_VIOLATES: "the change breaks the requirement",
    CHOICE_CONSISTENT: "the change keeps the requirement",
}


@dataclass(frozen=True)
class PlannedItem:
    """An item with the plan of its questions."""

    item: Item
    plans: tuple[QuestionPlan, ...]


def token_of(nodeid: str, ref: str) -> str:
    return "K-" + sha256_hex(f"{nodeid}|{ref}")[:TOKEN_HEX_DIGITS]


@dataclass(frozen=True)
class ClosedQuestion:
    question_id: str
    question_type: str
    instructions: str
    options: tuple[tuple[str, str], ...]

    def closed_record(self) -> dict[str, JsonValue]:
        return {
            "id": self.question_id,
            "type": self.question_type,
            "instructions": self.instructions,
            "options": [{"name": name, "description": description} for name, description in self.options],
        }

    def body_record(self) -> dict[str, JsonValue]:
        criteria: JsonValue = (
            [description for _, description in self.options] if self.question_type == "score" else dict(self.options)
        )
        return {"type": self.question_type, "instructions": self.instructions, "criteria": criteria}

    @property
    def names(self) -> tuple[str, ...]:
        return tuple(name for name, _ in self.options)


def _closed(state: str, questions: tuple[ClosedQuestion, ...]) -> dict[str, JsonValue]:
    return {"state": state, "questions": [question.closed_record() for question in questions]}


def _body(identifier: str, state: str, questions: tuple[ClosedQuestion, ...]) -> str:
    body: dict[str, JsonValue] = {
        "model": identifier,
        "state": state,
        "questions": {question.question_id: question.body_record() for question in questions},
    }
    return json.dumps(body, ensure_ascii=False, separators=SEPARATORS)


@dataclass(frozen=True)
class ItemFields:
    nodeid: str | None = None
    label: str | None = None
    declared: str | None = None
    token_to_ref: dict[str, str] | None = None
    mutant_id: str | None = None
    question_to_ref: dict[str, str] | None = None
    witness_status: str | None = None


def _item(
    request_id: str,
    unit: str,
    state: str,
    questions: tuple[ClosedQuestion, ...],
    family: KindFamily,
    fields: ItemFields,
) -> Item:
    closed_text = json.dumps(_closed(state, questions), ensure_ascii=False, separators=SEPARATORS)
    return Item(
        request_id=request_id,
        family=family,
        unit=unit,
        closed=json.loads(closed_text),
        closed_digest=sha256_hex(closed_text),
        template_digest=TEMPLATE_DIGEST,
        bodies={identifier: _body(identifier, state, questions) for identifier in identifiers()},
        nodeid=fields.nodeid,
        label=fields.label,
        declared=fields.declared,
        token_to_ref=fields.token_to_ref,
        mutant_id=fields.mutant_id,
        question_to_ref=fields.question_to_ref,
        witness_status=fields.witness_status,
    )


def _coverage_target(test: TestSpec) -> int:
    if test.label in ("GOOD", "GOOD_BROAD"):
        return COVERAGE_LEVELS.index("FULL")
    if test.label == "MIS_OTHER":
        return COVERAGE_LEVELS.index("PARTIAL")
    return COVERAGE_LEVELS.index("WEAK")


def alignment_item(test: TestSpec) -> PlannedItem:
    tokens = {ref: token_of(test.nodeid, ref) for ref in CRITERION_REFS}
    criterion = ClosedQuestion(
        CRITERION_QUESTION,
        "choice",
        CRITERION_INSTRUCTIONS,
        (
            *((token, f"the requirement {ref}") for ref, token in tokens.items()),
            (NONE_OPTION, "none of the requirements"),
        ),
    )
    coverage = ClosedQuestion(
        COVERAGE_QUESTION,
        "score",
        COVERAGE_INSTRUCTIONS,
        tuple((level, LEVEL_DESCRIPTIONS[level]) for level in COVERAGE_LEVELS),
    )
    answer = host_k1_answer(test)
    target = criterion.names.index(NONE_OPTION if answer == NONE_OPTION else tokens[answer])
    state = f"def {test.nodeid.split('::')[-1]}(): assert charge(1) == 2"
    fields = ItemFields(
        nodeid=test.nodeid,
        label=test.label,
        declared=test.cites_ref,
        token_to_ref={token: ref for ref, token in tokens.items()},
    )
    item = _item(
        alignment_request_id(test.nodeid), test.nodeid, state, (criterion, coverage), KindFamily.ALIGNMENT, fields
    )
    plans = (
        QuestionPlan(CRITERION_QUESTION, "choice", criterion.names, target),
        QuestionPlan(COVERAGE_QUESTION, "score", coverage.names, _coverage_target(test)),
    )
    return PlannedItem(item, plans)


def screen_item(mutant: MutantSpec) -> PlannedItem:
    question_ids = [f"q{index + 1}" for index in range(len(CRITERION_REFS))]
    questions = tuple(
        ClosedQuestion(
            question_id,
            "choice",
            SCREEN_INSTRUCTIONS,
            tuple((name, SCREEN_DESCRIPTIONS[name]) for name in SCREEN_OPTIONS),
        )
        for question_id in question_ids
    )
    state = f"mutant {mutant.mutant_id}: charge(x) now returns x + 1"
    fields = ItemFields(
        mutant_id=mutant.mutant_id,
        question_to_ref=dict(zip(question_ids, CRITERION_REFS, strict=True)),
        witness_status=WITNESS_MAPPED,
    )
    item = _item(mutant_request_id(mutant.mutant_id), mutant.mutant_id, state, questions, KindFamily.SCREEN, fields)
    plans = tuple(
        QuestionPlan(
            question_id,
            "choice",
            SCREEN_OPTIONS,
            0 if criterion_number(ref) in (mutant.failed or ()) else 1,
        )
        for question_id, ref in zip(question_ids, CRITERION_REFS, strict=True)
    )
    return PlannedItem(item, plans)


def planned_items() -> tuple[PlannedItem, ...]:
    """Every item of the world: one alignment item per shown test, one screen item per measured mutant."""
    return (
        *(alignment_item(test) for test in ITEM_TESTS),
        *(screen_item(mutant) for mutant in MEASURED_MUTANTS),
    )
