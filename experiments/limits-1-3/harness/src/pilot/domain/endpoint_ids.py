"""Identifiers of the decisional endpoints (PREREG 10)."""

from __future__ import annotations

from typing import Final

ENDPOINT_COUNT: Final = 11
ENDPOINT_IDS: Final = tuple(f"D{n}" for n in range(1, ENDPOINT_COUNT + 1))
