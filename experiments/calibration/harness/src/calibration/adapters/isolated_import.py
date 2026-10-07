"""Imports the `pilot` package of one unpacked snapshot without leaving a trace in the interpreter."""

from __future__ import annotations

import importlib
import sys
from collections.abc import Generator, Sequence
from contextlib import contextmanager
from pathlib import Path
from types import ModuleType
from typing import Final

PILOT_PACKAGE: Final = "pilot"
PILOT_MODULE_PREFIX: Final = PILOT_PACKAGE + "."


def _purge_pilot_modules() -> None:
    for name in [name for name in sys.modules if name == PILOT_PACKAGE or name.startswith(PILOT_MODULE_PREFIX)]:
        del sys.modules[name]


@contextmanager
def isolated_pilot_path(source_root: Path) -> Generator[None]:
    """Puts `source_root` first on `sys.path` with no `pilot*` module loaded; restores both on exit.

    No bytecode is written below the snapshot while the context is open.
    """
    saved_path = list(sys.path)
    saved_bytecode_flag = sys.dont_write_bytecode
    _purge_pilot_modules()
    sys.path.insert(0, str(source_root))
    sys.dont_write_bytecode = True
    importlib.invalidate_caches()
    try:
        yield
    finally:
        sys.dont_write_bytecode = saved_bytecode_flag
        sys.path[:] = saved_path
        _purge_pilot_modules()
        importlib.invalidate_caches()


def import_pilot_modules(source_root: Path, names: Sequence[str]) -> list[ModuleType]:
    """The named modules, imported from `source_root` only; two snapshots never share a module object."""
    with isolated_pilot_path(source_root):
        return [importlib.import_module(name) for name in names]
