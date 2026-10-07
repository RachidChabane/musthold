from __future__ import annotations

import json
from typing import ClassVar

import pytest

from calibration.domain.constants import (
    HTTP_BAD_REQUEST,
    HTTP_OK,
    HTTP_PAYLOAD_TOO_LARGE,
    HTTP_UNPROCESSABLE,
)
from calibration.domain.jsonread import RecordError
from calibration.domain.models import Flavour
from calibration.domain.server_wire import (
    CHOICE,
    SCORE,
    QuestionSpec,
    ReplyClass,
    classify_reply,
    is_state_too_large,
    level_probability,
    question_specs,
    read_reply,
    reply_problems,
)
from fixtures.wire_data import RECORDED_KEV_CHOICE_REPLY, RECORDED_OLLAMA_CHOICE_REPLY

COLOR_OPTIONS = ("blue", "green", "grey")
LEVELS = ("low", "mid", "high")
COLOR = QuestionSpec("color", CHOICE, COLOR_OPTIONS)
RATING = QuestionSpec("rating", SCORE, LEVELS)
OLLAMA_TOO_MANY_TOKENS = "prompt 0 has 9000 tokens; expected 1\u20134096 (input is never truncated)"
OLLAMA_TOO_MANY_TOKENS_ASCII_DASH = "prompt 0 has 9000 tokens; expected 1-4096 (input is never truncated)"
OLLAMA_NO_CONTEXT = "prompt 0 requires 9000 context tokens for scoring; model has 4096"
KEV_STATE_LENGTH = "state is 70,000 tokens, over the 65,536-token limit"


def _answer(question_type: str, probabilities: dict[str, float], **members: object) -> dict[str, object]:
    return {"type": question_type, "probabilities": probabilities, "confidence": 0.5, **members}


def _choice_answer(probabilities: dict[str, float] | None = None) -> dict[str, object]:
    shares = probabilities or {"blue": 0.2, "green": 0.7, "grey": 0.1}
    return _answer(CHOICE, shares, choice="green")


def _score_answer(probabilities: dict[str, float]) -> dict[str, object]:
    return _answer(SCORE, probabilities, score=1.2)


def _body(answers: dict[str, object]) -> bytes:
    return json.dumps({"model": "m", "answers": answers}).encode()


def _reply_problems(spec: QuestionSpec, answer: object) -> tuple[str, ...]:
    return reply_problems((spec,), HTTP_OK, _body({spec.question_id: answer}))


class TestStateTooLarge:
    @pytest.mark.parametrize("message", [OLLAMA_TOO_MANY_TOKENS, OLLAMA_TOO_MANY_TOKENS_ASCII_DASH, OLLAMA_NO_CONTEXT])
    def test_ollama_400_with_either_documented_message(self, message: str) -> None:
        assert is_state_too_large(Flavour.OLLAMA, HTTP_BAD_REQUEST, message.encode())

    def test_ollama_400_with_another_message_is_not_a_state_size_refusal(self) -> None:
        assert not is_state_too_large(Flavour.OLLAMA, HTTP_BAD_REQUEST, b"invalid request")

    def test_ollama_413_is_a_state_size_refusal_whatever_the_body(self) -> None:
        assert is_state_too_large(Flavour.OLLAMA, HTTP_PAYLOAD_TOO_LARGE, b"")

    def test_ollama_state_size_message_under_another_status_is_not_one(self) -> None:
        assert not is_state_too_large(Flavour.OLLAMA, HTTP_UNPROCESSABLE, OLLAMA_NO_CONTEXT.encode())

    def test_kev_422_with_a_state_length_message(self) -> None:
        assert is_state_too_large(Flavour.KEV, HTTP_UNPROCESSABLE, KEV_STATE_LENGTH.encode())

    def test_kev_422_that_is_not_a_state_length(self) -> None:
        assert not is_state_too_large(Flavour.KEV, HTTP_UNPROCESSABLE, b'{"detail":"questions must not be empty"}')

    def test_kev_does_not_use_the_ollama_forms(self) -> None:
        assert not is_state_too_large(Flavour.KEV, HTTP_PAYLOAD_TOO_LARGE, b"")
        assert not is_state_too_large(Flavour.KEV, HTTP_BAD_REQUEST, OLLAMA_NO_CONTEXT.encode())

    @pytest.mark.parametrize("flavour", list(Flavour))
    def test_a_transport_failure_is_never_a_state_size_refusal(self, flavour: Flavour) -> None:
        assert not is_state_too_large(flavour, None, KEV_STATE_LENGTH.encode())

    def test_undecodable_bytes_do_not_raise(self) -> None:
        assert not is_state_too_large(Flavour.KEV, HTTP_UNPROCESSABLE, b"\xff\xfe")


