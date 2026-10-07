from __future__ import annotations

import json

import pytest

from calibration.domain.canonical import sha256_hex
from calibration.domain.items import Item, KindFamily
from calibration.domain.jsonread import RecordError
from calibration.domain.records import RawMeta

BODIES = {"nimble:9b": '{"model":"nimble:9b"}', "kev-4b": '{"model":"kev-4b"}'}


def _alignment_item() -> Item:
    return Item(
        request_id="A|tests_x.py__t",
        family=KindFamily.ALIGNMENT,
        unit="PARK-1~1",
        closed={"questions": []},
        closed_digest="c" * 64,
        template_digest="t" * 64,
        bodies=BODIES,
        nodeid="tests/x.py::t",
        label="T1",
        declared="PARK-1~1",
        token_to_ref={"R1": "PARK-1~1"},
    )


def _screen_item(witness_status: str | None = "mapped") -> Item:
    return Item(
        request_id="M|mut001",
        family=KindFamily.SCREEN,
        unit="mut001",
        closed={"questions": [{"id": "q1"}]},
        closed_digest="c" * 64,
        template_digest="t" * 64,
        bodies=BODIES,
        mutant_id="mut001",
        question_to_ref={"q1": "PARK-1~1"},
        witness_status=witness_status,
    )


def _round_tripped(item: Item) -> Item:
    return Item.from_record(json.loads(json.dumps(item.to_record())))


class TestItemRecord:
    def test_an_alignment_item_round_trips(self) -> None:
        item = _alignment_item()
        assert _round_tripped(item) == item

    @pytest.mark.parametrize("status", ["mapped", "unmapped", None])
    def test_a_screen_item_round_trips(self, status: str | None) -> None:
        item = _screen_item(status)
        assert _round_tripped(item) == item

    def test_the_record_holds_the_digest_of_every_body(self) -> None:
        record = _alignment_item().to_record()
        assert record["body_digests"] == {identifier: sha256_hex(body) for identifier, body in BODIES.items()}

    def test_an_alignment_record_has_no_screen_fields_and_the_converse(self) -> None:
        alignment, screen = _alignment_item().to_record(), _screen_item().to_record()
        assert {"nodeid", "label", "declared", "token_to_ref"} <= set(alignment)
        assert not {"mutant_id", "question_to_ref", "witness_status"} & set(alignment)
        assert {"mutant_id", "question_to_ref", "witness_status"} <= set(screen)
        assert not {"nodeid", "label", "declared", "token_to_ref"} & set(screen)

    def test_the_family_is_written_by_value(self) -> None:
        assert _alignment_item().to_record()["kind_family"] == "alignment"
        assert _screen_item().to_record()["kind_family"] == "screen"

    def test_an_unknown_family_is_refused(self) -> None:
        record = _alignment_item().to_record()
        record["kind_family"] = "other"
        with pytest.raises(RecordError):
            Item.from_record(record)

    @pytest.mark.parametrize("field", ["request_id", "unit", "bodies", "closed", "nodeid", "token_to_ref"])
    def test_a_missing_field_is_refused(self, field: str) -> None:
        record = _alignment_item().to_record()
        del record[field]
        with pytest.raises(RecordError):
            Item.from_record(record)

    def test_a_record_that_is_not_an_object_is_refused(self) -> None:
        with pytest.raises(RecordError):
            Item.from_record([])


def _meta() -> RawMeta:
    return RawMeta(
        http_status=200,
        attempt=1,
        started="2026-10-07T21:00:00Z",
        finished="2026-10-07T21:00:01Z",
        latency_ms=1000.5,
        server_version="0.35.1",
        model_digest="sha256:" + "a" * 64,
        prefix_cache_hit=True,
        transport_error=None,
    )


class TestRawMeta:
    def test_a_meta_round_trips(self) -> None:
        meta = _meta()
        assert RawMeta.from_record(json.loads(json.dumps(meta.to_record()))) == meta

    def test_a_transport_failure_has_no_status(self) -> None:
        meta = RawMeta(None, 2, "s", "f", 0.0, "v", "d", None, "timeout")
        assert RawMeta.from_record(json.loads(json.dumps(meta.to_record()))) == meta

    def test_the_record_names_the_documented_fields(self) -> None:
        documented = {"http_status", "attempt", "started", "finished", "latency_ms", "server_version", "model_digest"}
        assert documented <= set(_meta().to_record())

    def test_an_integer_latency_is_read_as_a_float(self) -> None:
        record = _meta().to_record()
        record["latency_ms"] = 12
        assert RawMeta.from_record(record).latency_ms == 12.0

    @pytest.mark.parametrize("attempt", ["1", None, True])
    def test_an_attempt_that_is_not_an_integer_is_refused(self, attempt: object) -> None:
        record: dict[str, object] = dict(_meta().to_record())
        record["attempt"] = attempt
        with pytest.raises(ValueError, match="attempt"):
            RawMeta.from_record(record)

    @pytest.mark.parametrize("field", ["started", "finished", "server_version", "model_digest"])
    def test_a_missing_text_field_is_refused(self, field: str) -> None:
        record = _meta().to_record()
        del record[field]
        with pytest.raises(RecordError):
            RawMeta.from_record(record)
