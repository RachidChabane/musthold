"""Apply mutant edit sets to a project tree using `ast` positions (PREREG 6.1, 6.4 F1).

A target is a module-level function or class, or a `Class.method`.
"""

from __future__ import annotations

from collections.abc import Mapping
from pathlib import Path, PurePosixPath

from pilot.domain.definitions import apply_edit_set, definition_source, replace_definition
from pilot.domain.edits import Edit, EditError, EditFailure, EditSet
from pilot.domain.normalization import changed_line_count, normalize

__all__ = ["AstEditApplier", "changed_line_count", "definition_source", "normalize", "replace_definition"]


def _resolve(root: Path, file: str) -> Path:
    posix = PurePosixPath(file)
    if posix.is_absolute() or ".." in posix.parts:
        raise EditError(EditFailure.UNSAFE_PATH, file)
    path = root.joinpath(*posix.parts)
    if not path.is_file():
        raise EditError(EditFailure.MISSING_FILE, file)
    return path


class AstEditApplier:
    """Implements the `EditApplier` port with in-place, all-or-nothing rewriting."""

    def preview(self, project_root: Path, edit_set: EditSet) -> Mapping[str, tuple[str, str]]:
        """Map each edited file to (original text, mutated text) without writing."""
        originals = {
            file: _resolve(project_root, file).read_text(encoding="utf-8")
            for file in edit_set.files
        }
        mutated = apply_edit_set(originals, edit_set)
        return {file: (originals[file], mutated[file]) for file in originals}

    def apply(self, project_root: Path, edit_set: EditSet) -> None:
        for file, (_, mutated) in self.preview(project_root, edit_set).items():
            _resolve(project_root, file).write_text(mutated, encoding="utf-8")

    def inverse(self, project_root: Path, edit_set: EditSet) -> EditSet:
        """The edit set that restores every edited definition of `project_root`."""
        texts = {
            file: _resolve(project_root, file).read_text(encoding="utf-8")
            for file in edit_set.files
        }
        return EditSet(
            tuple(
                Edit(e.file, e.qualname, definition_source(texts[e.file], e.file, e.qualname))
                for e in edit_set.edits
            )
        )
