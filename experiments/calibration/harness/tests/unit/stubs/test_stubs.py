"""The stub servers answer what the real ones answer: replies are server-valid and carry the formulas of §5.4."""

from __future__ import annotations

import json

import pytest
from hypothesis import HealthCheck, given, settings
from hypothesis import strategies as st

from calibration.adapters.http_transport import UrllibTransport
from calibration.domain.confidence_formulas import confidence_matches, expected_confidence
from calibration.domain.models import Flavour
from calibration.domain.ports import TransportFailure
from calibration.domain.server_wire import ReplyClass, classify_reply, question_specs, read_reply
from fixtures.wire_data import (
    RECORDED_KEV_CHOICE_REPLY,
    RECORDED_KEV_MODELS_CARD,
    RECORDED_OLLAMA_CHOICE_REPLY,
    RECORDED_OLLAMA_CHOICE_REQUEST,
)
from support.stub_kev import KevBehaviour, StubKevServer
from support.stub_ollama import OllamaBehaviour, Oversize, StubOllamaServer

TIMEOUT = 10.0
CLOSED = {
    "questions": [
        {"id": "color", "type": "choice", "options": [{"name": "blue"}, {"name": "green"}, {"name": "grey"}]},
        {
            "id": "grade",
            "type": "score",
            "options": [{"name": "low"}, {"name": "mid"}, {"name": "high"}, {"name": "top"}],
        },
    ]
}
SPECS = question_specs(CLOSED)


def request_body(state: str, model: str = "tev1:0.8b") -> bytes:
    return json.dumps(
        {
            "model": model,
            "state": state,
            "questions": {
                "color": {"type": "choice", "instructions": "?", "criteria": {"blue": "b", "green": "g", "grey": "y"}},
                "grade": {"type": "score", "instructions": "?", "criteria": ["low", "mid", "high", "top"]},
            },
        }
    ).encode()


def post(url: str, body: bytes, timeout: float = TIMEOUT):  # type: ignore[no-untyped-def]
    return UrllibTransport().post(url + "/v1/systemone", body, timeout)


@pytest.mark.parametrize("flavour", [Flavour.OLLAMA, Flavour.KEV])
@given(state=st.text(max_size=200))
@settings(max_examples=20, deadline=None, suppress_health_check=[HealthCheck.function_scoped_fixture])
def test_replies_are_server_valid_with_the_confidence_of_their_flavour(flavour: Flavour, state: str) -> None:
    stub = StubOllamaServer() if flavour is Flavour.OLLAMA else StubKevServer()
    with stub:
        reply = post(stub.base_url, request_body(state))
    assert classify_reply(flavour, SPECS, reply.status, reply.body) is ReplyClass.VALID
    view = read_reply(SPECS, reply.body)
    for spec in SPECS:
        answer = view.answers[spec.question_id]
        recomputed = expected_confidence(flavour, spec.question_type, list(answer.probabilities.values()))
        assert answer.confidence is not None
        assert confidence_matches(answer.confidence, recomputed)


def test_a_score_answer_keys_probabilities_by_level_index_and_carries_the_legend() -> None:
    with StubKevServer() as stub:
        body = json.loads(post(stub.base_url, request_body("s")).body)
    score = body["answers"]["grade"]
    assert list(score["probabilities"]) == ["0", "1", "2", "3"]
    assert score["legend"] == {"0": "low", "1": "mid", "2": "high", "3": "top"}


def test_probabilities_are_a_function_of_state_and_question() -> None:
    with StubOllamaServer() as stub:
        first = post(stub.base_url, request_body("same")).body
        second = post(stub.base_url, request_body("same")).body
        other = post(stub.base_url, request_body("other")).body
    assert first == second
    assert first != other


@pytest.mark.parametrize("variant", [Oversize.TOKENS, Oversize.CONTEXT, Oversize.PAYLOAD])
def test_an_oversized_ollama_state_is_refused_in_each_recognised_way(variant: Oversize) -> None:
    behaviour = OllamaBehaviour(max_state_chars=10, oversize=variant)
    with StubOllamaServer(behaviour=behaviour) as stub:
        reply = post(stub.base_url, request_body("x" * 11))
    assert classify_reply(Flavour.OLLAMA, SPECS, reply.status, reply.body) is ReplyClass.STATE_TOO_LARGE


