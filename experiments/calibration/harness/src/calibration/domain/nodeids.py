"""Pytest node ids of the bound tests (PC3 a): `<file>::<Classname>::<name>`, the class part left out when empty."""

from __future__ import annotations

from typing import Final

NODE_SEPARATOR: Final = "::"
MODULE_SUFFIX: Final = ".py"
PATH_SEPARATOR: Final = "/"
DOTTED_SEPARATOR: Final = "."


def nodeid_of(file: str, classname: str, name: str) -> str:
    """The node id of a test from Shallnot's `source.file`, JUnit `classname` and `name`.

    A JUnit classname is the dotted module path followed by the enclosing classes; what remains once the module
    path is removed is the class scope.
    """
    module = file.removesuffix(MODULE_SUFFIX).replace(PATH_SEPARATOR, DOTTED_SEPARATOR)
    scope = classname.removeprefix(module).strip(DOTTED_SEPARATOR)
    classes = [part for part in scope.split(DOTTED_SEPARATOR) if part]
    return NODE_SEPARATOR.join([file, *classes, name])
