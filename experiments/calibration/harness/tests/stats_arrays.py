"""Typed array builders shared by the statistics tests."""

from __future__ import annotations

from collections.abc import Sequence

import numpy as np

from calibration.analysis.stats.arrays import Flags, Floats, Mask


def floats(values: Sequence[float]) -> Floats:
    return np.array(values, dtype=np.float64)


def flags(values: Sequence[int]) -> Flags:
    return np.array(values, dtype=np.int64)


def mask(values: Sequence[bool]) -> Mask:
    return np.array(values, dtype=np.bool_)
