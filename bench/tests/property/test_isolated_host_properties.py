from __future__ import annotations

import json

from hypothesis import given
from hypothesis import strategies as st

from specbench.domain.canonical import JsonValue
from specbench.domain.clean_room import PORTABLE_ROOT, portable
from specbench.domain.shallnot_config import ProofTrust, arm_shallnot_config

KINDS = st.lists(st.sampled_from(["alignment", "discriminating-power"]), unique=True, max_size=2).map(tuple)
BASES = st.sampled_from(
    [
        "version: 1\nspecs:\n  - specs\n",
        "version: 1\nproofs:\n  require:\n    - alignment\n  trust: local\n  code:\n    - src\nseverities:\n  x: error\n",
        "proofs:\n  trust: local\n  thresholds:\n    validity: 0.7\n",
    ]
)
SEGMENTS = st.text(alphabet="abc/_.-", min_size=0, max_size=12)


@given(BASES, KINDS, st.sampled_from(ProofTrust))
def test_the_arm_config_is_a_fixed_point_of_itself(base: str, require: tuple[str, ...], trust: ProofTrust) -> None:
    once = arm_shallnot_config(base, require, trust)
    assert arm_shallnot_config(once, require, trust) == once


@given(BASES, KINDS, st.sampled_from(ProofTrust))
def test_the_arm_config_names_exactly_the_required_kinds_and_the_trust(
    base: str, require: tuple[str, ...], trust: ProofTrust
) -> None:
    lines = arm_shallnot_config(base, require, trust).splitlines()
    assert lines.count(f"  trust: {trust.value}") == 1
    listed = [line.strip()[2:] for line in lines if line.startswith("    - ") and line.strip()[2:] in set(require)]
    assert sorted(listed) == sorted(require)
    assert sum(line.startswith("  require:") for line in lines) == 1


@given(SEGMENTS, st.lists(SEGMENTS, max_size=4))
def test_no_tree_path_survives_in_a_portable_report(suffix: str, others: list[str]) -> None:
    tree = "/runs/r1/cleanroom/task/ALL/abc/settle/tree"
    report: JsonValue = {"files": [f"{tree}/{suffix}", *[f"{tree}/{other}" for other in others]], "n": 1}
    assert tree not in json.dumps(portable(report, [tree]))
    assert json.dumps(portable(report, [tree])).count(PORTABLE_ROOT) == 1 + len(others)
