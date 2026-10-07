"""Architecture isolation: the verbs that prepare and run the measurement never reach the truth of the pilot.

The analysis is by static import analysis of the `calibration` package, independent of import-linter. The pilot reader
module is included in the scan: it states its allowlist, never a denylist, so it names no truth file."""

from __future__ import annotations

import ast
from collections.abc import Iterator
from pathlib import Path
from typing import Final

import pytest

from calibration.adapters.pilot_run_reader import AllowlistedPilotRunReader, ForbiddenPilotFile

PACKAGE: Final = "calibration"
RUN_VERBS: Final = f"{PACKAGE}.cli.run_verbs"
ANALYSIS: Final = f"{PACKAGE}.analysis"
TRUTH_MODULE: Final = f"{ANALYSIS}.truth"
PILOT_READER_MODULE: Final = f"{PACKAGE}.adapters.pilot_run_reader"
PROTECTED_LAYERS: Final = (f"{PACKAGE}.domain", f"{PACKAGE}.application", f"{PACKAGE}.adapters")
SOURCE_ROOT: Final = Path(__file__).resolve().parents[2] / "src"
PYTHON_SUFFIX: Final = ".py"
PACKAGE_MODULE: Final = "__init__"
ENCODING: Final = "utf-8"
TRUTH_FILE_NAMES: Final = (
    "oracle_classes.json",
    "relabel.jsonl",
    "kills.jsonl",
    "mutant_classes.json",
    "alignment.jsonl",
    "alignment_aggregate.json",
    "validity.jsonl",
    "labels.yaml",
    "planted_map.json",
)
READER_PREFIXES: Final = ("", "./", "../", "mutants/", "raw/", "arm_p/")


def module_file(module: str) -> Path | None:
    base = SOURCE_ROOT.joinpath(*module.split("."))
    if base.with_suffix(PYTHON_SUFFIX).is_file():
        return base.with_suffix(PYTHON_SUFFIX)
    package = base / f"{PACKAGE_MODULE}{PYTHON_SUFFIX}"
    return package if package.is_file() else None


def modules_below(package: str) -> list[str]:
    root = SOURCE_ROOT.joinpath(*package.split("."))
    names: list[str] = []
    for path in sorted(root.rglob(f"*{PYTHON_SUFFIX}")):
        parts = path.relative_to(SOURCE_ROOT).with_suffix("").parts
        names.append(".".join(parts[:-1] if parts[-1] == PACKAGE_MODULE else parts))
    return names


def parsed(module: str) -> ast.Module:
    path = module_file(module)
    assert path is not None, module
    return ast.parse(path.read_text(encoding=ENCODING))


def enclosing_packages(module: str) -> Iterator[str]:
    parts = module.split(".")
    for length in range(1, len(parts)):
        yield ".".join(parts[:length])


def imported_modules(module: str) -> set[str]:
    """Every project module that importing `module` executes: what it imports (anywhere in its body), `from` targets
    that are submodules, and the packages those modules sit in."""
    named: set[str] = set()
    for node in ast.walk(parsed(module)):
        if isinstance(node, ast.Import):
            named.update(alias.name for alias in node.names)
        elif isinstance(node, ast.ImportFrom) and node.module and node.level == 0:
            named.add(node.module)
            named.update(f"{node.module}.{alias.name}" for alias in node.names)
    project = {name for name in named if name.split(".")[0] == PACKAGE and module_file(name) is not None}
    return project | {package for name in project for package in enclosing_packages(name)}


def reachable_from(entry: str) -> set[str]:
    seen: set[str] = set()
    pending = [entry]
    while pending:
        current = pending.pop()
        if current in seen:
            continue
        seen.add(current)
        pending.extend(imported_modules(current) - seen)
    return seen


def string_literals(module: str) -> list[str]:
    return [
        node.value
        for node in ast.walk(parsed(module))
        if isinstance(node, ast.Constant) and isinstance(node.value, str)
    ]


def truth_names_in(module: str) -> set[str]:
    return {name for literal in string_literals(module) for name in TRUTH_FILE_NAMES if name in literal}


def imports_analysis(module: str) -> bool:
    return any(name == ANALYSIS or name.startswith(f"{ANALYSIS}.") for name in imported_modules(module))


@pytest.fixture(scope="module")
def run_side_modules() -> set[str]:
    return reachable_from(RUN_VERBS)


def test_the_run_side_reach_includes_the_pilot_reader_and_the_run_verbs_themselves(run_side_modules: set[str]) -> None:
    assert {RUN_VERBS, PILOT_READER_MODULE, f"{PACKAGE}.cli.wiring", f"{PACKAGE}.domain.constants"} <= run_side_modules


def test_no_module_reachable_from_the_run_verbs_imports_the_analysis_layer(run_side_modules: set[str]) -> None:
    assert not {name for name in run_side_modules if name == ANALYSIS or name.startswith(f"{ANALYSIS}.")}


def test_no_module_reachable_from_the_run_verbs_contains_a_truth_file_name(run_side_modules: set[str]) -> None:
    offenders = {name: truth_names_in(name) for name in run_side_modules if truth_names_in(name)}
    assert offenders == {}


def test_the_scan_detects_the_truth_names_where_they_live() -> None:
    assert {"oracle_classes.json", "kills.jsonl", "labels.yaml"} <= truth_names_in(TRUTH_MODULE)


@pytest.mark.parametrize("layer", PROTECTED_LAYERS)
def test_a_protected_layer_never_imports_the_analysis_layer(layer: str) -> None:
    assert [name for name in modules_below(layer) if imports_analysis(name)] == []


def test_the_analysis_layer_is_reachable_from_the_composition_root_only_through_the_analysis_verbs() -> None:
    importers = [name for name in modules_below(PACKAGE) if imports_analysis(name) and not name.startswith(ANALYSIS)]
    assert set(importers) <= {f"{PACKAGE}.cli.analysis_verbs", f"{PACKAGE}.cli.main"}


@pytest.fixture
def pilot_run_dir(tmp_path: Path) -> Path:
    directory = tmp_path / "run-02"
    for name in TRUTH_FILE_NAMES:
        (directory / name).parent.mkdir(parents=True, exist_ok=True)
        (directory / name).write_text("{}")
    return directory


@pytest.mark.parametrize("prefix", READER_PREFIXES)
@pytest.mark.parametrize("name", TRUTH_FILE_NAMES)
def test_the_pilot_reader_refuses_every_truth_file_even_when_it_exists(
    pilot_run_dir: Path, prefix: str, name: str
) -> None:
    reader = AllowlistedPilotRunReader(pilot_run_dir)
    with pytest.raises(ForbiddenPilotFile):
        reader.read_text(prefix + name)
    with pytest.raises(ForbiddenPilotFile):
        reader.exists(prefix + name)
    assert reader.digests() == {}