class TestServerValidity:
    def test_a_well_formed_choice_is_valid(self) -> None:
        assert _reply_problems(COLOR, _choice_answer()) == ()

    def test_the_recorded_replies_are_valid(self) -> None:
        for body in (RECORDED_OLLAMA_CHOICE_REPLY, RECORDED_KEV_CHOICE_REPLY):
            assert reply_problems((COLOR,), HTTP_OK, body.encode()) == ()

    def test_a_status_other_than_200_is_a_problem(self) -> None:
        assert reply_problems((COLOR,), HTTP_BAD_REQUEST, b"{}") == (f"HTTP status {HTTP_BAD_REQUEST}",)

    def test_a_transport_failure_is_a_problem(self) -> None:
        assert reply_problems((COLOR,), None, b"") == ("HTTP status None",)

    @pytest.mark.parametrize("body", [b"not json", b"\xff", b"[]", b"{}", b'{"answers":[]}'])
    def test_a_body_that_is_not_a_reply_object_is_a_problem(self, body: bytes) -> None:
        assert reply_problems((COLOR,), HTTP_OK, body)

    def test_a_missing_answer_is_a_problem(self) -> None:
        assert reply_problems((COLOR,), HTTP_OK, _body({}))

    def test_an_answer_of_the_wrong_type_is_a_problem(self) -> None:
        assert _reply_problems(COLOR, _score_answer({"0": 0.2, "1": 0.7, "2": 0.1}))

    def test_a_choice_outside_the_options_is_a_problem(self) -> None:
        answer = _answer(CHOICE, {"blue": 0.2, "green": 0.7, "grey": 0.1}, choice="red")
        assert _reply_problems(COLOR, answer)

    def test_probabilities_with_extra_or_missing_keys_are_a_problem(self) -> None:
        assert _reply_problems(COLOR, _choice_answer({"blue": 0.5, "green": 0.5}))
        assert _reply_problems(COLOR, _choice_answer({"blue": 0.2, "green": 0.6, "grey": 0.1, "red": 0.1}))

    def test_probabilities_that_do_not_sum_to_one_are_a_problem(self) -> None:
        assert _reply_problems(COLOR, _choice_answer({"blue": 0.2, "green": 0.5, "grey": 0.1}))

    def test_a_sum_within_the_tolerance_is_accepted(self) -> None:
        assert _reply_problems(COLOR, _choice_answer({"blue": 0.2, "green": 0.71, "grey": 0.1})) == ()

    @pytest.mark.parametrize("bad", ["0.7", None, True])
    def test_a_probability_that_is_not_a_number_is_a_problem(self, bad: object) -> None:
        answer = _choice_answer()
        answer["probabilities"] = {"blue": 0.2, "green": bad, "grey": 0.1}
        assert _reply_problems(COLOR, answer)

    def test_a_non_finite_probability_is_a_problem(self) -> None:
        body = RECORDED_OLLAMA_CHOICE_REPLY.replace("0.0028416608947914386", "NaN").encode()
        assert reply_problems((COLOR,), HTTP_OK, body)

    def test_a_score_needs_a_finite_number(self) -> None:
        answer = _score_answer({"0": 0.2, "1": 0.7, "2": 0.1})
        answer["score"] = "high"
        assert _reply_problems(RATING, answer)

    def test_score_probabilities_keyed_by_index_are_valid(self) -> None:
        assert _reply_problems(RATING, _score_answer({"0": 0.2, "1": 0.7, "2": 0.1})) == ()

    def test_score_probabilities_keyed_by_name_are_valid(self) -> None:
        assert _reply_problems(RATING, _score_answer({"low": 0.2, "mid": 0.7, "high": 0.1})) == ()

    def test_score_probabilities_mixing_both_keyings_are_a_problem(self) -> None:
        assert _reply_problems(RATING, _score_answer({"0": 0.2, "mid": 0.7, "high": 0.1}))

    def test_choice_probabilities_keyed_by_index_are_a_problem(self) -> None:
        assert _reply_problems(COLOR, _choice_answer({"0": 0.2, "1": 0.7, "2": 0.1}))


