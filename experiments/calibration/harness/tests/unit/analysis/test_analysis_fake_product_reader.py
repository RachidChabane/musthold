"""`FakeProductReader` reads recorded replies the way the Go builders do."""

from __future__ import annotations

import json
from typing import Final

import pytest

from calibration.adapters.go_layout import BuilderFailure
from calibration.domain.ports import DecodeOutcome, DecodeRequest, ScreenReadRequest
from support.fake_product_reader import FakeProductReader, go_float_text, go_round

CLOSED: Final = json.dumps(
    {
        "state": "a test",
        "questions": [
            {
                "id": "criterion",
                "type": "choice",
                "instructions": "Which?",
                "options": [{"name": "K-1", "description": "first"}, {"name": "NONE", "description": "none"}],
            },
            {
                "id": "coverage",
                "type": "score",
                "instructions": "How much?",
                "options": [{"name": "WEAK", "description": "weak"}, {"name": "FULL", "description": "full"}],
            },
        ],
    }
)
REPLY: Final = json.dumps(
    {
        "model": "m",
        "answers": {
            "criterion": {
                "type": "choice",
                "choice": "K-1",
                "confidence": 0.9,
                "probabilities": {"K-1": 0.9, "NONE": 0.1},
            },
            "coverage": {"type": "score", "score": 0.5, "confidence": 0.4, "probabilities": {"0": 0.5, "1": 0.5}},
        },
    }
)
SCREEN_DOCUMENT: Final = (
    '{"answers":[{"choice":"VIOLATES","id":"q1","probabilities":{"CONSISTENT":0.7,"VIOLATES":0.3}}]}'
)


def _decode(reply: str, closed: str = CLOSED) -> DecodeOutcome:
    return FakeProductReader().decode([DecodeRequest("k", closed, reply)])[0]


def test_an_accepted_reply_keeps_choices_and_maps_a_score_to_the_nearest_level() -> None:
    outcome = _decode(REPLY)
    assert outcome.accepted
    assert outcome.model == "m"
    criterion, coverage = outcome.answers
    assert (criterion.choice, criterion.confidence) == ("K-1", 0.9)
    assert coverage.choice == "FULL"
    assert dict(coverage.probabilities) == {}


def test_a_confidence_outside_the_unit_interval_is_refused_at_the_check_stage() -> None:
    outcome = _decode(REPLY.replace("0.9,", "1.5,", 1))
    assert not outcome.accepted
    assert outcome.stage == "check"


def test_a_score_outside_the_levels_is_refused_at_the_reply_stage() -> None:
    outcome = _decode(REPLY.replace('"score": 0.5', '"score": 2'))
    assert outcome.stage == "reply"


def test_an_unknown_field_of_the_closed_set_is_refused() -> None:
    assert _decode(REPLY, CLOSED.replace('"state"', '"extra": 1, "state"')).stage == "closed"


def test_the_document_is_compact_with_sorted_keys() -> None:
    document = _decode(REPLY).document
    assert document is not None
    assert document.startswith('{"answers":[{"choice":"K-1","confidence":0.9,"id":"criterion"')


def test_the_screen_reading_uses_the_stated_probability_of_violates() -> None:
    reading = FakeProductReader().read_screen([ScreenReadRequest("k", SCREEN_DOCUMENT, "q1")])[0]
    assert (reading.has_probability, reading.violation_probability) == (True, 0.3)
    missing = FakeProductReader().read_screen([ScreenReadRequest("k", SCREEN_DOCUMENT, "q2")])[0]
    assert missing.error is not None


def test_the_endpoint_digest_ignores_credentials_case_and_trailing_slash() -> None:
    reader = FakeProductReader()
    plain = reader.endpoint_digest("http://localhost:11434", "m")
    variant = reader.endpoint_digest("http://user:x@LOCALHOST:11434/v1/systemone/?k=v", "m")
    assert plain.digest == variant.digest
    assert plain.source == f"system-one:m@{plain.digest}"
    with pytest.raises(BuilderFailure):
        reader.endpoint_digest("ftp://host", "m")


def test_a_threshold_resolves_exact_source_then_kind_then_default() -> None:
    check = FakeProductReader().check_threshold(
        "{default: 0.2, host-votes: 0.7, 'host-votes(2)': 0.9}",
        0.5,
        ["host-votes(2)", "host-votes(3)", "system-one:m@d"],
    )
    assert check.resolved == {"host-votes": 0.7, "host-votes(2)": 0.9, "host-votes(3)": 0.7, "system-one:m@d": 0.2}
    with pytest.raises(BuilderFailure):
        FakeProductReader().check_threshold("1.5", 0.5, [])


@pytest.mark.parametrize(("value", "text"), [(1.0, "1"), (0.00001, "0.00001"), (1e-7, "1e-7"), (0.25, "0.25")])
def test_floats_are_written_as_the_go_encoder_writes_them(value: float, text: str) -> None:
    assert go_float_text(value) == text


def test_rounding_goes_half_away_from_zero() -> None:
    assert (go_round(2.5), go_round(0.5), go_round(-0.5)) == (3, 1, -1)
