"""What the H writer and the structural checks need to know of the Go code under test, read from the merge sources.

Only package clauses, import declarations and `go.mod` requirements are read: never a test, never a function body.
"""

from __future__ import annotations

import posixpath
import re
from collections.abc import Iterable, Mapping
from dataclasses import dataclass, field
from typing import Final

from specbench.domain.go_source import go_imports, go_package_name
from specbench.domain.languages import GO_TEST_SUFFIX, Language

GO_SUFFIX: Final = ".go"
ROOT_DIRECTORY: Final = "."
SIGNATURE_QUALIFIER: Final = re.compile(r"(?<![\w.\"/])([A-Za-z_][A-Za-z0-9_]*)\.[A-Za-z_]")
REQUIRE_BLOCK_START: Final = re.compile(r"^require\s*\(\s*$")
REQUIRE_SINGLE: Final = re.compile(r"^require\s+(\S+)\s+\S+")
REQUIRE_MEMBER: Final = re.compile(r"^(\S+)\s+v\S+")
INDIRECT_MARKER: Final = "// indirect"
BLOCK_END: Final = ")"


@dataclass(frozen=True)
class GoTarget:
    """The Go packages of the Interface: their declared names, and the third-party modules its signatures need."""

    module: str = ""
    package_by_directory: Mapping[str, str] = field(default_factory=dict[str, str])
    third_party_modules: frozenset[str] = frozenset()

    def package_of(self, directory: str) -> str | None:
        return self.package_by_directory.get(directory)

    def import_path_of(self, directory: str) -> str:
        return self.module if directory == ROOT_DIRECTORY else f"{self.module}/{directory}"

    def statement(self) -> str:
        """The package facts the H writer is told, one line per package directory; empty when none is known."""
        return "\n".join(
            f"- Package `{name}`, import path `{self.import_path_of(directory)}`: declare `package {name}_test` in its "
            f'files and import it as `{name} "{self.import_path_of(directory)}"`.'
            for directory, name in sorted(self.package_by_directory.items())
        )

    def allows_third_party(self, import_path: str) -> bool:
        return any(import_path == m or import_path.startswith(m + "/") for m in self.third_party_modules)


def direct_requirements(go_mod: str) -> frozenset[str]:
    """The module paths a `go.mod` requires directly, those marked `// indirect` left out."""
    found: set[str] = set()
    in_block = False
    for raw in go_mod.splitlines():
        line = raw.strip()
        if in_block and line == BLOCK_END:
            in_block = False
        elif in_block:
            member = REQUIRE_MEMBER.match(line)
            if member is not None and INDIRECT_MARKER not in line:
                found.add(member.group(1))
        elif REQUIRE_BLOCK_START.match(line):
            in_block = True
        else:
            single = REQUIRE_SINGLE.match(line)
            if single is not None and INDIRECT_MARKER not in line:
                found.add(single.group(1))
    return frozenset(found)


def _directory_of(path: str) -> str:
    return posixpath.dirname(path) or ROOT_DIRECTORY


def package_names_by_directory(directories: Iterable[str], sources: Mapping[str, str]) -> dict[str, str]:
    """The package name of each directory from the package clauses of its non-test `.go` sources.

    A directory whose sources disagree, or that has none, is left out: nothing can be stated for it.
    """
    wanted = frozenset(directories)
    names: dict[str, set[str]] = {}
    for path, source in sources.items():
        if not path.endswith(GO_SUFFIX) or path.endswith(GO_TEST_SUFFIX) or _directory_of(path) not in wanted:
            continue
        if (name := go_package_name(source)) is not None:
            names.setdefault(_directory_of(path), set()).add(name)
    return {directory: next(iter(found)) for directory, found in names.items() if len(found) == 1}


def go_directory_of(import_path: str, module: str) -> str | None:
    """The module-relative directory of a package import path, or None when it lies outside the module."""
    if import_path == module:
        return ROOT_DIRECTORY
    return import_path.removeprefix(module + "/") if import_path.startswith(module + "/") else None


def _is_under(path: str, module: str) -> bool:
    return path == module or path.startswith(module + "/")


def third_party_modules(signatures: Iterable[str], sources: Mapping[str, str], go_mod: str) -> frozenset[str]:
    """Direct requirements the Interface's signatures name: by import path, or by the qualifier a source imports it as."""
    requirements = direct_requirements(go_mod)
    text = "\n".join(signatures)
    qualifiers = frozenset(SIGNATURE_QUALIFIER.findall(text))
    modules = {m for m in requirements if m in text}
    for path, source in sources.items():
        if not path.endswith(GO_SUFFIX):
            continue
        for imported in go_imports(source):
            if imported.qualifier in qualifiers:
                modules |= {m for m in requirements if _is_under(imported.path, m)}
    return frozenset(modules)


def go_target_of(
    language: Language,
    module: str,
    entries: Iterable[tuple[str, str]],
    sources: Mapping[str, str],
    go_mod: str,
) -> GoTarget:
    """The target of a task from its Interface entries (import path, signature), merge sources and `go.mod`."""
    if language is not Language.GO:
        return GoTarget()
    pairs = list(entries)
    directories = {go_directory_of(path, module) for path, _ in pairs} - {None}
    return GoTarget(
        module,
        package_names_by_directory([d for d in directories if d is not None], sources),
        third_party_modules((signature for _, signature in pairs), sources, go_mod),
    )
