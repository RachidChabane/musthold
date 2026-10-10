"""Sealing with age through the `pyrage` library (the `age` CLI is not required) and the identity-file rules."""

from __future__ import annotations

import importlib
from collections.abc import Sequence
from pathlib import Path
from typing import Protocol, cast

from specbench.domain.h_selection import require_selected_h
from specbench.domain.sealing import SealedBundle, SealingError, pack_bundle, require_current_version, unpack_bundle

IDENTITY_PREFIX = "AGE-SECRET-KEY-"
IDENTITY_FILE_MODE = 0o600


class _Key(Protocol):
    def to_public(self) -> object: ...


class _Age(Protocol):
    """The slice of `pyrage` that the sealer uses; the library ships no type stubs."""

    def encrypt(self, plaintext: bytes, recipients: Sequence[object]) -> bytes: ...

    def decrypt(self, ciphertext: bytes, identities: Sequence[object]) -> bytes: ...


class _IdentityType(Protocol):
    def generate(self) -> _Key: ...

    def from_str(self, text: str) -> object: ...


class _RecipientType(Protocol):
    def from_str(self, text: str) -> object: ...


class _X25519(Protocol):
    Identity: _IdentityType
    Recipient: _RecipientType


def _age() -> _Age:
    return cast(_Age, importlib.import_module("pyrage"))


def _x25519() -> _X25519:
    return cast(_X25519, importlib.import_module("pyrage.x25519"))


class InsideRepositoryError(ValueError):
    """A path for sealed plaintext or an identity that lies inside the repository an agent can reach."""


def refuse_inside_repository(path: Path, forbidden_root: Path, what: str) -> Path:
    """The resolved path, or an error when it lies inside `forbidden_root`."""
    resolved = path.resolve()
    if resolved.is_relative_to(forbidden_root.resolve()):
        raise InsideRepositoryError(f"{what} {path} is inside {forbidden_root}")
    return resolved


def generate_identity() -> tuple[str, str]:
    """A fresh X25519 age identity and its recipient, as text."""
    key = _x25519().Identity.generate()
    return str(key), str(key.to_public())


class AgeSealStore:
    """`SealStore` over age: the bundle is packed deterministically, then encrypted to every recipient."""

    def seal(self, bundle: SealedBundle, recipients: Sequence[str]) -> bytes:
        require_current_version(bundle)
        require_selected_h(bundle)
        if not recipients:
            raise SealingError("sealing needs at least one recipient")
        parsed = [_x25519().Recipient.from_str(r) for r in recipients]
        return _age().encrypt(pack_bundle(bundle), parsed)

    def unseal(self, ciphertext: bytes, identity: str) -> SealedBundle:
        try:
            plaintext = _age().decrypt(ciphertext, [_x25519().Identity.from_str(identity)])
        except Exception as error:
            raise SealingError(f"cannot unseal: {error}") from error
        return unpack_bundle(plaintext)


def read_identity_file(path: Path, forbidden_root: Path) -> str:
    """The identity in a file that lives outside `forbidden_root`, the repository an agent can read."""
    resolved = refuse_inside_repository(path, forbidden_root, "identity")
    for line in resolved.read_text(encoding="utf-8").splitlines():
        if line.startswith(IDENTITY_PREFIX):
            return line.strip()
    raise SealingError(f"{path} holds no age identity")


def write_identity_file(path: Path, identity: str, forbidden_root: Path) -> None:
    resolved = refuse_inside_repository(path, forbidden_root, "identity")
    resolved.parent.mkdir(parents=True, exist_ok=True)
    resolved.write_text(identity + "\n", encoding="utf-8")
    resolved.chmod(IDENTITY_FILE_MODE)
