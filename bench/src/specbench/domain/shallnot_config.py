"""An arm's Shallnot configuration: the project's `shallnot.yaml` with the arm's `proofs.require` and `proofs.trust`."""

from __future__ import annotations

from enum import Enum
from typing import Final

PROOFS_KEY: Final = "proofs:"
REQUIRE_KEY: Final = "require:"
TRUST_KEY: Final = "trust:"
BLOCK_INDENT: Final = "  "
ITEM_INDENT: Final = "    "


class ProofTrust(Enum):
    """Whose statements the gate reads: only the verifier-checked, CI-produced ones, or also unsigned local ones."""

    SIGNED = "signed"
    LOCAL = "local"


def _is_top_level_key(line: str) -> bool:
    return bool(line) and not line[0].isspace() and not line.startswith("#")


def _split_proofs_block(lines: list[str]) -> tuple[list[str], list[str], list[str]]:
    """The lines before the `proofs:` block, the block's body, and the lines after it."""
    start = next((i for i, line in enumerate(lines) if line.rstrip() == PROOFS_KEY), None)
    if start is None:
        return lines, [], []
    end = next((i for i in range(start + 1, len(lines)) if _is_top_level_key(lines[i])), len(lines))
    return lines[:start], lines[start + 1 : end], lines[end:]


def _without_entries(body: list[str], keys: tuple[str, ...]) -> list[str]:
    """The block body without the entries named by `keys`, each with its nested lines."""
    kept: list[str] = []
    skipping = False
    for line in body:
        is_entry = line.startswith(BLOCK_INDENT) and not line.startswith(BLOCK_INDENT + " ") and bool(line.strip())
        if is_entry:
            skipping = line.strip().startswith(keys)
        if not skipping:
            kept.append(line)
    return kept


def arm_shallnot_config(base: str, require: tuple[str, ...], trust: ProofTrust) -> str:
    """`base` with its `proofs.require` replaced by `require` and its `proofs.trust` by `trust`; every other line stays."""
    before, body, after = _split_proofs_block(base.splitlines())
    required = [f"{BLOCK_INDENT}{REQUIRE_KEY}", *[f"{ITEM_INDENT}- {kind}" for kind in require]]
    if not require:
        required = [f"{BLOCK_INDENT}{REQUIRE_KEY} []"]
    kept = _without_entries(body, (REQUIRE_KEY, TRUST_KEY))
    block = [PROOFS_KEY, *required, f"{BLOCK_INDENT}{TRUST_KEY} {trust.value}", *kept]
    return "\n".join([*before, *block, *after]) + "\n"
