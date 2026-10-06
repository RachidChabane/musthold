"""Who does what (PREREG 4.1): judges, raters and generators with their pinned models."""

from __future__ import annotations

from enum import Enum
from typing import Final

from pilot.domain.model_call import Cli, ModelPin

OPUS: Final = ModelPin("claude-opus-5-5", Cli.CLAUDE)
SONNET: Final = ModelPin("claude-sonnet-5-5", Cli.CLAUDE)
TERRA: Final = ModelPin("gpt-5.6-terra", Cli.CODEX)


class Judge(Enum):
    V_S = "V-S"
    V_G = "V-G"
    A_OPUS = "A-OPUS"
    J_SONNET = "J-SONNET"
    J_GPT = "J-GPT"
    R_OAI = "R-OAI"
    R_AUTHOR = "R-AUTHOR"


JUDGE_PIN: Final = {
    Judge.V_S: SONNET,
    Judge.V_G: TERRA,
    Judge.A_OPUS: OPUS,
    Judge.J_SONNET: SONNET,
    Judge.J_GPT: TERRA,
    Judge.R_OAI: TERRA,
    Judge.R_AUTHOR: OPUS,
}

VALIDITY_JUDGES: Final = (Judge.V_S, Judge.V_G)
ALIGNMENT_JUDGES: Final = (Judge.J_SONNET, Judge.J_GPT)
RELABEL_RATERS: Final = (Judge.R_OAI, Judge.R_AUTHOR)
GENERATORS: Final = (OPUS, OPUS, OPUS, TERRA, TERRA)
