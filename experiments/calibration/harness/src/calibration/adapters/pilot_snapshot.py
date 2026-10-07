"""The unpacked pilot snapshot, and the pilot's own normalization and mutant identity read from its code (§2.4)."""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from pathlib import Path
from typing import Final

from calibration.adapters.isolated_import import import_pilot_modules
from calibration.application.build_ports import SnapshotSource, UnitPair
from calibration.domain.constants import ENCODING, FIXTURE_PROJECT, PILOT_DIR, PILOT_SOURCE_ROOT

FIXTURE_SOURCE_DIR: Final = "src/parkfee"
PYTHON_SUFFIX: Final = ".py"
POSIX_SEPARATOR: Final = "/"
NORMALIZATION_MODULE: Final = "pilot.domain.normalization"
DEFINITIONS_MODULE: Final = "pilot.domain.definitions"
IDS_MODULE: Final = "pilot.domain.ids"
EDITS_MODULE: Final = "pilot.domain.edits"
PILOT_DOMAIN_MODULES: Final = (NORMALIZATION_MODULE, DEFINITIONS_MODULE, IDS_MODULE, EDITS_MODULE)
EDITS_KEY: Final = "edits"
FILE_KEY: Final = "file"
QUALNAME_KEY: Final = "qualname"


class UnpackedSnapshot:
    """Implements `SnapshotSource` over a directory that holds the snapshot paths at their repository locations."""

    def __init__(self, root: Path) -> None:
        self._root = root

    @property
    def project_dir(self) -> Path:
        return self._root / PILOT_DIR / FIXTURE_PROJECT

    @property
    def pilot_source_root(self) -> Path:
        return self._root / PILOT_DIR / PILOT_SOURCE_ROOT

    def fixture_sources(self) -> dict[str, str]:
        project = self.project_dir
        files = sorted((project / FIXTURE_SOURCE_DIR).rglob(f"*{PYTHON_SUFFIX}"))
        return {
            POSIX_SEPARATOR.join(path.relative_to(project).parts): path.read_text(encoding=ENCODING) for path in files
        }


class PilotSnapshotDomain:
    """Implements `PilotDomain` by calling `pilot.domain` of the snapshot, imported in isolation."""

    def __init__(self, snapshot: SnapshotSource) -> None:
        normalization, definitions, ids, edits = import_pilot_modules(snapshot.pilot_source_root, PILOT_DOMAIN_MODULES)
        self._normalize = normalization.normalize
        self._definition_source = definitions.definition_source
        self._apply_edit_set = definitions.apply_edit_set
        self._mutant_id = ids.mutant_id
        self._edit_set_of = edits.EditSet.from_mapping

    def mutant_id(self, normalized_edits: Sequence[Mapping[str, str]]) -> str:
        return str(self._mutant_id(self._edit_set_of({EDITS_KEY: [dict(edit) for edit in normalized_edits]})))

    def unit_pairs(self, sources: Mapping[str, str], normalized_edits: Sequence[Mapping[str, str]]) -> list[UnitPair]:
        ordered = sorted(normalized_edits, key=lambda edit: (edit[FILE_KEY], edit[QUALNAME_KEY]))
        mutated = self._apply_edit_set(sources, self._edit_set_of({EDITS_KEY: [dict(edit) for edit in ordered]}))
        return [
            UnitPair(
                edit[FILE_KEY],
                edit[QUALNAME_KEY],
                self._unit(sources[edit[FILE_KEY]], edit),
                self._unit(mutated[edit[FILE_KEY]], edit),
            )
            for edit in ordered
        ]

    def _unit(self, source: str, edit: Mapping[str, str]) -> str:
        return str(self._normalize(self._definition_source(source, edit[FILE_KEY], edit[QUALNAME_KEY])))
