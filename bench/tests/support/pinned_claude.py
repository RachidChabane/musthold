"""A `PinnedClaude` for tests: a fixed version and a fixed executable, independent of `script/claude-code.pin`."""

from __future__ import annotations

import json
from collections.abc import Sequence
from dataclasses import dataclass
from typing import cast

from support.fake_claude_cli import DEFAULT_CLAUDE_VERSION

TEST_CLAUDE_VERSION = DEFAULT_CLAUDE_VERSION
OTHER_CLAUDE_VERSION = "2.1.291"
UNUSED_EXECUTABLE = "/pinned/bin/claude"


@dataclass(frozen=True)
class FixedClaude:
    path: str = UNUSED_EXECUTABLE
    version: str = TEST_CLAUDE_VERSION

    def executable(self) -> str:
        return self.path


def with_claude_version(lines: Sequence[str], version: str | None) -> list[str]:
    """The stream with `claude_code_version` set on its `system/init` event, or removed when `version` is `None`."""
    return [_stamped(line, version) for line in lines]


def _stamped(line: str, version: str | None) -> str:
    body: object = json.loads(line)
    if not isinstance(body, dict):
        return line
    event = cast("dict[str, object]", body)
    if event.get("type") != "system" or event.get("subtype") != "init":
        return line
    fields = {key: value for key, value in event.items() if key != "claude_code_version"}
    if version is not None:
        fields["claude_code_version"] = version
    return json.dumps(fields, separators=(",", ":"))
