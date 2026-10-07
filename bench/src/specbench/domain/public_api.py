"""The public API of the touched source files at the merge commit: names, signatures and one-line summaries.

Extracted deterministically from the source text, so the spec author and the leak probe read the same facts and no
model summarises the code.
"""

from __future__ import annotations

import ast
import posixpath
import re
from collections.abc import Mapping
from typing import Final

from specbench.domain.languages import Language

GO_FUNCTION: Final = re.compile(r"^func (?:\((?P<receiver>[^)]*)\) )?(?P<name>[A-Z][A-Za-z0-9_]*)(?P<rest>\(.*)$")
GO_TYPE: Final = re.compile(r"^type (?P<name>[A-Z][A-Za-z0-9_]*)\b(?P<rest>.*)$")
GO_PACKAGE: Final = re.compile(r"^package (\w+)", re.MULTILINE)
GO_OPENING_BRACE: Final = re.compile(r"\s*\{\s*$")
SOURCE_ROOTS: Final = ("src/", "lib/")
PYTHON_INIT: Final = "__init__"


def python_module_path(path: str) -> str:
    """`src/pkg/mod.py` is `pkg.mod`; `pkg/__init__.py` is `pkg`."""
    relative = path
    for root in SOURCE_ROOTS:
        relative = relative.removeprefix(root)
    parts = relative.removesuffix(".py").split("/")
    if parts[-1] == PYTHON_INIT:
        parts = parts[:-1]
    return ".".join(parts)


def _first_line(docstring: str | None) -> str:
    return docstring.strip().splitlines()[0] if docstring and docstring.strip() else ""


def _signature(node: ast.FunctionDef | ast.AsyncFunctionDef) -> str:
    keyword = "async def" if isinstance(node, ast.AsyncFunctionDef) else "def"
    returns = f" -> {ast.unparse(node.returns)}" if node.returns is not None else ""
    return f"{keyword} {node.name}({ast.unparse(node.args)}){returns}"


def _is_public_member(name: str) -> bool:
    return not name.startswith("_") or name == "__init__"


def python_api(path: str, source: str) -> list[str]:
    """Public classes, functions and methods of a Python module, one line each."""
    try:
        module = ast.parse(source)
    except SyntaxError:
        return []
    dotted = python_module_path(path)
    lines: list[str] = []
    for node in module.body:
        if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)) and not node.name.startswith("_"):
            summary = _first_line(ast.get_docstring(node))
            lines.append(f"{dotted}: {_signature(node)}" + (f"  # {summary}" if summary else ""))
        elif isinstance(node, ast.ClassDef) and not node.name.startswith("_"):
            bases = f"({', '.join(ast.unparse(b) for b in node.bases)})" if node.bases else ""
            lines.append(f"{dotted}: class {node.name}{bases}")
            for member in node.body:
                if isinstance(member, (ast.FunctionDef, ast.AsyncFunctionDef)) and _is_public_member(member.name):
                    lines.append(f"{dotted}: {node.name}.{_signature(member)}")
    return lines


def go_api(module: str, path: str, source: str) -> list[str]:
    """Exported functions, methods and types of a Go file, one line each, under the package's import path."""
    directory = posixpath.dirname(path)
    import_path = f"{module}/{directory}" if directory else module
    lines: list[str] = []
    for raw in source.splitlines():
        function = GO_FUNCTION.match(raw)
        if function is not None:
            receiver = f"({function.group('receiver')}) " if function.group("receiver") else ""
            lines.append(
                f"{import_path}: func {receiver}{function.group('name')}{GO_OPENING_BRACE.sub('', function.group('rest'))}"
            )
            continue
        declared = GO_TYPE.match(raw)
        if declared is not None:
            lines.append(
                f"{import_path}: type {declared.group('name')}{GO_OPENING_BRACE.sub('', declared.group('rest'))}"
            )
    return lines


def public_api(language: Language, module: str, sources: Mapping[str, str]) -> str:
    """The API lines of every given non-test source file, in path order."""
    lines: list[str] = []
    for path in sorted(sources):
        lines += python_api(path, sources[path]) if language is Language.PYTHON else go_api(module, path, sources[path])
    return "\n".join(lines)
