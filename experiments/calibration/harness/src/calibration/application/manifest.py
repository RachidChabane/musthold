"""`manifest.json` (§15.1): a document that several verbs add sections to, each section written by one verb."""

from __future__ import annotations

import json
from collections.abc import Mapping

from calibration.application.ports import RunStore
from calibration.domain.canonical import JsonValue, document_json
from calibration.domain.constants import MANIFEST_FILE
from calibration.domain.jsonread import json_object


class RunManifest:
    """Reads and updates the manifest of a run; a section is replaced as a whole."""

    def __init__(self, store: RunStore) -> None:
        self._store = store

    def read(self) -> Mapping[str, JsonValue]:
        if not self._store.exists(MANIFEST_FILE):
            return {}
        return json_object(json.loads(self._store.read_text(MANIFEST_FILE)), MANIFEST_FILE)

    def set_sections(self, sections: Mapping[str, JsonValue]) -> None:
        merged: dict[str, JsonValue] = {**self.read(), **sections}
        self._store.write_text(MANIFEST_FILE, document_json(merged))
