"""Validity-judge input of one mutant (PREREG 4.5, 6.5): functions and diff, never tests or kills."""

from __future__ import annotations

import difflib
from collections.abc import Mapping
from dataclasses import dataclass

from pilot.domain.call_graph import call_graph, callers_of, definition_names
from pilot.domain.definitions import apply_edit_set, definition_source
from pilot.domain.edits import EditSet
from pilot.domain.normalization import normalize

DEFINITION_HEADER = "# {file} :: {qualname}"
DEFINITION_SEPARATOR = "\n\n"
QUALNAME_SEPARATOR = "."


@dataclass(frozen=True)
class ValidityInput:
    original: str
    mutant: str
    diff: str


def build_validity_input(sources: Mapping[str, str], edit_set: EditSet) -> ValidityInput:
    mutated = apply_edit_set(sources, edit_set)
    return ValidityInput(
        original=_original_functions(sources, edit_set),
        mutant=_mutant_functions(mutated, edit_set),
        diff=_unified_diff(sources, mutated),
    )


def _original_functions(sources: Mapping[str, str], edit_set: EditSet) -> str:
    edited = [(edit.file, edit.qualname) for edit in edit_set.edits]
    context = _reaching_definitions(sources, edit_set, set(edited))
    return DEFINITION_SEPARATOR.join(
        _render(file, qualname, definition_source(sources[file], file, qualname))
        for file, qualname in [*edited, *context]
    )


def _reaching_definitions(
    sources: Mapping[str, str], edit_set: EditSet, edited: set[tuple[str, str]]
) -> list[tuple[str, str]]:
    graph = call_graph(sources.values())
    edited_names = {edit.qualname.rsplit(QUALNAME_SEPARATOR, 1)[-1] for edit in edit_set.edits}
    index = definition_names(sources)
    found = {
        location
        for name in callers_of(graph, edited_names)
        for location in index.get(name, [])
        if location not in edited
    }
    return sorted(found)


def _mutant_functions(mutated: Mapping[str, str], edit_set: EditSet) -> str:
    return DEFINITION_SEPARATOR.join(
        _render(edit.file, edit.qualname, definition_source(mutated[edit.file], edit.file, edit.qualname))
        for edit in edit_set.edits
    )


def _render(file: str, qualname: str, source: str) -> str:
    header = DEFINITION_HEADER.format(file=file, qualname=qualname)
    return f"{header}\n{normalize(source)}"


def _unified_diff(sources: Mapping[str, str], mutated: Mapping[str, str]) -> str:
    chunks = []
    for file in sorted(mutated):
        chunks.extend(
            difflib.unified_diff(
                normalize(sources[file]).splitlines(),
                normalize(mutated[file]).splitlines(),
                fromfile=f"a/{file}",
                tofile=f"b/{file}",
                lineterm="",
            )
        )
    return "\n".join(chunks)
