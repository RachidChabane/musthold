"""A scan's outputs as the product writes them: its printed headline, scan.json and finding.json."""

from __future__ import annotations

import json

COMMIT = "1a2b3c4d5e6f708192a3b4c5d6e7f8091a2b3c4d"
BASE = "9f8e7d6c5b4a39281706f5e4d3c2b1a098f7e6d5"

HEADLINE = """\
north/ledger @ 1a2b3c4 · pytest · 40 tests (38 green 3/3) · 30 sampled · seed 7 · change since 9f8e7d6
  full scan · 1h0m0s budget · 12m4s · 30 of 38 green tests sampled · 2 workers
  2/30 verify nothing   21/30 caught ≥ 1 change   7/30 unproven   8/38 not sampled
  unproven: 5 with < 3 proven changes or < 2 sites, 2 with an inconclusive outcome
  Not a pass: unproven and unsampled tests are unjudged.

#1 tests/test_totals.py::test_total_accepts_any_rate passes on all 6 proven changes in lines it ran
   ledger/totals.py:41  >= → >
   proven by: tests/test_totals.py::test_total_adds_rate kills it 3/3
   repro: .spec-proof/scan/1a2b3c4/1/repro.sh

#2 tests/test_rates.py::test_rate[high value] passes on all 4 proven changes in lines it ran; 1 more stopped it from starting
   ledger/rates.py:12  0.2 → 1.2   input amount=10: reference 2, mutant 12
   proven by: a differential run of rate, reproduced, on a reference that agrees with itself
   repro: .spec-proof/scan/1a2b3c4/2/repro.sh
120 mutants (64 proven, 3 did not build or start, 11 not run within the budget) · 12m4s (reference 20s, coverage 1m2s, draw 1s, mutants 10m41s) · no model used · promises not checked
2/30 sampled tests stay green on every proven change in lines they ran: on those lines, their green is no evidence the code is right.
"""


def statement(verifies_nothing: int = 2, partial: bool = False) -> str:
    mutants = [
        {"id": f"m{index}", "origin": "operator", "edits": [], "lines": [], "routes": [{"kind": "killed_by_test"}]}
        for index in range(64)
    ] + [{"id": f"n{index}", "origin": "operator", "edits": [], "lines": []} for index in range(42)]
    predicate: dict[str, object] = {
        "module": {"name": "north/ledger"},
        "producedBy": "local",
        "mode": "change",
        "base": BASE,
        "runner": "pytest",
        "seed": "7",
        "promisesChecked": False,
        "counts": {
            "tests": 40,
            "verifiesNothing": verifies_nothing,
            "caught": 21,
            "unproven": 9 - verifies_nothing,
            "notSampled": 8,
            "notGreen": 2,
        },
        "mutants": mutants,
        "tests": [],
    }
    if partial:
        predicate["partial"] = True
    return json.dumps(
        {
            "_type": "https://in-toto.io/Statement/v1",
            "subject": [{"name": COMMIT, "digest": {"sha256": "ab" * 32}}],
            "predicateType": "https://spec-proof.dev/scan/v1",
            "predicate": predicate,
        }
    )


def finding(test: str, kind: str = "killed_by_test", killer: str | None = None) -> str:
    route: dict[str, object] = {"kind": kind}
    if killer is not None:
        route["test"] = killer
    return json.dumps(
        {
            "commit": COMMIT,
            "runner": "pytest",
            "test": test,
            "mutant": {"id": "m1", "origin": "operator", "edits": [{"file": "ledger/totals.py"}], "lines": []},
            "route": route,
        }
    )
