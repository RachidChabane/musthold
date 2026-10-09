"""Interval charts as inline SVG, drawn at build time: one row per arm, a dot for the estimate and a bar for its interval.

The chart scales to its container; colours come from the stylesheet, so both themes apply. A row whose figure
is not estimable says so in place of a dot. Every row is repeated in the SVG description for screen readers,
and the page shows the same figures in a table beside it.
"""

from __future__ import annotations

import math
from collections.abc import Callable, Sequence
from dataclasses import dataclass
from typing import Final

from specbench.site.figures import Figure, FigureKind, Reading, format_interval, format_value, reading
from specbench.site.html import Markup, element, join

WIDTH: Final = 640.0
LABEL_WIDTH: Final = 104.0
LABEL_GAP: Final = 10.0
RIGHT_MARGIN: Final = 20.0
TOP_MARGIN: Final = 8.0
ROW_HEIGHT: Final = 30.0
AXIS_HEIGHT: Final = 34.0
DOT_RADIUS: Final = 5.0
CAP_HALF_HEIGHT: Final = 6.0
TICK_LENGTH: Final = 5.0
LABEL_BASELINE_OFFSET: Final = 4.0
TICK_LABEL_OFFSET: Final = 18.0
COORDINATE_DECIMALS: Final = 1
RATE_DOMAIN: Final = (0.0, 1.0)
DOMAIN_STEP: Final = 0.5
RATE_TICKS: Final = 5
NOT_ESTIMABLE: Final = "not estimable"


@dataclass(frozen=True)
class ChartRow:
    label: str
    figure: Figure


def _c(value: float) -> str:
    return f"{round(value, COORDINATE_DECIMALS) + 0.0:.{COORDINATE_DECIMALS}f}"


def _domain(rows: Sequence[ChartRow], kind: FigureKind, bounded: bool) -> tuple[float, float]:
    if bounded:
        return RATE_DOMAIN
    values = [0.0]
    for row in rows:
        estimate = row.figure.estimate
        values += [v for v in (estimate.lo, estimate.hi, estimate.value) if v is not None]
    step = DOMAIN_STEP if kind is FigureKind.RATE else 1.0
    lo = math.floor(min(values) / step) * step
    hi = math.ceil(max(values) / step) * step
    return (lo, hi if hi > lo else lo + step)


def _ticks(domain: tuple[float, float]) -> list[float]:
    lo, hi = domain
    return [lo + (hi - lo) * index / (RATE_TICKS - 1) for index in range(RATE_TICKS)]


def _row_text(row: ChartRow) -> str:
    figure = row.figure
    estimate = figure.estimate
    if reading(figure) is Reading.UNDEFINED or estimate.value is None or estimate.lo is None or estimate.hi is None:
        return f"{row.label}: {NOT_ESTIMABLE}"
    return (
        f"{row.label}: {format_value(figure.kind, estimate.value)} "
        f"{format_interval(figure.kind, estimate.lo, estimate.hi)}, {figure.sample.describe()}"
    )


def interval_chart(title: str, rows: Sequence[ChartRow], bounded: bool, chart_id: str) -> Markup:
    """An SVG chart of `rows`; `bounded` charts span 0 to 100 %, the others span their data and zero."""
    kind = rows[0].figure.kind if rows else FigureKind.RATE
    domain = _domain(rows, kind, bounded)
    plot_left, plot_right = LABEL_WIDTH, WIDTH - RIGHT_MARGIN
    height = TOP_MARGIN + ROW_HEIGHT * len(rows) + AXIS_HEIGHT
    axis_y = TOP_MARGIN + ROW_HEIGHT * len(rows)

    def x_of(value: float) -> float:
        lo, hi = domain
        return plot_left + (value - lo) / (hi - lo) * (plot_right - plot_left)

    parts: list[Markup] = [
        element("title", {"id": f"{chart_id}-title"}, title),
        element("desc", {"id": f"{chart_id}-desc"}, "; ".join(_row_text(row) for row in rows)),
    ]
    for tick in _ticks(domain):
        x = _c(x_of(tick))
        parts.append(element("line", {"class": "chart-grid", "x1": x, "x2": x, "y1": _c(TOP_MARGIN), "y2": _c(axis_y)}))
        parts.append(
            element(
                "text",
                {"class": "chart-tick", "x": x, "y": _c(axis_y + TICK_LABEL_OFFSET), "text-anchor": "middle"},
                format_value(kind, tick),
            )
        )
    if domain[0] < 0.0 < domain[1]:
        zero = _c(x_of(0.0))
        parts.append(
            element("line", {"class": "chart-zero", "x1": zero, "x2": zero, "y1": _c(TOP_MARGIN), "y2": _c(axis_y)})
        )
    parts.append(
        element(
            "line",
            {"class": "chart-axis", "x1": _c(plot_left), "x2": _c(plot_right), "y1": _c(axis_y), "y2": _c(axis_y)},
        )
    )
    for index, row in enumerate(rows):
        parts.append(_row(row, TOP_MARGIN + ROW_HEIGHT * (index + 0.5), x_of))
    return element(
        "svg",
        {
            "class": "chart",
            "viewBox": f"0 0 {_c(WIDTH)} {_c(height)}",
            "role": "img",
            "aria-labelledby": f"{chart_id}-title {chart_id}-desc",
            "preserveAspectRatio": "xMinYMin meet",
        },
        join(parts),
    )


def _row(row: ChartRow, y: float, x_of: Callable[[float], float]) -> Markup:
    figure = row.figure
    estimate = figure.estimate
    label = element(
        "text",
        {
            "class": "chart-label",
            "x": _c(LABEL_WIDTH - LABEL_GAP),
            "y": _c(y + LABEL_BASELINE_OFFSET),
            "text-anchor": "end",
        },
        row.label,
    )
    undefined = reading(figure) is Reading.UNDEFINED
    if undefined or estimate.value is None or estimate.lo is None or estimate.hi is None:
        note = element(
            "text",
            {"class": "chart-missing", "x": _c(LABEL_WIDTH), "y": _c(y + LABEL_BASELINE_OFFSET)},
            NOT_ESTIMABLE,
        )
        return element("g", {"class": "chart-row"}, label, note)
    lo, hi, value = x_of(estimate.lo), x_of(estimate.hi), x_of(estimate.value)
    css = "chart-row is-reference" if figure.reference else "chart-row"
    marks = [
        element("line", {"class": "chart-bar", "x1": _c(lo), "x2": _c(hi), "y1": _c(y), "y2": _c(y)}),
        element(
            "line",
            {
                "class": "chart-cap",
                "x1": _c(lo),
                "x2": _c(lo),
                "y1": _c(y - CAP_HALF_HEIGHT),
                "y2": _c(y + CAP_HALF_HEIGHT),
            },
        ),
        element(
            "line",
            {
                "class": "chart-cap",
                "x1": _c(hi),
                "x2": _c(hi),
                "y1": _c(y - CAP_HALF_HEIGHT),
                "y2": _c(y + CAP_HALF_HEIGHT),
            },
        ),
        element("circle", {"class": "chart-dot", "cx": _c(value), "cy": _c(y), "r": _c(DOT_RADIUS)}),
    ]
    return element("g", {"class": css}, element("title", None, _row_text(row)), label, *marks)
