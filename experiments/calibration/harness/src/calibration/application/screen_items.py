"""The mutant items (§3.4, §4.3): one screen request per non-stillborn, status-OK mutant of arms T and P."""

from __future__ import annotations

import json
from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from typing import Final

from calibration.application.build_ports import PilotDomain, UnitPair
from calibration.domain.canonical import JsonValue, sha256_hex
from calibration.domain.items import Item, KindFamily
from calibration.domain.jsonread import json_object, mapping, sequence, text
from calibration.domain.pilot_records import (
    DIFF_DIFFERS,
    MEASURED_ARMS,
    MutantDocument,
    ProcessingRow,
)
from calibration.domain.schedule import mutant_request_id
from calibration.domain.witness_mapping import Signatures, map_differential

WITNESS_MAPPED: Final = "mapped"
WITNESS_UNMAPPED: Final = "unmapped"
WITNESS_NONE: Final = "none"


@dataclass(frozen=True)
class ScreenCandidate:
    """A mutant of the population with its normalized units and the witness its differential maps to."""

    mutant_id: str
    arm: str
    units: tuple[UnitPair, ...]
    witness: dict[str, JsonValue] | None
    witness_status: str
    unmapped_reason: str | None

    def builder_input(self) -> dict[str, JsonValue]:
        edits: list[JsonValue] = [
            {"file": unit.file, "qualname": unit.qualname, "before": unit.before, "after": unit.after}
            for unit in self.units
        ]
        witnesses: list[JsonValue] = [self.witness] if self.witness is not None else []
        return {"mutant_id": self.mutant_id, "edits": edits, "witnesses": witnesses}


@dataclass(frozen=True)
class ScreenAssembly:
    """The screen items of a run: `blocked` when the pilot's arm records are missing and K4 cannot be measured."""

    items: tuple[Item, ...]
    candidates: tuple[ScreenCandidate, ...]
    unmapped: tuple[ScreenCandidate, ...]
    blocked: bool


BLOCKED_SCREEN: Final = ScreenAssembly((), (), (), True)


def population_ids(processing: Sequence[ProcessingRow], documents: Mapping[str, MutantDocument]) -> list[str]:
    """Non-stillborn status-OK mutants whose document names arm T or P; a mutant shared across arms is one item,
    under the arm its document records (the first writer)."""
    return sorted(
        row.mutant_id
        for row in processing
        if row.evaluated and row.mutant_id in documents and documents[row.mutant_id].arm in MEASURED_ARMS
    )


def prepare_candidate(
    document: MutantDocument,
    differential: Mapping[str, object] | None,
    sources: Mapping[str, str],
    signatures: Signatures,
    pilot_domain: PilotDomain,
) -> ScreenCandidate:
    units = tuple(pilot_domain.unit_pairs(sources, document.normalized_edits))
    witness: dict[str, JsonValue] | None = None
    status, reason = WITNESS_NONE, None
    if differential is not None and differential.get("status") == DIFF_DIFFERS:
        mapped = map_differential(differential, signatures)
        witness, reason = mapped.witness, mapped.unmapped_reason
        status = WITNESS_MAPPED if witness is not None else WITNESS_UNMAPPED
    return ScreenCandidate(document.mutant_id, document.arm, units, witness, status, reason)


def assemble_screen_items(build: Mapping[str, object], candidates: Sequence[ScreenCandidate]) -> list[Item]:
    """Turns the output of the discpower builder into items; an item that answers no candidate has no witness status
    and `screen_problems` reports it."""
    by_id = {candidate.mutant_id: candidate for candidate in candidates}
    items: list[Item] = []
    for index, raw in enumerate(sequence(build.get("items"), "build.items")):
        entry = mapping(raw, f"build.items[{index}]")
        mutant_id = text(entry.get("mutant_id"), "item.mutant_id")
        closed_text = text(entry.get("closed"), "item.closed")
        candidate = by_id.get(mutant_id)
        items.append(
            Item(
                request_id=mutant_request_id(mutant_id),
                family=KindFamily.SCREEN,
                unit=mutant_id,
                closed=json_object(json.loads(closed_text), "closed"),
                closed_digest=sha256_hex(closed_text),
                template_digest=text(entry.get("template_digest"), "item.template_digest"),
                bodies={
                    text(identifier, "body model"): text(body, "body")
                    for identifier, body in mapping(entry.get("bodies"), "item.bodies").items()
                },
                mutant_id=mutant_id,
                question_to_ref={
                    text(question, "question"): text(ref, "question ref")
                    for question, ref in mapping(entry.get("question_to_ref"), "item.question_to_ref").items()
                },
                witness_status=candidate.witness_status if candidate is not None else None,
            )
        )
    return sorted(items, key=lambda item: item.request_id)


def screen_problems(
    items: Sequence[Item], candidates: Sequence[ScreenCandidate], expected_template_digest: str
) -> list[str]:
    """PC3 (d) for the screen items, and one item per candidate."""
    problems = [
        f"{item.request_id}: template digest {item.template_digest} is not the module's"
        for item in items
        if item.template_digest != expected_template_digest
    ]
    built = {item.unit for item in items}
    wanted = {candidate.mutant_id for candidate in candidates}
    problems.extend(f"mutant {mutant_id} has no request" for mutant_id in sorted(wanted - built))
    problems.extend(f"request for the unknown mutant {mutant_id}" for mutant_id in sorted(built - wanted))
    return problems
