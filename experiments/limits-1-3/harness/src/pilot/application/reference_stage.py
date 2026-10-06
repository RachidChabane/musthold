"""Stage reference: the unmodified fixture passes the full suite on every confirmation run (PREREG 6.4 F2)."""

from __future__ import annotations

from pilot.application.ports import RunRequest, RunStore, VariantRunner
from pilot.application.process_mutants import ReferenceUnstable
from pilot.domain.constants import REFERENCE_RUN_COUNT
from pilot.domain.pipeline_constants import REFERENCE_FILENAME


def ensure_reference(store: RunStore, runner: VariantRunner) -> list[str]:
    """Runs the reference REFERENCE_RUN_COUNT times; returns the sorted node ids of the suite."""
    if store.exists(REFERENCE_FILENAME):
        recorded = store.read_json(REFERENCE_FILENAME)
        if recorded["passed_all"]:
            return sorted(recorded["runs"][0])
    requests = {f"reference-{n}": RunRequest() for n in range(1, REFERENCE_RUN_COUNT + 1)}
    results = runner.run_many(requests)
    ordered = [results[name] for name in sorted(results)]
    passed_all = all(result.all_passed for result in ordered)
    store.write_json(
        REFERENCE_FILENAME,
        {
            "runs": [
                {nodeid: outcome.value for nodeid, outcome in sorted(result.outcomes.items())}
                for result in ordered
            ],
            "passed_all": passed_all,
        },
    )
    if not passed_all:
        raise ReferenceUnstable("the reference did not pass every confirmation run")
    return sorted(ordered[0].outcomes)
