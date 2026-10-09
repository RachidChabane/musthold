"""Shared inputs of the site tests: the repository root, the fixture build and a figure builder."""

from __future__ import annotations

from collections.abc import Mapping
from pathlib import Path
from typing import Final

from specbench.adapters.site_inputs import read_fixture_inputs
from specbench.site.build import build_site
from specbench.site.figures import Comparison, Estimate, Figure, FigureKind, Sample, Unit
from specbench.site.model import SiteInputs

REPOSITORY_ROOT: Final = Path(__file__).resolve().parents[3]
GOLDEN_COMMIT: Final = "0123456789abcdef0123456789abcdef01234567"
GOLDEN_GENERATOR: Final = "specbench golden"
TRUNKS: Final = Unit("trunk", "trunks")


def fixture_inputs() -> SiteInputs:
    return read_fixture_inputs(REPOSITORY_ROOT, GOLDEN_COMMIT, GOLDEN_GENERATOR)


def fixture_site() -> Mapping[str, str]:
    return build_site(fixture_inputs())


def make_figure(
    value: float | None,
    lo: float | None,
    hi: float | None,
    n: int = 6,
    kind: FigureKind = FigureKind.RATE,
    comparison: Comparison = Comparison.NONE,
    reference: bool = False,
) -> Figure:
    return Figure(
        "harnesses/claude-code/arms/ALL/yield",
        kind,
        Estimate(value, lo, hi),
        Sample(n, TRUNKS, 6),
        comparison,
        reference,
    )
