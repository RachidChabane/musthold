from __future__ import annotations

from specbench.domain.shallnot_config import ProofTrust, arm_shallnot_config

ENABLED_CONFIG = """version: 1
specs:
  - specs
tests:
  - tests
results:
  - test-results/pytest.xml
test_commands:
  - pytest -o junit_family=xunit1 --junitxml=test-results/pytest.xml
proofs:
  require:
    - discriminating-power
    - alignment
  trust: local
  code:
    - parcel
"""


def test_the_arm_replaces_the_required_kinds_and_the_trust_and_keeps_the_rest() -> None:
    config = arm_shallnot_config(ENABLED_CONFIG, ("alignment",), ProofTrust.SIGNED)
    assert config == ENABLED_CONFIG.replace("    - discriminating-power\n", "").replace("trust: local", "trust: signed")


def test_an_arm_that_requires_nothing_writes_an_empty_list() -> None:
    config = arm_shallnot_config(ENABLED_CONFIG, (), ProofTrust.LOCAL)
    assert "  require: []\n  trust: local\n  code:\n    - parcel\n" in config
    assert "discriminating-power" not in config


def test_a_config_without_a_proofs_block_gets_one_at_the_end() -> None:
    base = "version: 1\nspecs:\n  - specs\n"
    assert arm_shallnot_config(base, ("alignment",), ProofTrust.SIGNED) == (
        "version: 1\nspecs:\n  - specs\nproofs:\n  require:\n    - alignment\n  trust: signed\n"
    )


def test_a_block_that_follows_the_proofs_block_is_left_alone() -> None:
    base = "proofs:\n  require:\n    - alignment\n  trust: local\nseverities:\n  skipped_requirement: warning\n"
    config = arm_shallnot_config(base, ("discriminating-power",), ProofTrust.SIGNED)
    assert config == (
        "proofs:\n  require:\n    - discriminating-power\n  trust: signed\nseverities:\n  skipped_requirement: warning\n"
    )


def test_the_thresholds_nested_in_the_proofs_block_stay() -> None:
    base = "proofs:\n  trust: local\n  thresholds:\n    validity: 0.7\n  require:\n    - alignment\n"
    config = arm_shallnot_config(base, (), ProofTrust.SIGNED)
    assert config == "proofs:\n  require: []\n  trust: signed\n  thresholds:\n    validity: 0.7\n"
