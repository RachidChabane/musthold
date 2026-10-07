"""What `analyze` checks before it computes anything (§2.2, §2.3, §14.1): blindness, qualification and the bindings."""

from __future__ import annotations

import json
from collections.abc import Mapping
from dataclasses import dataclass
from datetime import UTC, datetime
from pathlib import Path
from typing import Final

from calibration.analysis.run_data import RunData
from calibration.analysis.truth import TIMING_FILE, PilotTruth
from calibration.domain.canonical import JsonValue
from calibration.domain.constants import ENCODING
from calibration.domain.freeze import PreconditionFailed, stamped_before
from calibration.domain.jsonread import as_mapping, json_object, mapping, optional_text, text
from calibration.domain.pilot_records import MEASURED_ARMS
from calibration.domain.ports import Timestamp, TimestampReader, VersionControl

BLINDNESS: Final = "blindness"
SEAL: Final = "seal"
BINDINGS: Final = "bindings"
STARTED: Final = "started"


@dataclass(frozen=True)
class Qualification:
    """Whether run-02's oracle records cover every non-stillborn status-OK mutant of arms T and P (§2.2)."""

    evaluated: int
    with_oracle: int
    missing: tuple[str, ...]

    def to_record(self) -> dict[str, JsonValue]:
        return {"evaluated": self.evaluated, "with_oracle": self.with_oracle, "missing": list(self.missing)}


def qualification(truth: PilotTruth) -> Qualification:
    evaluated = sorted(
        row.mutant_id
        for row in truth.processing
        if row.evaluated
        and truth.mutants.get(row.mutant_id) is not None
        and truth.mutants[row.mutant_id].arm in MEASURED_ARMS
    )
    missing = tuple(mutant_id for mutant_id in evaluated if mutant_id not in truth.oracle)
    return Qualification(len(evaluated), len(evaluated) - len(missing), missing)


def binding_problems(truth: PilotTruth) -> list[str]:
    """`analyze` checks that D(t) equals the label's `cites` (§3.3)."""
    return [
        f"{label.nodeid}: bound to {truth.bindings.get(label.nodeid)}, labelled as citing {label.cites}"
        for label in truth.labels
        if truth.bindings.get(label.nodeid) != label.cites
    ]


def earliest_started(timing: Mapping[str, object]) -> datetime | None:
    """The earliest `started` of the pilot's `timing.json`, as an aware datetime."""
    moments: list[datetime] = []
    for entry in timing.values():
        record = as_mapping(entry)
        started = optional_text(record.get(STARTED), STARTED) if record is not None else None
        if started is None:
            continue
        moment = datetime.fromisoformat(started)
        moments.append(moment if moment.tzinfo else moment.astimezone())
    return min(moments).astimezone(UTC) if moments else None


def blindness_problems(
    stamp: Timestamp | None, analysis_commit_time: datetime, pilot_timing: Mapping[str, object]
) -> list[str]:
    """The tag's timestamp genTime must precede the pilot analysis commit's date and the earliest `started` of the
    pilot run's `timing.json`."""
    if stamp is None:
        return ["the timestamp of the pre-registration cannot be read"]
    problems: list[str] = []
    if not stamped_before(stamp, analysis_commit_time):
        problems.append(
            f"the tag was stamped at {stamp.generated_at.isoformat()}, not before the pilot analysis commit "
            f"at {analysis_commit_time.isoformat()}"
        )
    first = earliest_started(pilot_timing)
    if first is None:
        problems.append("the pilot run records no stage start")
    elif not stamped_before(stamp, first):
        problems.append(f"the tag was stamped at {stamp.generated_at.isoformat()}, not before the first pilot stage")
    return problems


def check_preconditions(
    run: RunData,
    truth: PilotTruth,
    vcs: VersionControl,
    timestamps: TimestampReader,
    receipt: Path,
    pilot_run_dir: Path,
    pilot_inputs: Mapping[str, JsonValue],
) -> dict[str, JsonValue]:
    """Raises `PreconditionFailed` on a blindness, seal or bindings violation; otherwise returns the record of what
    was checked, with the qualification of the pilot run."""
    pilot = mapping(run.manifest.get("pilot"), "manifest.pilot")
    analysis_commit = text(pilot.get("analysis_commit"), "manifest.pilot.analysis_commit")
    problems = blindness_problems(
        timestamps.read(receipt),
        vcs.commit_time(analysis_commit),
        json_object(json.loads((pilot_run_dir / TIMING_FILE).read_text(encoding=ENCODING)), TIMING_FILE),
    )
    if problems:
        raise PreconditionFailed(BLINDNESS, "; ".join(problems))
    mismatches = pilot_inputs.get("seal_mismatches")
    if isinstance(mismatches, list) and mismatches:
        raise PreconditionFailed(SEAL, f"files read from run-02 differ from its seal: {mismatches}")
    bindings = binding_problems(truth)
    if bindings:
        raise PreconditionFailed(BINDINGS, "; ".join(bindings))
    return {"blindness": "checked", "qualification": qualification(truth).to_record()}
