"""Apply mutant edit sets to a project tree using `ast` positions (PREREG 6.1, 6.4 F1).

A target is a module-level function or class, or a `Class.method`.
"""

from __future__ import annotations

import ast
import textwrap
from collections.abc import Mapping
from pathlib import Path, PurePosixPath

from pilot.domain.edits import Edit, EditError, EditFailure, EditSet
from pilot.domain.normalization import changed_line_count, normalize

__all__ = ["AstEditApplier", "changed_line_count", "definition_source", "normalize", "replace_definition"]

_DefNode = ast.FunctionDef | ast.AsyncFunctionDef | ast.ClassDef
_QUALNAME_SEPARATOR = "."
_METHOD_DEPTH = 2


def replace_definition(module_source: str, edit: Edit) -> str:
    """Return the module text with the edit's definition replaced by `new_source`."""
    replacement = _checked_replacement(edit)
    node = _locate(_parse(module_source, edit.file), edit.qualname)
    first, last = _span(node)
    lines = module_source.splitlines(keepends=True)
    block = textwrap.indent(replacement, " " * node.col_offset)
    result = "".join(lines[: first - 1]) + block + "".join(lines[last:])
    _parse(result, edit.file)
    return result


def definition_source(module_source: str, file: str, qualname: str) -> str:
    """Source of a definition, dedented, decorators included."""
    node = _locate(_parse(module_source, file), qualname)
    first, last = _span(node)
    lines = module_source.splitlines(keepends=True)[first - 1 : last]
    return textwrap.dedent("".join(lines))


def _checked_replacement(edit: Edit) -> str:
    source = textwrap.dedent(edit.new_source).strip("\n") + "\n"
    tree = _parse(source, edit.file)
    expected = edit.qualname.rsplit(_QUALNAME_SEPARATOR, 1)[-1]
    definition = tree.body[0] if len(tree.body) == 1 else None
    if not isinstance(definition, _DefNode) or definition.name != expected:
        raise EditError(
            EditFailure.NOT_A_SINGLE_DEFINITION,
            f"new_source of {edit.qualname} must be exactly one definition named {expected}",
        )
    return source


def _parse(source: str, file: str) -> ast.Module:
    try:
        return ast.parse(source)
    except SyntaxError as error:
        raise EditError(EditFailure.UNPARSEABLE, f"{file}: {error.msg} (line {error.lineno})") from error


def _locate(tree: ast.Module, qualname: str) -> _DefNode:
    parts = qualname.split(_QUALNAME_SEPARATOR)
    scope: list[ast.stmt] = tree.body
    if len(parts) == _METHOD_DEPTH:
        classes = [n for n in tree.body if isinstance(n, ast.ClassDef) and n.name == parts[0]]
        scope = classes[-1].body if classes else []
    elif len(parts) != 1:
        raise EditError(EditFailure.ABSENT_QUALNAME, f"unsupported qualname {qualname!r}")
    matches = [n for n in scope if isinstance(n, _DefNode) and n.name == parts[-1]]
    if not matches:
        raise EditError(EditFailure.ABSENT_QUALNAME, f"no definition {qualname!r}")
    return matches[-1]


def _span(node: _DefNode) -> tuple[int, int]:
    first = min([node.lineno, *(d.lineno for d in node.decorator_list)])
    return first, node.end_lineno or node.lineno


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
        mutated = dict(originals)
        for edit in edit_set.edits:
            mutated[edit.file] = replace_definition(mutated[edit.file], edit)
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
