"""The CI identity of a clean-room gate: one run id and one seed per tree, whatever the arm (ADR 0011, decision 12).

The product draws the operator mutants and sentinels of a CI run from a seed, the SHA-256 of the head commit and the
first CI run that saw it. A clean room commits the agent's tree alone first and each arm's setup on top, so the heads of
two arms differ. The seed of every arm is therefore derived from the agent-only commit and the run id, and the seed
record of the final head is restored with that value: the same tree is drawn the same CI requests in every arm.
"""

from __future__ import annotations

import hashlib
import json
from dataclasses import dataclass
from typing import Final

from specbench.domain.arm_policy import MAX_HOST_WAITS
from specbench.domain.canonical import JsonValue

CI_RUN_PREFIX: Final = "bench"
RUN_ID_DIGEST_LENGTH: Final = 16
SEED_SEPARATOR: Final = b"\x00"
"""The byte between the head commit and the run id in the seed hash (`hostbundle.CISeed`)."""
SEED_BYTES: Final = 8
SEEDS_DIRECTORY: Final = ".spec-proof/seeds"
SEED_RECORD_SUFFIX: Final = ".json"
CI_ROUND_TRIPS: Final = MAX_HOST_WAITS
"""The most CI round trips a gate makes: as many as Shallnot waits for the host in a row."""


def ci_run_id(tree_digest: str) -> str:
    """The id of the simulated CI run of a tree: `bench-` and the first 16 hex digits of the tree digest."""
    return f"{CI_RUN_PREFIX}-{tree_digest[:RUN_ID_DIGEST_LENGTH]}"


def ci_seed(head_sha: str, first_run_id: str) -> int:
    """The seed of a CI run, equal to `hostbundle.CISeed` of the product: the first 8 bytes, big-endian, of the SHA-256 of
    the head commit, a NUL byte and the run id."""
    digest = hashlib.sha256(head_sha.encode() + SEED_SEPARATOR + first_run_id.encode()).digest()
    return int.from_bytes(digest[:SEED_BYTES], "big")


@dataclass(frozen=True)
class SeedRecord:
    """What the product keeps for a head commit in `.spec-proof/seeds/<head>.json` and reuses on the next CI run."""

    head_sha: str
    first_run_id: str
    seed: int

    @classmethod
    def for_tree(cls, head_sha: str, agent_head: str, run_id: str) -> SeedRecord:
        """The record of `head_sha` carrying the seed of the agent-only commit, so that it does not depend on the arm."""
        return cls(head_sha, run_id, ci_seed(agent_head, run_id))

    @property
    def path(self) -> str:
        return f"{SEEDS_DIRECTORY}/{self.head_sha}{SEED_RECORD_SUFFIX}"

    def to_record(self) -> dict[str, JsonValue]:
        return {"headSha": self.head_sha, "firstRunId": self.first_run_id, "seed": self.seed}

    def to_json(self) -> str:
        return json.dumps(self.to_record(), separators=(",", ":"))


@dataclass(frozen=True)
class CiIdentity:
    """The CI identity of a clean room: the run id of its tree, the agent-only commit and the head after the arm's setup."""

    run_id: str
    agent_head: str
    head: str

    @property
    def seed_record(self) -> SeedRecord:
        return SeedRecord.for_tree(self.head, self.agent_head, self.run_id)
