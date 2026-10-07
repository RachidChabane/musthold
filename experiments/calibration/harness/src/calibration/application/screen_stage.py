"""The screen half of `build-items`: the pilot's arm records, the identity check PC3 (b) and the screen requests."""

from __future__ import annotations

from collections.abc import Callable, Mapping
from pathlib import Path

from calibration.application.build_ports import Builders, PilotDomain, PilotRunReader, SnapshotSource
from calibration.application.deviation_log import DeviationLog
from calibration.application.screen_items import (
    BLOCKED_SCREEN,
    ScreenAssembly,
    assemble_screen_items,
    population_ids,
    prepare_candidate,
    screen_problems,
)
from calibration.domain.constants import (
    PILOT_DIFFERENTIAL_FILE,
    PILOT_MUTANTS_DIR,
    PILOT_PROCESSING_FILE,
)
from calibration.domain.deviations import DeviationKind
from calibration.domain.models import identifiers
from calibration.domain.pilot_records import (
    MEASURED_ARMS,
    MutantDocument,
    parse_differential,
    parse_mutant_document,
    parse_processing,
)
from calibration.domain.witness_mapping import function_signatures

K4_SCOPE = ["K4"]


class ScreenStage:
    """Builds the K4 items; when an arm record is missing the kind is BLOCKED (§14.1 PC2) and nothing is built."""

    def __init__(
        self,
        reader: PilotRunReader,
        log: DeviationLog,
        pilot_domain_of: Callable[[SnapshotSource], PilotDomain],
    ) -> None:
        self._reader = reader
        self._log = log
        self._pilot_domain_of = pilot_domain_of

    def build(
        self,
        source: SnapshotSource,
        builders: Builders,
        project: Path,
        shallnot: Path,
        expected_template_digest: str,
        problems: list[str],
    ) -> ScreenAssembly:
        if not (self._reader.exists(PILOT_PROCESSING_FILE) and self._reader.exists(PILOT_DIFFERENTIAL_FILE)):
            self._log.record(DeviationKind.BLOCKED, K4_SCOPE, "the pilot run lacks its arm records")
            return BLOCKED_SCREEN
        processing = parse_processing(self._reader.read_text(PILOT_PROCESSING_FILE))
        differential = parse_differential(self._reader.read_text(PILOT_DIFFERENTIAL_FILE))
        documents = self._documents([row.mutant_id for row in processing])
        if documents is None:
            self._log.record(DeviationKind.BLOCKED, K4_SCOPE, "the pilot run lacks a mutant record")
            return BLOCKED_SCREEN
        pilot_domain = self._pilot_domain_of(source)
        problems.extend(identity_problems(documents, pilot_domain))
        sources = source.fixture_sources()
        signatures = function_signatures(sources)
        candidates = tuple(
            prepare_candidate(documents[mutant_id], differential.get(mutant_id), sources, signatures, pilot_domain)
            for mutant_id in population_ids(processing, documents)
        )
        build = builders.screen_build(project, shallnot, identifiers(), [c.builder_input() for c in candidates])
        items = tuple(assemble_screen_items(build, candidates))
        problems.extend(screen_problems(items, candidates, expected_template_digest))
        unmapped = tuple(candidate for candidate in candidates if candidate.unmapped_reason is not None)
        return ScreenAssembly(items, candidates, unmapped, False)

    def _documents(self, mutant_ids: list[str]) -> dict[str, MutantDocument] | None:
        documents: dict[str, MutantDocument] = {}
        for mutant_id in mutant_ids:
            relative = f"{PILOT_MUTANTS_DIR}/{mutant_id}.json"
            if not self._reader.exists(relative):
                return None
            documents[mutant_id] = parse_mutant_document(self._reader.read_text(relative))
        return documents


def identity_problems(documents: Mapping[str, MutantDocument], pilot_domain: PilotDomain) -> list[str]:
    """PC3 (b): every arm-T and arm-P mutant id is reproduced from its normalized edits."""
    return [
        f"mutant {mutant_id} is not reproduced by the pilot's id from its normalized edits"
        for mutant_id, document in sorted(documents.items())
        if document.arm in MEASURED_ARMS and pilot_domain.mutant_id(document.normalized_edits) != mutant_id
    ]
