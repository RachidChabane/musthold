"""Static call graph over function names of a package (PREREG 6.4 F3, 6.5)."""

from __future__ import annotations

import ast
from collections.abc import Iterable, Mapping

DefNode = ast.FunctionDef | ast.AsyncFunctionDef
CallGraph = dict[str, set[str]]


def call_graph(sources: Iterable[str]) -> CallGraph:
    graph: CallGraph = {}
    for source in sources:
        for node in ast.walk(ast.parse(source)):
            if isinstance(node, DefNode):
                graph.setdefault(node.name, set()).update(called_names(node))
    return graph


def called_names(node: ast.AST) -> set[str]:
    names: set[str] = set()
    for child in ast.walk(node):
        if isinstance(child, ast.Call):
            target = child.func
            if isinstance(target, ast.Name):
                names.add(target.id)
            elif isinstance(target, ast.Attribute):
                names.add(target.attr)
    return names


def reachable(graph: Mapping[str, set[str]], start: str) -> set[str]:
    seen = {start}
    pending = [start]
    while pending:
        for callee in graph.get(pending.pop(), ()):
            if callee not in seen:
                seen.add(callee)
                pending.append(callee)
    return seen


def callers_of(graph: Mapping[str, set[str]], targets: set[str]) -> set[str]:
    """Names whose call chains reach any target name, targets excluded."""
    return {name for name in graph if name not in targets and targets & reachable(graph, name)}


def definition_names(sources: Mapping[str, str]) -> dict[str, list[tuple[str, str]]]:
    """Map function name to its (file, qualname) definitions: module-level functions and methods."""
    index: dict[str, list[tuple[str, str]]] = {}
    for file, source in sorted(sources.items()):
        for node in ast.parse(source).body:
            if isinstance(node, DefNode):
                index.setdefault(node.name, []).append((file, node.name))
            elif isinstance(node, ast.ClassDef):
                for member in node.body:
                    if isinstance(member, DefNode):
                        index.setdefault(member.name, []).append((file, f"{node.name}.{member.name}"))
    return index
