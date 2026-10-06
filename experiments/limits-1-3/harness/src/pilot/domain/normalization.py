"""Source normalization (PREREG 6.4 F1): comments and docstrings stripped through `ast`."""

from __future__ import annotations

import ast
import difflib
import hashlib
from collections.abc import Mapping


def normalize(source: str) -> str:
    """Strip comments and docstrings through `ast`, then re-emit with `ast.unparse`."""
    tree = ast.parse(source)
    _DocstringStripper().visit(tree)
    ast.fix_missing_locations(tree)
    return ast.unparse(tree)


class _DocstringStripper(ast.NodeTransformer):
    def generic_visit(self, node: ast.AST) -> ast.AST:
        super().generic_visit(node)
        if isinstance(node, ast.Module | ast.ClassDef | ast.FunctionDef | ast.AsyncFunctionDef):
            body = list(node.body)
            while body and _is_string_statement(body[0]):
                body.pop(0)
            node.body = body or ([] if isinstance(node, ast.Module) else [ast.Pass()])
        return node


def _is_string_statement(node: ast.stmt) -> bool:
    return (
        isinstance(node, ast.Expr)
        and isinstance(node.value, ast.Constant)
        and isinstance(node.value.value, str)
    )


def changed_line_count(original: str, mutated: str) -> int:
    """Lines changed after normalization; a replaced block counts the larger side."""
    before, after = normalize(original).splitlines(), normalize(mutated).splitlines()
    matcher = difflib.SequenceMatcher(None, before, after, autojunk=False)
    return sum(
        max(i2 - i1, j2 - j1)
        for tag, i1, i2, j1, j2 in matcher.get_opcodes()
        if tag != "equal"
    )


def normalized_digest(sources: Mapping[str, str]) -> str:
    """Stable digest of normalized module texts, keyed by file."""
    digest = hashlib.sha256()
    for file in sorted(sources):
        digest.update(f"{file}\0{normalize(sources[file])}\0".encode())
    return digest.hexdigest()
