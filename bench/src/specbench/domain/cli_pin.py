"""The verdict on the Claude Code version a session reports against the version the run is frozen to."""

from __future__ import annotations


def claude_version_fault(pinned: str, reported: str | None) -> str | None:
    """Why a session did not run the pinned Claude Code, or `None` when it did.

    A session whose stream names no version is as much a fault as one that names another: the freeze cannot be shown.
    """
    if reported is None:
        return f"the stream reports no Claude Code version, the pin is {pinned}"
    if reported != pinned:
        return f"the session ran Claude Code {reported}, the pin is {pinned}"
    return None
