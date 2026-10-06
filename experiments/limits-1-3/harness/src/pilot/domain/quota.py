"""Quota and rate-limit failures of a model CLI: they pause a stage, they never mark a call missing."""

from __future__ import annotations

import re
from typing import Final

QUOTA_PATTERNS: Final = (
    r"usage limit",
    r"rate[ _-]?limit",
    r"\bquota\b",
    r"insufficient_quota",
    r"too many requests",
    r"\b429\b",
    r"hit your limit",
    r"limit (has been )?reached",
    r"credit balance",
    r"resource.?exhausted",
)
_QUOTA_REGEX: Final = re.compile("|".join(QUOTA_PATTERNS), re.IGNORECASE)


class QuotaExhausted(Exception):
    """A model call hit a quota or rate limit; the stage stops and a rerun resumes it."""


def looks_like_quota_exhausted(*streams: str) -> bool:
    return any(_QUOTA_REGEX.search(stream) for stream in streams)
