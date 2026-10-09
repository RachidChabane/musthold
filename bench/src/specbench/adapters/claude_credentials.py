"""Where the long-lived Claude Code token comes from: the macOS keychain or the environment.

The owner stores the token once, with `security add-generic-password -s spec-proof-claude-oauth -a "$(id -un)" -w`.
A source returns the value or `None` when it holds none; it fails with `CredentialSourceFailed`, whose message never
carries the value or anything a tool printed. The caller puts the value into the environment of the one subprocess that
needs it.
"""

from __future__ import annotations

import os
import pwd
import subprocess
from collections.abc import Mapping, Sequence
from typing import Final

from specbench.application.ports import ClaudeCredentialSource, CredentialSourceFailed

OAUTH_TOKEN_VARIABLE: Final = "CLAUDE_CODE_OAUTH_TOKEN"
KEYCHAIN_SERVICE: Final = "spec-proof-claude-oauth"
SECURITY_EXECUTABLE: Final = "security"
SECURITY_ITEM_NOT_FOUND: Final = 44
"""The exit code of `security find-generic-password` for an item that does not exist."""
SECURITY_TIMEOUT_SECONDS: Final = 30
"""A locked keychain can wait for the owner; a query that long is a failure, not an absence."""


def without_oauth_token(environ: Mapping[str, str]) -> dict[str, str]:
    """The environment of every process that is not the Claude Code session itself."""
    return {name: value for name, value in environ.items() if name != OAUTH_TOKEN_VARIABLE}


def login_name() -> str:
    """The macOS login, as `id -un` prints it."""
    return pwd.getpwuid(os.getuid()).pw_name


class EnvironmentCredentialSource:
    """The variable `CLAUDE_CODE_OAUTH_TOKEN` of the given environment."""

    def __init__(self, environ: Mapping[str, str]) -> None:
        self._environ = environ

    def token(self) -> str | None:
        return self._environ.get(OAUTH_TOKEN_VARIABLE) or None


class KeychainCredentialSource:
    """The keychain item `service` of `account`, read through `security`."""

    def __init__(
        self,
        service: str = KEYCHAIN_SERVICE,
        account: str | None = None,
        executable: str = SECURITY_EXECUTABLE,
    ) -> None:
        self._service = service
        self._account = account
        self._executable = executable

    def token(self) -> str | None:
        argv = [
            self._executable,
            "find-generic-password",
            "-s",
            self._service,
            "-a",
            self._account or login_name(),
            "-w",
        ]
        try:
            completed = subprocess.run(
                argv,
                capture_output=True,
                text=True,
                timeout=SECURITY_TIMEOUT_SECONDS,
                env=without_oauth_token(os.environ),
                check=False,
            )
        except FileNotFoundError:
            raise CredentialSourceFailed(f"the {self._executable} executable was not found") from None
        except subprocess.TimeoutExpired:
            raise CredentialSourceFailed(f"{self._executable} did not answer in {SECURITY_TIMEOUT_SECONDS} s") from None
        if completed.returncode == SECURITY_ITEM_NOT_FOUND:
            return None
        if completed.returncode != 0:
            raise CredentialSourceFailed(f"{self._executable} exited with code {completed.returncode}")
        return completed.stdout.strip() or None


class FirstCredentialSource:
    """The first source, in order, that holds a token."""

    def __init__(self, sources: Sequence[ClaudeCredentialSource]) -> None:
        self._sources = tuple(sources)

    def token(self) -> str | None:
        for source in self._sources:
            token = source.token()
            if token:
                return token
        return None


def default_credential_source(environ: Mapping[str, str]) -> ClaudeCredentialSource:
    """The environment first, then the keychain."""
    return FirstCredentialSource([EnvironmentCredentialSource(environ), KeychainCredentialSource()])


def token_or_none(source: ClaudeCredentialSource) -> str | None:
    """The token of a source, or `None` when it holds none or cannot tell; the failure is never carried further."""
    try:
        return source.token()
    except CredentialSourceFailed:
        return None
