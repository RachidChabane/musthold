"""Relabel-rater input: every test with the link it declares, taken from the traceability bindings (PREREG 3.4)."""

from __future__ import annotations

import ast
from collections.abc import Mapping, Sequence
from typing import Any

from pilot.domain.definitions import is_verifies_decorator
from pilot.domain.model_call import PayloadCheck
from pilot.domain.prompts import SOURCE_FILE_HEADER

LINK_COMMENT = "# nodeid: {nodeid}  declares: {ref}"
TEST_SEPARATOR = "::"


def render_tests(test_files: Mapping[str, str], bindings: Mapping[str, str]) -> str:
    """Each test file with its `verifies` decorators replaced by a comment naming the declared link."""
    chunks = []
    for path in sorted(test_files):
        header = SOURCE_FILE_HEADER.format(path=path)
        chunks.append(f"{header}\n{_annotated(path, test_files[path], bindings)}")
    return "\n\n".join(chunks)


def _annotated(path: str, source: str, bindings: Mapping[str, str]) -> str:
    lines = source.splitlines()
    replacements: list[tuple[int, int, str]] = []
    for node in ast.parse(source).body:
        if isinstance(node, ast.FunctionDef):
            nodeid = f"{path}{TEST_SEPARATOR}{node.name}"
            span = _verifies_span(node)
            if span is not None and nodeid in bindings:
                replacements.append((*span, LINK_COMMENT.format(nodeid=nodeid, ref=bindings[nodeid])))
    for first, last, comment in sorted(replacements, reverse=True):
        lines[first - 1 : last] = [comment]
    return "\n".join(lines)


def _verifies_span(node: ast.FunctionDef) -> tuple[int, int] | None:
    spans = [
        (d.lineno, d.end_lineno or d.lineno)
        for d in node.decorator_list
        if is_verifies_decorator(d)
    ]
    return (min(s[0] for s in spans), max(s[1] for s in spans)) if spans else None


def relabel_problems(expected_nodeids: Sequence[str]) -> PayloadCheck:
    wanted = sorted(expected_nodeids)

    def check(payload: Any) -> list[str]:
        returned = sorted(entry["nodeid"] for entry in payload["tests"])
        if returned == wanted:
            return []
        missing = sorted(set(wanted) - set(returned))
        extra = sorted(set(returned) - set(wanted))
        return [f"nodeids differ: missing {missing}, unexpected {extra}, duplicates {len(returned) - len(set(returned))}"]

    return check
