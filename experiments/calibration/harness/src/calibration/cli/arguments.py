"""The slice of `argparse` sub-parsers the verb modules use."""

from __future__ import annotations

from argparse import ArgumentParser
from typing import Protocol


class Subparsers(Protocol):
    def add_parser(self, name: str, *, help: str) -> ArgumentParser: ...