def test_an_oversized_kev_state_is_refused_with_422() -> None:
    with StubKevServer(behaviour=KevBehaviour(max_state_chars=10)) as stub:
        reply = post(stub.base_url, request_body("x" * 11))
    assert reply.status == 422
    assert classify_reply(Flavour.KEV, SPECS, reply.status, reply.body) is ReplyClass.STATE_TOO_LARGE


def test_an_invalid_reply_is_invalid() -> None:
    with StubOllamaServer(behaviour=OllamaBehaviour(invalid_reply=True)) as stub:
        reply = post(stub.base_url, request_body("s"))
    assert classify_reply(Flavour.OLLAMA, SPECS, reply.status, reply.body) is ReplyClass.INVALID


def test_the_model_name_can_be_echoed_differently() -> None:
    with StubOllamaServer(behaviour=OllamaBehaviour(echo_model="other:1b")) as stub:
        assert json.loads(post(stub.base_url, request_body("s")).body)["model"] == "other:1b"


def test_a_hang_beyond_the_client_timeout_is_a_timeout() -> None:
    with (
        StubOllamaServer(behaviour=OllamaBehaviour(hang_seconds=3.0)) as stub,
        pytest.raises(TransportFailure) as raised,
    ):
        post(stub.base_url, request_body("s"), timeout=0.3)
    assert raised.value.timed_out


def test_a_model_loads_on_its_first_request_and_unloads_on_keep_alive_zero() -> None:
    with StubOllamaServer() as stub:
        transport = UrllibTransport()
        assert json.loads(transport.get(stub.base_url + "/api/ps", TIMEOUT).body) == {"models": []}
        post(stub.base_url, request_body("s"))
        loaded = json.loads(transport.get(stub.base_url + "/api/ps", TIMEOUT).body)
        assert [model["name"] for model in loaded["models"]] == ["tev1:0.8b"]
        assert loaded["models"][0]["context_length"] == 2048
        transport.post(stub.base_url + "/api/generate", b'{"model":"tev1:0.8b","keep_alive":0}', TIMEOUT)
        assert json.loads(transport.get(stub.base_url + "/api/ps", TIMEOUT).body) == {"models": []}


def test_every_request_is_logged() -> None:
    with StubOllamaServer() as stub:
        body = request_body("s")
        post(stub.base_url, body)
        UrllibTransport().get(stub.base_url + "/api/version", TIMEOUT)
        assert [(request.method, request.path) for request in stub.requests] == [
            ("POST", "/v1/systemone"),
            ("GET", "/api/version"),
        ]
        assert stub.requests[0].body == body


def test_the_kev_card_counts_prefix_cache_hits_and_misses() -> None:
    with StubKevServer() as stub:
        post(stub.base_url, request_body("a"))
        post(stub.base_url, request_body("a"))
        post(stub.base_url, request_body("b"))
        cache = json.loads(UrllibTransport().get(stub.base_url + "/v1/models", TIMEOUT).body)["models"][0][
            "prefix_cache"
        ]
    assert (cache["hits"], cache["misses"]) == (1, 2)


def test_the_recorded_wire_data_is_what_the_harness_accepts() -> None:
    closed = {
        "questions": [{"id": "color", "type": "choice", "options": [{"name": n} for n in ("blue", "green", "grey")]}]
    }
    specs = question_specs(closed)
    assert json.loads(RECORDED_OLLAMA_CHOICE_REQUEST)["state"] == "the sky is green"
    for flavour, recorded in ((Flavour.OLLAMA, RECORDED_OLLAMA_CHOICE_REPLY), (Flavour.KEV, RECORDED_KEV_CHOICE_REPLY)):
        assert classify_reply(flavour, specs, 200, recorded.encode()) is ReplyClass.VALID
        answer = read_reply(specs, recorded.encode()).answers["color"]
        assert answer.confidence is not None
        assert confidence_matches(
            answer.confidence, expected_confidence(flavour, "choice", list(answer.probabilities.values()))
        )
    assert json.loads(RECORDED_KEV_MODELS_CARD)["models"][0]["prefix_cache"]["hits"] == 0
