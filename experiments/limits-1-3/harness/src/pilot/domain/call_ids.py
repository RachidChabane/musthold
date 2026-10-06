"""Call identifiers: a call id names a directory under `raw/`, and one id is never used for two calls."""

from __future__ import annotations

from typing import Final

GENERATE_CALL_ID: Final = "generate__{ref}__{index}"
WRITE_TESTS_CALL_ID: Final = "write_tests__{ref}__{writer}"
LUNA_GENERATE_CALL_ID: Final = "xgen__{ref}__{writer}"
RELABEL_CALL_ID: Final = "relabel__{rater}"
VALIDITY_CALL_ID: Final = "validity__{judge}__{mutant_id}"
AUDIT_CALL_ID: Final = "audit__{judge}__{mutant_id}"
ALIGNMENT_CALL_ID: Final = "align__{judge}__{arm}__{slug}__r{rep}"
PROBE_CALL_ID: Final = "probe__{judge}__{slug}__r{rep}"
