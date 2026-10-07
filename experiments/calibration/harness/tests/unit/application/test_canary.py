from __future__ import annotations

import json
from collections.abc import Sequence

import pytest

from calibration.application.canary import CanaryResult, CanaryRunner
from calibration.application.exchange import Exchange
from calibration.domain.constants import CHARS_PER_TOKEN_PROBE, CONFIDENCE_TOLERANCE
from calibration.domain.models import Flavour, ModelSpec, model_by_id
from calibration.domain.ports import HttpReply, TransportFailure
from calibration.domain.server_wire import question_specs
from support.application_fakes import (
    EchoBuilders,
    FakeModelServer,
    MirroringReader,
    ScriptedPostTransport,
    queued_replies,
)
from support.fakes import FakeClock, FakeSleeper
from support.run_directory_seed import (
    CANARY_CHOICE_ID,
    CANARY_CHOICE_OPTIONS,
    CANARY_CLOSED_TEXT,
    CANARY_STATE,
    INVALID_REPLY,
    ITEM_REPLY,
    OLLAMA_PROMPT_TOO_LONG_BODY,
    canary_reply,
    too_large_reply,
)

CHECK_NAMES = (
    "server-valid",
    "relay-accepts",
    "document-keeps-probabilities",
    "confidence-formula",
    "state-size-probe",
)
CONTEXT_TOKENS = 1000
BREAKING_SHIFT = 10 * CONFIDENCE_TOLERANCE
WITHIN_TOLERANCE_SHIFT = CONFIDENCE_TOLERANCE / 2


def run_canary(
    model: ModelSpec,
    replies: Sequence[HttpReply | TransportFailure],
    *,
    reader: MirroringReader | None = None,
    context_tokens: int | None = CONTEXT_TOKENS,
) -> tuple[CanaryResult, ScriptedPostTransport]:
    clock = FakeClock()
    transport = ScriptedPostTransport(queued_replies(replies))
    server = FakeModelServer(model.model_id, model.flavour, [], loaded_context_tokens=context_tokens)
    exchange = Exchange(transport, clock, FakeSleeper(clock), model.flavour)
    runner = CanaryRunner(EchoBuilders(), reader or MirroringReader(), exchange, server, CANARY_CLOSED_TEXT)
    return runner.run(model), transport


def verdicts(result: CanaryResult) -> dict[str, bool]:
    return {check.name: check.passed for check in result.checks}


def failed(result: CanaryResult) -> list[str]:
    return [check.name for check in result.checks if not check.passed]


@pytest.mark.parametrize("model_id", ["M3", "M4"])
def test_a_conforming_server_passes_every_check(model_id: str) -> None:
    model = model_by_id(model_id)

    result, _ = run_canary(model, [canary_reply(model.flavour), too_large_reply(model.flavour)])

    assert result.passed
    assert tuple(check.name for check in result.checks) == CHECK_NAMES
    assert failed(result) == []


def test_the_record_carries_the_model_the_checks_and_the_probe() -> None:
    model = model_by_id("M3")
    reply = canary_reply(model.flavour)

    result, _ = run_canary(model, [reply, too_large_reply(model.flavour)])
    record = result.to_record()

    assert record["model"] == "M3"
    assert record["passed"] is True
    assert record["response"] == reply.body.decode()
    assert record["probe"] == {"status": 413, "response": ""}
    assert isinstance(record["checks"], list)
    assert len(record["checks"]) == len(CHECK_NAMES)


def test_the_canary_request_is_the_rendered_closed_set_sent_to_the_servers_endpoint() -> None:
    model = model_by_id("M3")

    result, transport = run_canary(model, [canary_reply(model.flavour), too_large_reply(model.flavour)])

    assert transport.posts[0].decode() == CANARY_CLOSED_TEXT
    assert result.request == CANARY_CLOSED_TEXT
    assert transport.urls[0] == f"http://server.invalid/{model.model_id}/v1/systemone"


def test_the_reply_is_read_by_the_relay_with_the_closed_set() -> None:
    model = model_by_id("M3")
    reader = MirroringReader()
    reply = canary_reply(model.flavour)

    run_canary(model, [reply, too_large_reply(model.flavour)], reader=reader)

    assert [(r.closed, r.reply) for r in reader.decoded] == [(CANARY_CLOSED_TEXT, reply.body.decode())]


