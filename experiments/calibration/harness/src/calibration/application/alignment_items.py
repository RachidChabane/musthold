"""The alignment items (§3.3, §4.2): one request per test the module's masker can show blind."""

from __future__ import annotations

import json
from collections.abc import Mapping
from dataclasses import dataclass

from calibration.domain.canonical import JsonValue, sha256_hex
from calibration.domain.items import Item, KindFamily
from calibration.domain.jsonread import json_object, mapping, sequence, text
from calibration.domain.nodeids import nodeid_of
from calibration.domain.schedule import alignment_request_id


@dataclass(frozen=True)
class MaskerRejection:
    """A test the module's masker rejects: not an item, listed with the module's reason (§3.3)."""

    request_id: str
    nodeid: str
    label: str
    reason: str
    detail: str

    def to_record(self) -> dict[str, JsonValue]:
        return {
            "request_id": self.request_id,
            "nodeid": self.nodeid,
            "label": self.label,
            "reason": self.reason,
            "detail": self.detail,
        }


@dataclass(frozen=True)
class AlignmentAssembly:
    """What the alignment builder produced: items, rejections and the declared criteria of every bound test."""

    items: tuple[Item, ...]
    rejections: tuple[MaskerRejection, ...]
    declared: Mapping[str, tuple[str, ...]]
    criteria: tuple[str, ...]
    template_digest: str


def assemble_alignment_items(build: Mapping[str, object]) -> AlignmentAssembly:
    """Turns the output of `calibration-build build` into items; the closed set and bodies stay verbatim."""
    items: list[Item] = []
    rejections: list[MaskerRejection] = []
    declared: dict[str, tuple[str, ...]] = {}
    for index, raw in enumerate(sequence(build.get("items"), "build.items")):
        entry = mapping(raw, f"build.items[{index}]")
        nodeid = nodeid_of(
            text(entry.get("file"), "item.file"),
            text(entry.get("classname"), "item.classname"),
            text(entry.get("name"), "item.name"),
        )
        refs = tuple(text(ref, "item.declared") for ref in sequence(entry.get("declared"), "item.declared"))
        declared[nodeid] = refs
        request_id = alignment_request_id(nodeid)
        label = text(entry.get("label"), "item.label")
        rejected = entry.get("rejected")
        if rejected is not None:
            reason = mapping(rejected, "item.rejected")
            rejections.append(
                MaskerRejection(
                    request_id,
                    nodeid,
                    label,
                    text(reason.get("reason"), "reason"),
                    text(reason.get("detail"), "detail"),
                )
            )
            continue
        items.append(_item(entry, request_id, nodeid, label, refs))
    criteria = tuple(
        text(mapping(raw, "build.criteria[]").get("ref"), "criterion ref")
        for raw in sequence(build.get("criteria"), "build.criteria")
    )
    return AlignmentAssembly(
        items=tuple(sorted(items, key=lambda item: item.request_id)),
        rejections=tuple(sorted(rejections, key=lambda rejection: rejection.request_id)),
        declared=declared,
        criteria=criteria,
        template_digest=text(build.get("template_digest"), "build.template_digest"),
    )


def _item(entry: Mapping[str, object], request_id: str, nodeid: str, label: str, refs: tuple[str, ...]) -> Item:
    closed_text = text(entry.get("closed"), "item.closed")
    bodies = {
        text(identifier, "body model"): text(body, f"body of {identifier}")
        for identifier, body in mapping(entry.get("bodies"), "item.bodies").items()
    }
    tokens = {
        text(token, "token"): text(ref, "token ref")
        for token, ref in mapping(entry.get("token_to_ref"), "item.token_to_ref").items()
    }
    return Item(
        request_id=request_id,
        family=KindFamily.ALIGNMENT,
        unit=nodeid,
        closed=json_object(json.loads(closed_text), "closed"),
        closed_digest=sha256_hex(closed_text),
        template_digest=text(entry.get("template_digest"), "item.template_digest"),
        bodies=bodies,
        nodeid=nodeid,
        label=label,
        declared=refs[0] if refs else None,
        token_to_ref=tokens,
    )


def alignment_problems(
    assembly: AlignmentAssembly, bindings: Mapping[str, str], expected_template_digest: str
) -> list[str]:
    """PC3 (a) and (d) for the alignment items: the module's bindings equal run-02's, and every template digest is
    the module's own."""
    problems: list[str] = []
    for nodeid in sorted(set(assembly.declared) | set(bindings)):
        refs = assembly.declared.get(nodeid)
        recorded = bindings.get(nodeid)
        if refs is None:
            problems.append(f"{nodeid} is bound in the pilot and not by the module")
        elif recorded is None:
            problems.append(f"{nodeid} is bound by the module and not in the pilot")
        elif refs != (recorded,):
            problems.append(f"{nodeid}: the module binds {list(refs)}, the pilot {recorded}")
    for item in assembly.items:
        if item.template_digest != expected_template_digest:
            problems.append(f"{item.request_id}: template digest {item.template_digest} is not the module's")
    if assembly.template_digest != expected_template_digest:
        problems.append("the builder's template digest differs from the module's decide.closed digest")
    return problems
