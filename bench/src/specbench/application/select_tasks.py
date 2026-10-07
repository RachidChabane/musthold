"""`select`: walk the seeded ranking of each language, probe each candidate, keep the first feasible ones.

A candidate the probe rejects is logged with its stage and the next one in seeded order takes its place, so the
selection is deterministic given the seed and the probe's verdicts, and the funnel stays complete.
"""

from __future__ import annotations

from collections.abc import Mapping, Sequence

from specbench.application.ports import FeasibilityProbe
from specbench.domain.candidates import AttritionEntry, Candidate
from specbench.domain.languages import Language
from specbench.domain.mining import Rejection, Stage
from specbench.domain.selection import ProbedTask, Selection


def select_feasible(
    ranking: Mapping[Language, Sequence[Candidate]],
    probe: FeasibilityProbe,
    tasks_per_language: int,
    reserves_per_language: int,
) -> Selection:
    wanted = tasks_per_language + reserves_per_language
    tasks: list[ProbedTask] = []
    reserves: list[ProbedTask] = []
    attrition: list[AttritionEntry] = []
    for language in Language:
        accepted: list[ProbedTask] = []
        for candidate in ranking.get(language, ()):
            if len(accepted) >= wanted:
                attrition.append(AttritionEntry(candidate.label, Stage.NOT_SELECTED, "beyond the selection depth"))
                continue
            outcome = probe.probe(candidate)
            if isinstance(outcome, Rejection):
                attrition.append(AttritionEntry(candidate.label, outcome.stage, outcome.reason))
            else:
                accepted.append(outcome)
        tasks += accepted[:tasks_per_language]
        reserves += accepted[tasks_per_language:]
    return Selection(tuple(tasks), tuple(reserves), tuple(attrition))
