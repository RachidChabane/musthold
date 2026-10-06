"""Definitions inside Python sources: locate, extract and replace by qualname (PREREG 6.1, 6.4 F1).

A target is a module-level function or class, or a `Class.method`.
"""

from __future__ import annotations

import ast
import textwrap
from collections.abc import Mapping

from pilot.domain.constants import VERIFIES_MARKER_NAME
from pilot.domain.edits import Edit, EditError, EditFailure, EditSet

DefNode = ast.FunctionDef | ast.AsyncFunctionDef | ast.ClassDef
QUALNAME_SEPARATOR = "."
METHOD_DEPTH = 2


def parse_module(source: str, file: str) -> ast.Module:
    try:
        return ast.parse(source)
    except SyntaxError as error:
        raise EditError(EditFailure.UNPARSEABLE, f"{file}: {error.msg} (line {error.lineno})") from error


def locate(tree: ast.Module, qualname: str) -> DefNode:
    parts = qualname.split(QUALNAME_SEPARATOR)
    scope: list[ast.stmt] = tree.body
    if len(parts) == METHOD_DEPTH:
        classes = [n for n in tree.body if isinstance(n, ast.ClassDef) and n.name == parts[0]]
        scope = classes[-1].body if classes else []
    elif len(parts) != 1:
        raise EditError(EditFailure.ABSENT_QUALNAME, f"unsupported qualname {qualname!r}")
    matches = [n for n in scope if isinstance(n, DefNode) and n.name == parts[-1]]
    if not matches:
        raise EditError(EditFailure.ABSENT_QUALNAME, f"no definition {qualname!r}")
    return matches[-1]


def span(node: DefNode) -> tuple[int, int]:
    first = min([node.lineno, *(d.lineno for d in node.decorator_list)])
    return first, node.end_lineno or node.lineno


def definition_source(module_source: str, file: str, qualname: str) -> str:
    """Source of a definition, dedented, decorators included."""
    node = locate(parse_module(module_source, file), qualname)
    first, last = span(node)
    lines = module_source.splitlines(keepends=True)[first - 1 : last]
    return textwrap.dedent("".join(lines))


def replace_definition(module_source: str, edit: Edit) -> str:
    """Return the module text with the edit's definition replaced by `new_source`."""
    replacement = _checked_replacement(edit)
    node = locate(parse_module(module_source, edit.file), edit.qualname)
    first, last = span(node)
    lines = module_source.splitlines(keepends=True)
    block = textwrap.indent(replacement, " " * node.col_offset)
    result = "".join(lines[: first - 1]) + block + "".join(lines[last:])
    parse_module(result, edit.file)
    return result


def apply_edit_set(sources: Mapping[str, str], edit_set: EditSet) -> dict[str, str]:
    """Edited texts of the files the edit set touches; every edit must resolve."""
    mutated: dict[str, str] = {}
    for edit in edit_set.edits:
        if edit.file not in sources:
            raise EditError(EditFailure.MISSING_FILE, edit.file)
        mutated[edit.file] = replace_definition(mutated.get(edit.file, sources[edit.file]), edit)
    return mutated


def _checked_replacement(edit: Edit) -> str:
    source = textwrap.dedent(edit.new_source).strip("\n") + "\n"
    tree = parse_module(source, edit.file)
    expected = edit.qualname.rsplit(QUALNAME_SEPARATOR, 1)[-1]
    definition = tree.body[0] if len(tree.body) == 1 else None
    if not isinstance(definition, DefNode) or definition.name != expected:
        raise EditError(
            EditFailure.NOT_A_SINGLE_DEFINITION,
            f"new_source of {edit.qualname} must be exactly one definition named {expected}",
        )
    return source


def is_verifies_decorator(decorator: ast.expr) -> bool:
    """Whether a decorator is the `verifies` marker, written `pytest.mark.verifies(...)` or `verifies(...)`."""
    call = decorator.func if isinstance(decorator, ast.Call) else decorator
    name = call.attr if isinstance(call, ast.Attribute) else getattr(call, "id", None)
    return name == VERIFIES_MARKER_NAME
