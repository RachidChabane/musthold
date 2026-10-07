"""The records `build-items` writes once every precondition holds: items, schedule, exclusions, inputs, manifest."""

from __future__ import annotations

import hashlib
from collections.abc import Mapping
from dataclasses import dataclass
from typing import Final

from calibration.application.alignment_items import AlignmentAssembly
from calibration.application.build_ports import BuilderBuild, PilotRunReader
from calibration.application.builder_check import BuilderCheck
from calibration.application.deviation_log import DeviationLog
from calibration.application.layout import item_path
from calibration.application.manifest import RunManifest
from calibration.application.ports import RunStore
from calibration.application.screen_items import ScreenAssembly
from calibration.domain.canonical import JsonValue, document_json, sha256_hex
from calibration.domain.constants import (
    EXCLUDED_FILE,
    PILOT_FREEZE_TAG,
    PILOT_INPUTS_FILE,
    PREREG_TAG,
    SCHEDULE_FILE,
)
from calibration.domain.deviations import DeviationKind
from calibration.domain.provenance_rules import builder_rollup
from calibration.domain.schedule import Schedule

CHECK_FILE: Final = "builder_check.json"
SNAPSHOT_PREREG_PREFIX: Final = "experiments/limits-1-3/prereg/"
PROMPT_FILES: Final = {
    "alignment": ("prompts/alignment.md", "schemas/alignment.tmpl.schema.json"),
    "validity": ("prompts/validity.md", "schemas/validity.schema.json"),
}
SOURCE2_DIGEST_SEPARATOR: Final = "\x00"


@dataclass(frozen=True)
class BuildFacts:
    """What the manifest records about where the items come from."""

    run_id: str
    prereg_commit: str
    harness_commit: str
    harness_clean: bool
    pilot_freeze_commit: str
    pilot_analysis_commit: str
    snapshot_files: Mapping[str, str]
    export_digests: Mapping[str, str]
    build: BuilderBuild
    go_procs: int
    alignment_template_digest: str
    screen_template_digest: str


def source2_template_digest(prompt_digest: str | None, schema_digest: str | None) -> str:
    """The template digest of a source-(2) key: the SHA-256 of the pilot prompt and schema the votes answered."""
    return hashlib.sha256(f"{prompt_digest}{SOURCE2_DIGEST_SEPARATOR}{schema_digest}".encode()).hexdigest()


def prompt_digests(snapshot_files: Mapping[str, str]) -> dict[str, JsonValue]:
    digests: dict[str, JsonValue] = {}
    for name, (prompt, schema) in PROMPT_FILES.items():
        prompt_digest = snapshot_files.get(SNAPSHOT_PREREG_PREFIX + prompt)
        schema_digest = snapshot_files.get(SNAPSHOT_PREREG_PREFIX + schema)
        digests[name] = {
            "prompt": prompt_digest,
            "schema": schema_digest,
            "template": source2_template_digest(prompt_digest, schema_digest),
        }
    return digests


class BuildRecordsWriter:
    def __init__(self, store: RunStore, reader: PilotRunReader) -> None:
        self._store = store
        self._reader = reader
        self._log = DeviationLog(store)
        self._manifest = RunManifest(store)

    def write(
        self, alignment: AlignmentAssembly, screen: ScreenAssembly, check: BuilderCheck, facts: BuildFacts
    ) -> None:
        items = [*alignment.items, *screen.items]
        for item in items:
            self._store.write_text(item_path(item.request_id), document_json(item.to_record()))
        schedule = Schedule.of(item.request_id for item in items)
        self._store.write_text(
            SCHEDULE_FILE, document_json({"pass1": list(schedule.pass1), "pass2": list(schedule.pass2)})
        )
        excluded: dict[str, JsonValue] = {
            "masker_rejections": [rejection.to_record() for rejection in alignment.rejections],
            "witness_unmapped": [
                {"mutant_id": candidate.mutant_id, "reason": candidate.unmapped_reason}
                for candidate in sorted(screen.unmapped, key=lambda candidate: candidate.mutant_id)
            ],
        }
        self._store.write_text(EXCLUDED_FILE, document_json(excluded))
        inputs: dict[str, JsonValue] = {
            "files": self._reader.digests(),
            "seal_mismatches": self._reader.verify_against_seal(),
        }
        self._store.write_text(PILOT_INPUTS_FILE, document_json(inputs))
        self._store.write_text(CHECK_FILE, document_json(check.to_record(facts.harness_commit)))
        if not facts.harness_clean:
            self._log.record(
                DeviationKind.PROTOCOL, ["all"], "the harness had uncommitted changes when the items were built"
            )
        self._manifest.set_sections(self._manifest_sections(alignment, screen, facts))

    @staticmethod
    def _manifest_sections(
        alignment: AlignmentAssembly, screen: ScreenAssembly, facts: BuildFacts
    ) -> dict[str, JsonValue]:
        return {
            "run_id": facts.run_id,
            "prereg_tag": PREREG_TAG,
            "prereg_commit": facts.prereg_commit,
            "harness_commit": facts.harness_commit,
            "harness_clean": facts.harness_clean,
            "builder_digest": builder_rollup(facts.build.binary_digests),
            "pilot": {
                "freeze_tag": PILOT_FREEZE_TAG,
                "freeze_commit": facts.pilot_freeze_commit,
                "analysis_commit": facts.pilot_analysis_commit,
            },
            "snapshot_digest": sha256_hex(document_json(dict(facts.snapshot_files))),
            "product_export": {
                "commit": facts.prereg_commit,
                "digest": sha256_hex(document_json(dict(facts.export_digests))),
                "files": dict(facts.export_digests),
            },
            "builders": {
                "source_digests": dict(facts.build.source_digests),
                "binary_digests": dict(facts.build.binary_digests),
                "go_procs": facts.go_procs,
            },
            "template_digests": {
                "alignment": facts.alignment_template_digest,
                "discpower": facts.screen_template_digest,
            },
            "source2_prompt_digests": prompt_digests(facts.snapshot_files),
            "counts": {
                "alignment_items": len(alignment.items),
                "masker_rejections": len(alignment.rejections),
                "screen_items": len(screen.items),
                "screen_candidates": len(screen.candidates),
                "screen_blocked": screen.blocked,
            },
        }
