"""Who does what (PREREG 4.1): the host model, the contrast model, the judges, the writers and the mutant sources."""

from __future__ import annotations

from enum import Enum
from typing import Final

from pilot.domain.model_call import Cli, ModelPin

OPUS: Final = ModelPin("claude-opus-5-5", Cli.CLAUDE)
SONNET: Final = ModelPin("claude-sonnet-5-5", Cli.CLAUDE)
TERRA: Final = ModelPin("gpt-5.6-terra", Cli.CODEX)
LUNA: Final = ModelPin("gpt-6-luna", Cli.CODEX)

HOST: Final = SONNET
"""H: executes every product role (generation, both validity votes, the primary alignment judge)."""
CONTRAST: Final = OPUS
"""X: the measurement contrast, never a product role."""

CANARY_PINS: Final = (SONNET, OPUS)
"""The pins `canary` measures: those that make new calls in the decisional run, Claude only."""
LUNA_CANARY_PINS: Final = (LUNA,)
"""The pins `run-luna` measures before its own calls."""


class Judge(Enum):
    V_H1 = "V-H1"
    V_H2 = "V-H2"
    A_OPUS = "A-OPUS"
    J_H = "J-H"
    J_X = "J-X"
    R_OAI = "R-OAI"
    R_AUTHOR = "R-AUTHOR"


JUDGE_PIN: Final = {
    Judge.V_H1: HOST,
    Judge.V_H2: HOST,
    Judge.A_OPUS: OPUS,
    Judge.J_H: HOST,
    Judge.J_X: CONTRAST,
    Judge.R_OAI: TERRA,
    Judge.R_AUTHOR: OPUS,
}

VALIDITY_JUDGES: Final = (Judge.V_H1, Judge.V_H2)
ALIGNMENT_JUDGES: Final = (Judge.J_H, Judge.J_X)
RELABEL_RATERS: Final = (Judge.R_OAI, Judge.R_AUTHOR)


class Writer(Enum):
    """Test writers: the identifier is the suffix of the call id and the key of the suite."""

    SONNET = "S"
    OPUS = "O"
    LUNA = "L"
    FIXTURE = "F"


WRITER_PIN: Final = {Writer.SONNET: SONNET, Writer.OPUS: OPUS, Writer.LUNA: LUNA}
PRIMARY_WRITERS: Final = (Writer.SONNET, Writer.OPUS)


class MutantSource(Enum):
    """Where a mutant comes from; `OPERATOR` uses no model."""

    SONNET = "S"
    OPUS = "O"
    OPERATOR = "op"
    TERRA = "terra"
    LUNA = "luna"


PRIMARY_SOURCES: Final = (MutantSource.SONNET, MutantSource.OPUS, MutantSource.OPERATOR)
SOURCE_PIN: Final = {
    MutantSource.SONNET: SONNET,
    MutantSource.OPUS: OPUS,
    MutantSource.TERRA: TERRA,
    MutantSource.LUNA: LUNA,
}

ARM_T_CALLS_PER_CRITERION: Final = 5
ARM_T_FIRST_CALL_INDEX: Final = 6
CONTRAST_CALLS_PER_CRITERION: Final = 3
