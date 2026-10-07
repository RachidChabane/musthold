"""The fixed text of `REPORT.md` and of its lint (§15.2): headings, notes, the limits paragraph and the vendor table."""

from __future__ import annotations

from typing import Final

TITLE: Final = "# Calibration of System One confidence per source (limit 4)"
SECTION_PREFIX: Final = "## "
SOURCE2_NOTE: Final = "Source-(2) votes answered the pilot's prompts, not the product's closed question sets."
QUOTED_BEGIN: Final = "<!-- quoted:begin -->"
QUOTED_END: Final = "<!-- quoted:end -->"
CONCLUSION_BEGIN: Final = "<!-- conclusion:begin -->"
CONCLUSION_END: Final = "<!-- conclusion:end -->"
EXPLORATORY_LABEL: Final = "Exploratory"
EXPLORATORY_WORD_LIMIT: Final = 300
ADR_PHRASE: Final = "confirm ALIGNED"

SECTIONS: Final = (
    "Provenance",
    "Bands",
    "Conclusions",
    "Source (3): UNDETERMINED",
    "Admission test",
    "Parameter proposals",
    "Reliability and risk-coverage",
    "Determinism",
    "Comparisons",
    "Checks",
    "Sensitivity analyses",
    "Machine safety",
    "Deviations",
    "Vendor-reported figures",
    "Limits",
    "Exploratory discussion",
)

LIMITS: Final = (
    "one author-built fixture, one language and one domain;",
    "38 tests and 10 criteria;",
    "alignment truth comes from author labels verified by execution against a hand-written violation set;",
    "mutant truth is relative to model-written oracles, whose author model is not recorded;",
    "the mutant items are model-written and planted mutants, while the product screens operator mutants;",
    "the screen state shows all ten statements without the specification's precedence paragraph, and its witness "
    "comes from the pilot's differential;",
    "the source-(2) votes answered the pilot's prompts on the pilot's inputs, not the product's closed question "
    "sets, and one host model cast all of them;",
    "the alignment items are too few for the admission test;",
    "decisions within a mutant are correlated;",
    "one laptop, and server numerics depend on hardware and kernels;",
    "intervals are unadjusted.",
)

VENDOR_FIGURES: Final = (
    (
        "nimble 9B",
        "decision accuracy on Bespoke Labs' 13 public datasets",
        "75.7 %",
        "https://ollama.com/library/nimble",
    ),
    ("tev1 4B", "decision accuracy on the same datasets", "73.3 %", "https://ollama.com/library/tev1"),
    ("tev1 0.8B", "decision accuracy on the same datasets", "63.5 %", "https://ollama.com/library/tev1"),
    ("Kev 0.8B", "transfer-v4 test accuracy", "69.7 %", "https://huggingface.co/jaredpalmer/kev-0.8b"),
    ("Kev 0.8B", "documents-v1 accuracy", "85.1 %", "https://huggingface.co/jaredpalmer/kev-0.8b"),
    ("Kev 0.8B", "breadth-v1 held-out accuracy", "58.6 %", "https://huggingface.co/jaredpalmer/kev-0.8b"),
    ("Kev 0.8B", "hard-v1 accuracy", "66.5 %", "https://huggingface.co/jaredpalmer/kev-0.8b"),
    ("Kev 0.8B", "coverage at 5 % error or less", "14.5 %", "https://huggingface.co/jaredpalmer/kev-0.8b"),
)
VENDOR_MARK: Final = "vendor-reported"
