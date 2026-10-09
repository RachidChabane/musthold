"""The pieces every page shares: a figure with its interval and sample, a count, a verdict, a chart with its table.

A figure and a count carry `data-figure`, the path of the value in `results.json`, and the raw values they show,
so the site check can verify each one against the results it was built from.
"""

from __future__ import annotations

import json
from collections.abc import Sequence
from typing import Final

from specbench.site.charts import ChartRow, interval_chart
from specbench.site.figures import (
    MIN_REPOSITORIES_FOR_NOMINAL_COVERAGE,
    Figure,
    FigureKind,
    Reading,
    caution_text,
    cautions,
    format_interval,
    format_value,
    reading,
    reading_text,
    too_few_repositories,
)
from specbench.site.html import Content, Markup, classed, element, join

FIGURE_CLASS: Final = "fig"
COUNT_CLASS: Final = "count"
REFERENCE_SUBJECT: Final = "tests only (T)"
VERDICT_CLASSES: Final = {
    "pass": "is-pass",
    "fail": "is-fail",
    "blocked": "is-blocked",
    "let-go": "is-let-go",
    "escalate": "is-escalate",
    "passed": "is-pass",
    "failed": "is-fail",
    "errored": "is-fail",
    "skipped": "is-escalate",
    "not_run": "is-escalate",
}


def raw(value: object) -> str:
    """The JSON text of a value, as the check reads it back from `results.json`."""
    return json.dumps(value)


def figure(fig: Figure, subject: str = REFERENCE_SUBJECT) -> Markup:
    """A figure: estimate, interval, sample size, and its reading and cautions when it has any."""
    estimate = fig.estimate
    state = reading(fig)
    attributes: dict[str, str | None] = {
        "class": FIGURE_CLASS,
        "data-figure": fig.pointer,
        "data-estimate": raw(estimate.value),
        "data-lo": raw(estimate.lo),
        "data-hi": raw(estimate.hi),
        "data-n": str(fig.sample.n),
    }
    parts: list[Content] = []
    if state is Reading.REFERENCE:
        attributes["class"] = f"{FIGURE_CLASS} is-reference"
        parts.append(classed("span", "est", reading_text(fig, subject)))
    elif state is Reading.UNDEFINED or estimate.value is None or estimate.lo is None or estimate.hi is None:
        attributes["class"] = f"{FIGURE_CLASS} is-undefined"
        parts.append(classed("span", "est", reading_text(fig, subject)))
    else:
        parts += [
            classed("span", "est", format_value(fig.kind, estimate.value)),
            " ",
            classed("span", "ci", format_interval(fig.kind, estimate.lo, estimate.hi)),
        ]
        text = reading_text(fig, subject)
        if text:
            parts.append(classed("span", "reading", text))
        parts += [classed("span", "caution", caution_text(caution, fig)) for caution in cautions(fig)]
    parts.append(classed("span", "n", fig.sample.describe()))
    return element("span", attributes, *parts)


def count(pointer: str, value: object, shown: str | None = None) -> Markup:
    """A count or a fact read from the results, with the raw value it shows."""
    return element(
        "span",
        {"class": COUNT_CLASS, "data-figure": pointer, "data-value": raw(value)},
        shown if shown is not None else str(value),
    )


def percent_of(level: float) -> str:
    return format_value(FigureKind.RATE, level)


def verdict(value: str) -> Markup:
    css = VERDICT_CLASSES.get(value, "is-unknown")
    return element("span", {"class": f"verdict {css}"}, value)


def coverage_note(pointer: str, repositories: int) -> Markup:
    """The warning every table of a small sample carries."""
    if not too_few_repositories(repositories):
        return Markup("")
    return classed(
        "p",
        "note",
        join(
            [
                count(pointer, repositories),
                f" repositories. The percentile cluster bootstrap needs about {MIN_REPOSITORIES_FOR_NOMINAL_COVERAGE} "
                "repositories to reach its nominal coverage, so these intervals are likely too narrow; "
                "read every figure as a description of this sample.",
            ]
        ),
    )


def chart_figure(title: str, caption: str, rows: Sequence[ChartRow], bounded: bool, chart_id: str) -> Markup:
    """An interval chart with its caption; the page shows the same figures in a table beside it."""
    return element(
        "figure",
        {"class": "chart-figure"},
        element("figcaption", None, caption),
        interval_chart(title, rows, bounded, chart_id),
    )
