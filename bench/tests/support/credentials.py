"""Credential sources for the tests, and a fake `security` tool that stands in for the macOS keychain."""

from __future__ import annotations

import stat
from dataclasses import dataclass
from enum import StrEnum
from pathlib import Path

from specbench.application.ports import CredentialSourceFailed

FAKE_SECURITY_NAME = "security"
FAKE_TOKEN = "sk-ant-oat01-FAKE-d41d8cd98f00b204e9800998ecf8427e"
FAKE_SECURITY_FAILURE_EXIT = 51


class KeychainItem(StrEnum):
    PRESENT = "present"
    ABSENT = "absent"
    FAILING = "failing"


@dataclass(frozen=True)
class StaticCredentialSource:
    """A source that holds the given token, or none."""

    value: str | None

    def token(self) -> str | None:
        return self.value


class FailingCredentialSource:
    """A source that cannot tell."""

    def token(self) -> str | None:
        raise CredentialSourceFailed("the keychain tool exited with code 51")


def install_fake_security(directory: Path, item: KeychainItem, token: str = FAKE_TOKEN) -> Path:
    """Writes a `security` executable into `directory`. It logs its arguments next to itself, and answers a
    `find-generic-password` query as macOS does for an item that exists, that does not (exit 44) or that fails; the
    failing mode also prints the token on both streams, to show that nothing it prints is kept."""
    directory.mkdir(parents=True, exist_ok=True)
    log = directory / "calls.log"
    script = f"""#!/bin/sh
printf '%s\\n' "$*" >> "{log}"
printf 'inherited-token=%s\\n' "${{CLAUDE_CODE_OAUTH_TOKEN-unset}}" >> "{log}"
case "{item.value}" in
present) echo "{token}"; exit 0;;
absent) echo "{token}" >&2; exit 44;;
*) echo "{token}"; echo "{token}" >&2; exit {FAKE_SECURITY_FAILURE_EXIT};;
esac
"""
    executable = directory / FAKE_SECURITY_NAME
    executable.write_text(script, encoding="utf-8")
    executable.chmod(executable.stat().st_mode | stat.S_IXUSR)
    return executable


def security_calls(directory: Path) -> list[str]:
    log = directory / "calls.log"
    return log.read_text(encoding="utf-8").splitlines() if log.is_file() else []
