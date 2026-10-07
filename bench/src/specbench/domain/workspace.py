"""The agent workspace: the base tree plus the agent-visible spec and the product's init output, never a sealed byte."""

from __future__ import annotations

from collections.abc import Mapping
from dataclasses import dataclass

from specbench.domain.oracle import SPEC_PATH
from specbench.domain.sealing import SealedBundle, sealed_leaks


class WorkspaceLeak(ValueError):
    """A file that the builder itself must emit would expose sealed material."""


@dataclass(frozen=True)
class Workspace:
    files: Mapping[str, bytes]
    omitted: tuple[str, ...]


def protected_files(bundle: SealedBundle) -> dict[str, bytes]:
    """Every sealed file except the spec, which is the one sealed file the agent is shown."""
    return {path: content for path, content in bundle.files.items() if path != SPEC_PATH}


def build_workspace(
    base_tree: Mapping[str, bytes], spec: bytes, init_files: Mapping[str, bytes], bundle: SealedBundle
) -> Workspace:
    """The files of a fresh single-commit workspace.

    Base files that collide with a sealed path or hold sealed bytes are omitted and
    reported; the spec and the init output must be clean, or the build fails.
    """
    protected = protected_files(bundle)
    kept: dict[str, bytes] = {}
    omitted: list[str] = []
    for path in sorted(base_tree):
        if sealed_leaks([(path, base_tree[path])], protected):
            omitted.append(path)
        else:
            kept[path] = base_tree[path]
    added = {SPEC_PATH: spec, **init_files}
    leaks = sealed_leaks(sorted(added.items()), protected)
    if leaks:
        raise WorkspaceLeak("; ".join(leaks))
    return Workspace({**kept, **added}, tuple(omitted))