def test_an_invalid_reply_fails_only_server_validity_and_skips_the_reply_checks() -> None:
    model = model_by_id("M3")
    replies = [INVALID_REPLY, INVALID_REPLY, too_large_reply(model.flavour)]

    result, _ = run_canary(model, replies)

    assert verdicts(result) == {"server-valid": False, "state-size-probe": True}
    assert not result.passed


def test_a_reply_that_is_valid_only_at_the_retry_passes_server_validity() -> None:
    model = model_by_id("M3")

    result, _ = run_canary(model, [INVALID_REPLY, canary_reply(model.flavour), too_large_reply(model.flavour)])

    assert result.passed


def test_a_reply_without_the_canary_questions_fails_server_validity() -> None:
    model = model_by_id("M3")

    result, _ = run_canary(model, [ITEM_REPLY, ITEM_REPLY, too_large_reply(model.flavour)])

    assert failed(result) == ["server-valid"]


def test_a_transport_failure_twice_fails_server_validity() -> None:
    model = model_by_id("M3")
    replies = [TransportFailure("refused"), TransportFailure("refused"), too_large_reply(model.flavour)]

    result, _ = run_canary(model, replies)

    assert failed(result) == ["server-valid"]
    assert result.response == ""


def test_a_reply_the_relay_refuses_fails_only_the_relay_check() -> None:
    model = model_by_id("M3")

    result, _ = run_canary(
        model, [canary_reply(model.flavour), too_large_reply(model.flavour)], reader=MirroringReader(accepted=False)
    )

    assert failed(result) == ["relay-accepts"]
    relay = next(check for check in result.checks if check.name == "relay-accepts")
    assert "relay" in relay.detail


def test_a_document_that_alters_the_probabilities_fails_only_that_check() -> None:
    model = model_by_id("M3")
    altered: dict[str, float] = dict.fromkeys(CANARY_CHOICE_OPTIONS, 1 / len(CANARY_CHOICE_OPTIONS))

    result, _ = run_canary(
        model,
        [canary_reply(model.flavour), too_large_reply(model.flavour)],
        reader=MirroringReader(probabilities=altered),
    )

    assert failed(result) == ["document-keeps-probabilities"]


def test_a_document_that_alters_the_confidence_fails_only_that_check() -> None:
    model = model_by_id("M3")

    result, _ = run_canary(
        model,
        [canary_reply(model.flavour), too_large_reply(model.flavour)],
        reader=MirroringReader(confidence=0.123),
    )

    assert failed(result) == ["document-keeps-probabilities"]
    detail = next(check.detail for check in result.checks if check.name == "document-keeps-probabilities")
    assert CANARY_CHOICE_ID in detail


def test_a_document_without_the_choice_answer_fails_that_check() -> None:
    model = model_by_id("M3")

    result, _ = run_canary(
        model,
        [canary_reply(model.flavour), too_large_reply(model.flavour)],
        reader=MirroringReader(keeps_answers=False),
    )

    assert failed(result) == ["document-keeps-probabilities"]


@pytest.mark.parametrize("model_id", ["M3", "M4"])
def test_a_choice_confidence_off_the_formula_fails_only_the_formula_check(model_id: str) -> None:
    model = model_by_id(model_id)
    reply = canary_reply(model.flavour, choice_shift=-BREAKING_SHIFT)

    result, _ = run_canary(model, [reply, too_large_reply(model.flavour)])

    assert failed(result) == ["confidence-formula"]
    detail = next(check.detail for check in result.checks if check.name == "confidence-formula")
    assert CANARY_CHOICE_ID in detail


@pytest.mark.parametrize("model_id", ["M3", "M4"])
def test_a_score_confidence_off_the_formula_fails_only_the_formula_check(model_id: str) -> None:
    model = model_by_id(model_id)
    reply = canary_reply(model.flavour, score_shift=BREAKING_SHIFT)

    result, _ = run_canary(model, [reply, too_large_reply(model.flavour)])

    assert failed(result) == ["confidence-formula"]


def test_a_confidence_within_the_tolerance_passes_the_formula_check() -> None:
    model = model_by_id("M3")
    reply = canary_reply(model.flavour, choice_shift=WITHIN_TOLERANCE_SHIFT, score_shift=-WITHIN_TOLERANCE_SHIFT)

    result, _ = run_canary(model, [reply, too_large_reply(model.flavour)])

    assert result.passed


