"""A synthetic run: trunk records drawn from a fixed seed, with the structure of a sample of the real benchmark."""

from __future__ import annotations

import random
from pathlib import Path

from specbench.adapters.run_store import RunStore
from specbench.domain.arms import FROZEN_MODEL, Harness
from specbench.domain.canonical import JsonValue
from specbench.domain.manifest import Manifest, RunLayout

SEED = 7
REPOSITORIES = (
    ("acme/parkfee", "python"),
    ("acme/ledger", "python"),
    ("acme/tzkit", "python"),
    ("acme/gopher", "go"),
    ("acme/router", "go"),
    ("acme/shelf", "go"),
)
HARNESSES = (Harness.CLAUDE_CODE, Harness.CODEX)
GATING = {"TR": 0.3, "AL": 0.5, "DP": 0.6, "CO": 0.3, "ALL": 0.75, "PL": 0.4, "ALL-native": 0.75}


def _truth(rng: random.Random, defective: bool) -> dict[str, JsonValue]:
    return {"defective": defective, "defective_full": defective or rng.random() < 0.15, "o_failed": rng.random() < 0.2}


def _arm(rng: random.Random, harness: Harness, defective: bool, block_rate: float | None) -> dict[str, JsonValue]:
    blocks = block_rate is not None and defective and rng.random() < block_rate
    false_block = block_rate is not None and not defective and rng.random() < 0.2
    rejected = blocks or false_block
    repaired = blocks and rng.random() < 0.5
    final_defective = defective and (not blocks or (not repaired and rng.random() < 0.4))
    verdict = "pass" if not rejected or repaired else rng.choice(["let-go", "escalate", "blocked"])
    return {
        "escalated_during": verdict == "escalate",
        "final": _truth(rng, final_defective),
        "model": FROZEN_MODEL[harness],
        "model_calls": rng.randint(8, 30) if block_rate is not None else 0,
        "seconds": rng.randint(300, 2400) if block_rate is not None else 120,
        "selection_pass": not rejected if block_rate is not None else True,
        "verdict": verdict,
    }


def _blind_spot(rng: random.Random, task_index: int) -> list[JsonValue]:
    cells: list[JsonValue] = []
    for criterion in (1, 2):
        for writer in ("coder", "upstream"):
            for source in ("generator", "opus", "operator"):
                evaluable = rng.randint(2, 6)
                base = {"coder": 0.35, "upstream": 0.25}[writer] + (0.1 if source != "operator" else 0.0)
                escaped = sum(rng.random() < base for _ in range(evaluable))
                cells.append(
                    {
                        "criterion": f"T{task_index}-{criterion}",
                        "writer": writer,
                        "source": source,
                        "escaped": escaped,
                        "evaluable": evaluable,
                    }
                )
    return cells


def trunk_record(
    rng: random.Random, index: int, repository: str, language: str, harness: Harness
) -> dict[str, JsonValue]:
    defective = rng.random() < 0.4
    reached = rng.random() < 0.9
    arms: dict[str, JsonValue] = {}
    if reached:
        arms["T"] = _arm(rng, harness, defective, None)
        arms["N"] = {**_arm(rng, harness, defective, None), "selection_pass": None}
        for name, rate in GATING.items():
            arms[name] = _arm(rng, harness, defective, rate)
    return {
        "absent_probes": [
            {"arm": "TR", "module": "alignment", "counted_as_passed": False},
            {"arm": "AL", "module": "discriminating-power", "counted_as_passed": False},
        ],
        "arms": arms,
        "blind_spot": _blind_spot(rng, index),
        "composition": [
            {
                "criterion": f"T{index}-{c}",
                "module": module,
                "all_verdict": "pass",
                "composed_verdict": "pass" if rng.random() < 0.95 else "escalate",
            }
            for c in (1, 2)
            for module in ("alignment", "discriminating-power")
        ],
        "decisions": [
            {
                "source": src,
                "confidence_permille": rng.randint(500, 1000) if src != "undetermined" else None,
                "truth": rng.random() < 0.8 if src != "undetermined" else None,
            }
            for src in rng.choices(["endpoint", "host-votes", "undetermined"], weights=[3, 4, 1], k=12)
        ],
        "harness": harness.value,
        "judge_replay": {
            "replay_model": "claude-opus-5-5" if harness is Harness.CLAUDE_CODE else "claude-sonnet-5-5",
            "same_pass": rng.random() < 0.5,
            "replay_pass": rng.random() < 0.5,
        },
        "language": language,
        "model": FROZEN_MODEL[harness],
        "reached_s_t": reached,
        "repository": repository,
        "s_n": _truth(rng, defective or rng.random() < 0.2),
        "s_t": _truth(rng, defective) if reached else None,
        "schema": "specbench.trunk/1",
        "task_id": f"T{index}",
    }


def synthetic_trunks() -> list[dict[str, JsonValue]]:
    rng = random.Random(SEED)
    return [
        trunk_record(rng, index, repository, language, harness)
        for index, (repository, language) in enumerate(REPOSITORIES, start=1)
        for harness in HARNESSES
    ]


def write_synthetic_run(root: Path, manifest: Manifest) -> RunLayout:
    layout = RunLayout(root)
    store = RunStore(layout)
    store.write_manifest(manifest)
    for record in synthetic_trunks():
        store.write_trunk(record, layout.trunk(str(record["task_id"]), Harness(str(record["harness"]))))
    return layout
