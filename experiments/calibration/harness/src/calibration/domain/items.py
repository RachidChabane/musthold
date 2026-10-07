"""Items (§15.1): one decision request with the bodies archived for every model, before any model call."""

from __future__ import annotations

from collections.abc import Mapping
from dataclasses import dataclass, replace
from enum import Enum

from calibration.domain.canonical import JsonValue, sha256_hex
from calibration.domain.jsonread import enum_of, mapping, optional_text, text


class KindFamily(Enum):
    ALIGNMENT = "alignment"
    SCREEN = "screen"


@dataclass(frozen=True)
class Item:
    """`{request_id, kind_family, unit, closed, template_digest, bodies}` and the fields of its family.

    `bodies` maps each model identifier of §5.1 to the verbatim request body. `closed_digest` is the SHA-256 of
    the verbatim `closed` member the builder rendered; `closed` is its parsed form.
    """

    request_id: str
    family: KindFamily
    unit: str
    closed: Mapping[str, JsonValue]
    closed_digest: str
    template_digest: str
    bodies: Mapping[str, str]
    nodeid: str | None = None
    label: str | None = None
    declared: str | None = None
    token_to_ref: Mapping[str, str] | None = None
    mutant_id: str | None = None
    question_to_ref: Mapping[str, str] | None = None
    witness_status: str | None = None

    def body_digests(self) -> dict[str, str]:
        return {identifier: sha256_hex(body) for identifier, body in sorted(self.bodies.items())}

    def to_record(self) -> dict[str, JsonValue]:
        record: dict[str, JsonValue] = {
            "request_id": self.request_id,
            "kind_family": self.family.value,
            "unit": self.unit,
            "closed": self.closed,
            "closed_digest": self.closed_digest,
            "template_digest": self.template_digest,
            "bodies": dict(self.bodies),
            "body_digests": self.body_digests(),
        }
        if self.family is KindFamily.ALIGNMENT:
            record.update(
                {
                    "nodeid": self.nodeid,
                    "label": self.label,
                    "declared": self.declared,
                    "token_to_ref": dict(self.token_to_ref or {}),
                }
            )
        else:
            record.update(
                {
                    "mutant_id": self.mutant_id,
                    "question_to_ref": dict(self.question_to_ref or {}),
                    "witness_status": self.witness_status,
                }
            )
        return record

    @classmethod
    def from_record(cls, raw: object) -> Item:
        body = mapping(raw, "item")
        family = enum_of(KindFamily, body.get("kind_family"), "item.kind_family")
        bodies = {
            text(identifier, "body model"): text(value, f"body of {identifier}")
            for identifier, value in mapping(body.get("bodies"), "item.bodies").items()
        }
        item = cls(
            request_id=text(body.get("request_id"), "item.request_id"),
            family=family,
            unit=text(body.get("unit"), "item.unit"),
            closed=dict(mapping(body.get("closed"), "item.closed")),  # type: ignore[arg-type]
            closed_digest=text(body.get("closed_digest"), "item.closed_digest"),
            template_digest=text(body.get("template_digest"), "item.template_digest"),
            bodies=bodies,
        )
        return item._with_family_fields(body)

    def _with_family_fields(self, body: Mapping[str, object]) -> Item:
        if self.family is KindFamily.ALIGNMENT:
            return replace(
                self,
                nodeid=text(body.get("nodeid"), "item.nodeid"),
                label=text(body.get("label"), "item.label"),
                declared=text(body.get("declared"), "item.declared"),
                token_to_ref=_string_map(body.get("token_to_ref"), "item.token_to_ref"),
            )
        return replace(
            self,
            mutant_id=text(body.get("mutant_id"), "item.mutant_id"),
            question_to_ref=_string_map(body.get("question_to_ref"), "item.question_to_ref"),
            witness_status=optional_text(body.get("witness_status"), "item.witness_status"),
        )


def _string_map(value: object, where: str) -> dict[str, str]:
    return {text(key, where): text(item, where) for key, item in mapping(value, where).items()}
