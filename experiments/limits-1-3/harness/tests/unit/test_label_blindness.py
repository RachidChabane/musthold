"""No module reachable from the pipeline verbs may name the ground truth (PREREG 1)."""

from __future__ import annotations

import ast
import importlib.util
import re
from pathlib import Path

import pytest

ROOT_MODULES = ("pilot.cli.pipeline_commands",)
GROUND_TRUTH_MODULES = (
    "pilot.analysis",
    "pilot.cli.layout",
    "pilot.cli.commands",
    "pilot.domain.ground_truth_paths",
    "pilot.adapters.prereg_files",
    "pilot.adapters.plain_pytest",
    "pilot.application.verify_labels",
    "pilot.application.runner_crosscheck",
    "pilot.application.assemble_arm_p",
    "pilot.domain.labels",
    "pilot.domain.label_rules",
    "pilot.domain.label_matrix",
    "pilot.domain.variants",
    "pilot.domain.isolation_table",
    "pilot.application.isolation_table_stage",
    "pilot.domain.outcome_matrix",
)
FORBIDDEN_STRINGS = ("labels.yaml", "oracle", "violations", "refactor", "planted", "planted_map", "arm_p/labels")
FORBIDDEN_NAMES = ("oracle", "refactor", "arm_p_labels")
_WORD = "(?<![A-Za-z_]){}(?![A-Za-z_])"


def find_module(name: str):
    try:
        return importlib.util.find_spec(name)
    except (ModuleNotFoundError, ValueError):
        return None


def module_source(name: str) -> tuple[Path, str] | None:
    spec = find_module(name)
    if spec is None or spec.origin is None or not spec.origin.endswith(".py"):
        return None
    return Path(spec.origin), Path(spec.origin).read_text(encoding="utf-8")


def imported_modules(name: str, source: str) -> set[str]:
    found: set[str] = set()
    for node in ast.walk(ast.parse(source)):
        if isinstance(node, ast.Import):
            found |= {a.name for a in node.names if a.name.startswith("pilot")}
        elif isinstance(node, ast.ImportFrom) and node.module and node.module.startswith("pilot"):
            found.add(node.module)
            found |= {f"{node.module}.{a.name}" for a in node.names if find_module(f"{node.module}.{a.name}")}
    return found


def reachable(roots: tuple[str, ...]) -> dict[str, str]:
    seen: dict[str, str] = {}
    pending = list(roots)
    while pending:
        name = pending.pop()
        if name in seen or (found := module_source(name)) is None:
            continue
        seen[name] = found[1]
        pending += sorted(imported_modules(name, found[1]))
    return seen


def docstring_nodes(tree: ast.AST) -> set[int]:
    ids = set()
    for node in ast.walk(tree):
        if isinstance(node, ast.Module | ast.ClassDef | ast.FunctionDef | ast.AsyncFunctionDef):
            first = node.body[0] if node.body else None
            if isinstance(first, ast.Expr) and isinstance(first.value, ast.Constant):
                ids.add(id(first.value))
    return ids


def forbidden_uses(source: str) -> list[str]:
    tree = ast.parse(source)
    skipped = docstring_nodes(tree)
    uses = []
    for node in ast.walk(tree):
        if isinstance(node, ast.Constant) and isinstance(node.value, str) and id(node) not in skipped:
            uses += [f"string {node.value!r} names {word}" for word in FORBIDDEN_STRINGS if re.search(_WORD.format(re.escape(word)), node.value, re.IGNORECASE)]
        if isinstance(node, ast.Attribute) and node.attr in FORBIDDEN_NAMES:
            uses.append(f"attribute .{node.attr}")
        if isinstance(node, ast.Name) and node.id in FORBIDDEN_NAMES:
            uses.append(f"name {node.id}")
    return uses


@pytest.fixture(scope="module")
def pipeline_modules() -> dict[str, str]:
    return reachable(ROOT_MODULES)


def test_the_walk_reaches_the_pipeline_and_its_shared_foundations(pipeline_modules):
    assert {"pilot.cli.pipeline_commands", "pilot.application.core_run", "pilot.application.secondary_run",
            "pilot.application.relabel_stage", "pilot.domain.constants", "pilot.adapters.model_cli"} <= set(pipeline_modules)


def test_no_ground_truth_module_is_reachable(pipeline_modules):
    assert sorted(set(pipeline_modules) & set(GROUND_TRUTH_MODULES)) == []
    assert [m for m in pipeline_modules if m.startswith("pilot.analysis")] == []


def test_no_reachable_module_names_the_ground_truth(pipeline_modules):
    offences = {name: uses for name, source in pipeline_modules.items() if (uses := forbidden_uses(source))}
    assert offences == {}


def test_the_detector_flags_ground_truth_paths_and_ignores_docstrings_and_look_alikes():
    assert forbidden_uses('x = Path("prereg") / "violations"')
    assert forbidden_uses('p = "prereg/arm_p/labels.yaml"')
    assert forbidden_uses("layout.oracle")
    assert forbidden_uses('d = "oracle"')
    assert not forbidden_uses('def f():\n    """Reads the oracle."""\n    return {"isolation_violations": 1}')


def test_the_detector_follows_function_level_imports():
    source = "def f():\n    from pilot.cli.layout import ExperimentLayout\n"
    assert "pilot.cli.layout" in imported_modules("m", source)
