"""Array aliases and the exact-sum primitives every statistic shares.

Sums go through `math.fsum`, which is correctly rounded and therefore independent of
the order of the terms and of the platform's vector units.
"""

from __future__ import annotations

import math
from typing import Final

import numpy as np
from numpy.typing import NDArray

Floats = NDArray[np.float64]
Flags = NDArray[np.int64]
Mask = NDArray[np.bool_]
Rows = NDArray[np.intp]

NAN: Final = float("nan")


def exact_sum(values: Floats | Flags) -> float:
    return math.fsum(values.tolist())


def exact_mean(values: Floats | Flags) -> float:
    if values.size == 0:
        return NAN
    return exact_sum(values) / values.size


def safe_ratio(numerator: float, denominator: float) -> float:
    return numerator / denominator if denominator != 0 else NAN


def json_number(value: float) -> float | None:
    return None if math.isnan(value) else value
