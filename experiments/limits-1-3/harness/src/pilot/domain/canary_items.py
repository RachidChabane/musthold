"""Synthetic canary items, one per call type, in a domain unrelated to the fixture (PREREG 4.4)."""

from __future__ import annotations

from collections.abc import Mapping
from typing import Final

from pilot.domain.call_type import CallType
from pilot.domain.prompts import (
    alignment_item,
    generation_item,
    leak_audit_item,
    probe_item,
    relabel_item,
    validity_item,
    writer_item,
)

CANARY_TOKENS: Final = tuple(f"K-{n:04d}" for n in range(1, 11))
CANARY_NONCE: Final = "canary"
CANARY_SPEC: Final = "\n".join(
    f"- PARK-{n}~1: WHEN the text {clause} THE SYSTEM SHALL {outcome}."
    for n, (clause, outcome) in enumerate(
        [
            ("is empty", "report 0 words"),
            ("holds one word", "report 1 word"),
            ("holds words separated by single spaces", "report one word per separated token"),
            ("holds words separated by several spaces", "count each word once"),
            ("starts with spaces", "ignore the leading spaces"),
            ("ends with spaces", "ignore the trailing spaces"),
            ("holds only spaces", "report 0 words"),
            ("holds a tab between words", "treat the tab as a separator"),
            ("holds a line break between words", "treat the line break as a separator"),
            ("holds a word of more than 40 characters", "count it as one word"),
        ],
        start=1,
    )
)
CANARY_SOURCE: Final = {
    "src/wordstat/count.py": "def count_words(text: str) -> int:\n    return len(text.split())\n",
}
CANARY_ORIGINAL: Final = "# src/wordstat/count.py :: count_words\ndef count_words(text: str) -> int:\n    return len(text.split())"
CANARY_MUTANT: Final = "# src/wordstat/count.py :: count_words\ndef count_words(text: str) -> int:\n    return len(text.split(' '))"
CANARY_DIFF: Final = (
    "--- a/src/wordstat/count.py\n+++ b/src/wordstat/count.py\n@@ -1,2 +1,2 @@\n"
    " def count_words(text: str) -> int:\n-    return len(text.split())\n+    return len(text.split(' '))"
)
CANARY_TEST: Final = (
    "from wordstat import count_words\n\n\ndef test_case():\n    assert count_words('a b') == 2\n"
)
CANARY_SIGNATURES: Final = "def count_words(text: str) -> int:\n    ..."
CANARY_CODEBOOK: Final = (
    "GOOD: the test checks the declared requirement and fails when it is violated.\n"
    "WEAK: the test checks a strictly weaker property.\nVACUOUS: the test cannot fail.\n"
    "MIS_OTHER: the test checks another requirement.\nOFFSPEC: the test pins behaviour no requirement covers.\n"
    "GOOD_BROAD: the test also depends on a second declared requirement."
)
CANARY_TESTS: Final = (
    "=== tests/test_words.py ===\nfrom wordstat import count_words\n\n\n"
    "# nodeid: tests/test_words.py::test_two_words  declares: PARK-3~1\n"
    "def test_two_words():\n    assert count_words('a b') == 2\n"
)
CANARY_LEAK_FILES: Final = {
    "spec/words.md": CANARY_SPEC,
    **CANARY_SOURCE,
    "tests/test_words.py": CANARY_TEST,
}


def canary_item(call_type: CallType, templates: Mapping[CallType, str]) -> str:
    template = templates[call_type]
    if call_type is CallType.GENERATE:
        target = CANARY_SPEC.splitlines()[2]
        return generation_item(
            template, CANARY_SPEC, "PARK-3~1", target.split(": ", 1)[1], CANARY_SOURCE, CANARY_NONCE
        )
    if call_type is CallType.WRITE_TESTS:
        target = CANARY_SPEC.splitlines()[2]
        return writer_item(
            template, CANARY_SPEC, "PARK-3~1", target.split(": ", 1)[1], CANARY_SOURCE, CANARY_SIGNATURES, CANARY_NONCE
        )
    if call_type in (CallType.VALIDITY, CallType.AUDIT):
        return validity_item(template, CANARY_SPEC, CANARY_ORIGINAL, CANARY_MUTANT, CANARY_DIFF, "")
    if call_type is CallType.ALIGNMENT:
        criteria = "\n".join(
            f"{token}: {line.split(': ', 1)[1]}"
            for token, line in zip(CANARY_TOKENS, CANARY_SPEC.splitlines(), strict=True)
        )
        return alignment_item(template, criteria, CANARY_TEST, CANARY_SIGNATURES)
    if call_type is CallType.PROBE:
        return probe_item(template, CANARY_TEST)
    if call_type is CallType.RELABEL:
        return relabel_item(template, CANARY_CODEBOOK, CANARY_SPEC, CANARY_SOURCE, CANARY_TESTS)
    return leak_audit_item(template, CANARY_LEAK_FILES)