def test_the_kev_formula_is_not_accepted_from_an_ollama_server() -> None:
    kev = model_by_id("M4")
    reply_of_the_wrong_flavour = canary_reply(Flavour.OLLAMA)

    result, _ = run_canary(kev, [reply_of_the_wrong_flavour, too_large_reply(kev.flavour)])

    assert failed(result) == ["confidence-formula"]


def test_a_reply_without_confidence_fails_the_formula_check() -> None:
    model = model_by_id("M3")
    document = json.loads(canary_reply(model.flavour).body)
    del document["answers"][CANARY_CHOICE_ID]["confidence"]
    reply = HttpReply(200, json.dumps(document).encode())

    result, _ = run_canary(model, [reply, too_large_reply(model.flavour)], reader=MirroringReader())

    assert "confidence-formula" in failed(result)


def probe_state(transport: ScriptedPostTransport) -> str:
    return json.loads(transport.posts[1])["state"]


def test_the_probe_state_holds_more_than_four_characters_per_token_of_the_loaded_context() -> None:
    model = model_by_id("M3")

    _, transport = run_canary(model, [canary_reply(model.flavour), too_large_reply(model.flavour)])

    state = probe_state(transport)
    assert len(state) > CHARS_PER_TOKEN_PROBE * CONTEXT_TOKENS
    assert len(state) - len(CANARY_STATE) < CHARS_PER_TOKEN_PROBE * CONTEXT_TOKENS


def test_the_probe_state_is_the_canary_state_repeated() -> None:
    model = model_by_id("M3")

    _, transport = run_canary(model, [canary_reply(model.flavour), too_large_reply(model.flavour)])

    assert set(probe_state(transport).split(" ")) == set(CANARY_STATE.split(" "))
    assert probe_state(transport).startswith(CANARY_STATE)


def test_the_probe_keeps_the_canary_questions() -> None:
    model = model_by_id("M3")

    _, transport = run_canary(model, [canary_reply(model.flavour), too_large_reply(model.flavour)])

    assert json.loads(transport.posts[1])["questions"] == json.loads(CANARY_CLOSED_TEXT)["questions"]


@pytest.mark.parametrize(
    ("model_id", "refusal"),
    [
        ("M3", too_large_reply(Flavour.OLLAMA)),
        ("M3", HttpReply(400, OLLAMA_PROMPT_TOO_LONG_BODY)),
        ("M4", too_large_reply(Flavour.KEV)),
    ],
)
def test_every_state_too_large_form_passes_the_probe(model_id: str, refusal: HttpReply) -> None:
    model = model_by_id(model_id)

    result, _ = run_canary(model, [canary_reply(model.flavour), refusal])

    assert result.passed
    assert result.probe_status == refusal.status
    assert result.probe_response == refusal.body.decode()


def test_an_http_200_answer_to_the_oversized_state_fails_the_probe() -> None:
    model = model_by_id("M3")

    result, _ = run_canary(model, [canary_reply(model.flavour), canary_reply(model.flavour)])

    assert failed(result) == ["state-size-probe"]
    assert result.probe_status == 200


def test_another_error_answer_to_the_oversized_state_fails_the_probe() -> None:
    model = model_by_id("M3")

    result, transport = run_canary(model, [canary_reply(model.flavour), INVALID_REPLY, INVALID_REPLY])

    assert failed(result) == ["state-size-probe"]
    assert len(transport.posts) == 3


def test_the_probe_is_never_retried_after_a_state_too_large_answer() -> None:
    model = model_by_id("M3")

    _, transport = run_canary(model, [canary_reply(model.flavour), too_large_reply(model.flavour)])

    assert len(transport.posts) == 2


def test_an_unknown_loaded_context_fails_the_probe_without_sending_it() -> None:
    model = model_by_id("M3")

    result, transport = run_canary(model, [canary_reply(model.flavour)], context_tokens=None)

    assert failed(result) == ["state-size-probe"]
    assert result.probe_status is None
    assert len(transport.posts) == 1


def test_the_probe_runs_even_when_the_reply_checks_are_skipped() -> None:
    model = model_by_id("M3")

    _, transport = run_canary(model, [INVALID_REPLY, INVALID_REPLY, too_large_reply(model.flavour)])

    assert len(transport.posts) == 3


def test_the_questions_of_the_canary_are_one_choice_and_one_score() -> None:
    specs = question_specs(json.loads(CANARY_CLOSED_TEXT))

    assert [spec.question_type for spec in specs] == ["choice", "score"]
