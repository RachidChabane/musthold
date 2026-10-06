"""A plain table: the unit of every CSV and of every Markdown table in the report."""

from __future__ import annotations

import math
from collections.abc import Sequence
from dataclasses import dataclass
from typing import Any

Cell = Any
DECIMALS = 3
EMPTY_CELL = ""
PIPE = "|"
ESCAPED_PIPE = "\\|"


def format_cell(value: Cell, decimals: int = DECIMALS) -> str:
    if value is None:
        return EMPTY_CELL
    if isinstance(value, float):
        return EMPTY_CELL if math.isnan(value) else f"{value:.{decimals}f}"
    return str(value)


@dataclass(frozen=True)
class Table:
    name: str
    title: str
    header: Sequence[str]
    rows: Sequence[Sequence[Cell]]
    decimals: int = DECIMALS

    def markdown(self) -> str:
        lines = [f"| {' | '.join(self.header)} |", f"|{'|'.join('---' for _ in self.header)}|"]
        for row in self.rows:
            cells = (format_cell(c, self.decimals).replace(PIPE, ESCAPED_PIPE) for c in row)
            lines.append(f"| {' | '.join(cells)} |")
        return "\n".join(lines)

    def csv_rows(self) -> list[list[str]]:
        return [list(self.header), *([format_cell(c, self.decimals) for c in row] for row in self.rows)]