class TestClassification:
    def test_state_size_refusal_takes_precedence(self) -> None:
        result = classify_reply(Flavour.KEV, (COLOR,), HTTP_UNPROCESSABLE, KEV_STATE_LENGTH.encode())
        assert result is ReplyClass.STATE_TOO_LARGE

    def test_a_valid_reply(self) -> None:
        result = classify_reply(Flavour.OLLAMA, (COLOR,), HTTP_OK, RECORDED_OLLAMA_CHOICE_REPLY.encode())
        assert result is ReplyClass.VALID

    def test_an_unreadable_reply_is_invalid(self) -> None:
        assert classify_reply(Flavour.KEV, (COLOR,), HTTP_OK, b"oops") is ReplyClass.INVALID

    def test_a_422_that_is_not_a_state_length_is_invalid(self) -> None:
        assert classify_reply(Flavour.KEV, (COLOR,), HTTP_UNPROCESSABLE, b"{}") is ReplyClass.INVALID


class TestReading:
    def test_the_recorded_ollama_choice_is_read(self) -> None:
        view = read_reply((COLOR,), RECORDED_OLLAMA_CHOICE_REPLY.encode())
        answer = view.answers["color"]
        assert (view.model, answer.choice, answer.score) == ("tev1:0.8b", "green", None)
        assert answer.probabilities["green"] == pytest.approx(0.9948621078449252)
        assert answer.confidence == pytest.approx(0.9674686077174837)

    def test_a_score_view_keeps_the_score(self) -> None:
        body = _body({"rating": _score_answer({"0": 0.2, "1": 0.7, "2": 0.1})})
        view = read_reply((RATING,), body)
        assert view.answers["rating"].score == pytest.approx(1.2)
        assert view.answers["rating"].choice is None

    def test_the_cached_prompt_token_count_is_read_when_present(self) -> None:
        document = json.loads(_body({"color": _choice_answer()}))
        document["prompt_eval_cached_count"] = 7
        assert read_reply((COLOR,), json.dumps(document).encode()).cached_prompt_tokens == 7

    def test_the_cached_prompt_token_count_is_absent_otherwise(self) -> None:
        assert read_reply((COLOR,), _body({"color": _choice_answer()})).cached_prompt_tokens is None

    def test_a_reply_without_valid_probabilities_is_refused(self) -> None:
        with pytest.raises(RecordError):
            read_reply((COLOR,), _body({"color": _choice_answer({"blue": 1.0})}))


class TestLevelProbability:
    def test_the_index_key_is_used_first(self) -> None:
        assert level_probability({"1": 0.7, "mid": 0.3}, 1, "mid") == pytest.approx(0.7)

    def test_the_name_key_is_used_otherwise(self) -> None:
        assert level_probability({"low": 0.2, "mid": 0.7, "high": 0.1}, 1, "mid") == pytest.approx(0.7)


class TestQuestionSpecs:
    CLOSED: ClassVar[dict[str, object]] = {
        "questions": [
            {"id": "color", "type": CHOICE, "options": [{"name": "blue"}, {"name": "green"}]},
            {"id": "rating", "type": SCORE, "options": [{"name": "low"}, {"name": "high"}]},
        ]
    }

    def test_questions_are_read_in_order(self) -> None:
        specs = question_specs(self.CLOSED)
        assert [(s.question_id, s.question_type, s.options) for s in specs] == [
            ("color", CHOICE, ("blue", "green")),
            ("rating", SCORE, ("low", "high")),
        ]

    def test_a_choice_accepts_names_only_and_a_score_indexes_or_names(self) -> None:
        choice, score = question_specs(self.CLOSED)
        assert choice.probability_keys == (frozenset({"blue", "green"}),)
        assert score.probability_keys == (frozenset({"0", "1"}), frozenset({"low", "high"}))

    def test_a_closed_set_without_questions_is_refused(self) -> None:
        with pytest.raises(RecordError):
            question_specs({})
