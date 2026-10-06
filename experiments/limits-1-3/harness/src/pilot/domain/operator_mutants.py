"""Arm O: deterministic AST operator mutants of the source, one per site and operator (PREREG 6.3)."""

from __future__ import annotations

import ast
import copy
from collections.abc import Callable, Iterator, Mapping
from dataclasses import dataclass

from pilot.domain.edits import Edit, EditSet

COMPARE_OPERATORS: tuple[type[ast.cmpop], ...] = (ast.Lt, ast.LtE, ast.Gt, ast.GtE, ast.Eq, ast.NotEq)
ARITHMETIC_OPERATORS: tuple[type[ast.operator], ...] = (ast.Add, ast.Sub, ast.Mult, ast.Div, ast.FloorDiv)
CONSTANT_SHIFTS: tuple[Callable[[int | float], int | float], ...] = (
    lambda value: value + 1,
    lambda value: value - 1,
    lambda value: value * 10,
)
Mutator = Callable[[ast.AST], None]
_FunctionDef = ast.FunctionDef | ast.AsyncFunctionDef


@dataclass(frozen=True)
class OperatorMutant:
    edit_set: EditSet


def operator_mutants(sources: Mapping[str, str]) -> list[OperatorMutant]:
    """Mutants of every function and method of every `.py` file, in file then source order; no-ops are skipped."""
    mutants = []
    for file in sorted(sources):
        for qualname, function in _functions(ast.parse(sources[file])):
            original = ast.unparse(function)
            for variant in _variants(function):
                changed = ast.unparse(variant)
                if changed != original:
                    mutants.append(OperatorMutant(EditSet((Edit(file, qualname, changed),))))
    return mutants


def _functions(tree: ast.Module) -> Iterator[tuple[str, _FunctionDef]]:
    for node in tree.body:
        if isinstance(node, _FunctionDef):
            yield node.name, node
        elif isinstance(node, ast.ClassDef):
            for member in node.body:
                if isinstance(member, _FunctionDef):
                    yield f"{node.name}.{member.name}", member


def _variants(function: _FunctionDef) -> Iterator[_FunctionDef]:
    for position, node in enumerate(ast.walk(function)):
        for mutate in _mutators(node):
            clone = copy.deepcopy(function)
            mutate(list(ast.walk(clone))[position])
            yield clone


def _mutators(node: ast.AST) -> list[Mutator]:
    mutators: list[Mutator] = []
    if isinstance(node, ast.Compare):
        mutators += _compare_mutators(node)
    if isinstance(node, ast.BinOp):
        mutators += [_replace_operator(operator, "op") for operator in _others(ARITHMETIC_OPERATORS, node.op)]
    if isinstance(node, ast.Constant) and _is_number(node.value):
        mutators += [_shift_constant(shift) for shift in CONSTANT_SHIFTS]
    if isinstance(node, ast.If | ast.IfExp | ast.While):
        mutators.append(_negate_test)
    if isinstance(node, ast.BoolOp):
        mutators += [_negate_operand(index) for index in range(len(node.values))]
    if isinstance(node, ast.If):
        mutators += _return_deletions(node)
    return mutators


def _others(operators: tuple[type, ...], current: ast.AST) -> list[type]:
    return [operator for operator in operators if not isinstance(current, operator)]


def _is_number(value: object) -> bool:
    return isinstance(value, int | float) and not isinstance(value, bool)


def _compare_mutators(node: ast.Compare) -> list[Mutator]:
    return [
        _replace_compare(index, operator)
        for index, current in enumerate(node.ops)
        for operator in _others(COMPARE_OPERATORS, current)
    ]


def _replace_compare(index: int, operator: type[ast.cmpop]) -> Mutator:
    def mutate(node: ast.AST) -> None:
        assert isinstance(node, ast.Compare)
        node.ops[index] = operator()

    return mutate


def _replace_operator(operator: type, field: str) -> Mutator:
    def mutate(node: ast.AST) -> None:
        setattr(node, field, operator())

    return mutate


def _shift_constant(shift: Callable[[int | float], int | float]) -> Mutator:
    def mutate(node: ast.AST) -> None:
        assert isinstance(node, ast.Constant) and isinstance(node.value, int | float)
        node.value = shift(node.value)

    return mutate


def _negate_test(node: ast.AST) -> None:
    assert isinstance(node, ast.If | ast.IfExp | ast.While)
    node.test = ast.UnaryOp(op=ast.Not(), operand=node.test)


def _negate_operand(index: int) -> Mutator:
    def mutate(node: ast.AST) -> None:
        assert isinstance(node, ast.BoolOp)
        node.values[index] = ast.UnaryOp(op=ast.Not(), operand=node.values[index])

    return mutate


def _return_deletions(node: ast.If) -> list[Mutator]:
    return [
        _delete_return(branch, index)
        for branch in ("body", "orelse")
        for index, statement in enumerate(getattr(node, branch))
        if isinstance(statement, ast.Return)
    ]


def _delete_return(branch: str, index: int) -> Mutator:
    def mutate(node: ast.AST) -> None:
        getattr(node, branch)[index] = ast.Pass()

    return mutate
