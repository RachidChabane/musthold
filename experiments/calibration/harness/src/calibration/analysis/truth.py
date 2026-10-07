"""The truth and judgment files of the pilot (§1): only `analyze` and `derive` read them.

Aggregate files whose container shape the pilot's RUN_RECORDS leaves open are read in both the list-of-records and
the keyed-mapping form, as the pilot's own reader does.
"""

from __future__ import annotations

import json
import re
from collections import defaultdict
from collections.abc import Mapping
from dataclasses import dataclass
from pathlib import Path
from typing import Final

import yaml

from calibration.analysis.votes import NONE_ANSWER, JudgeVote, ValidityVote
from calibration.domain.constants import ENCODING
from calibration.domain.jsonread import (
    RecordError,
    as_list,
    as_mapping,
    integer,
    mapping,
    optional_text,
    sequence,
    text,
)
from calibration.domain.pilot_records import (
    DIFF_NO_DIFF_FOUND,
    ProcessingRow,
    criterion_number,
    parse_bindings,
    parse_differential,
    parse_jsonl,
    parse_processing,
)

LABELS_FILE: Final = "labels.yaml"
ARM_P_LABELS_FILE: Final = "arm_p/labels.yaml"
ORACLE_FILE: Final = "oracle_classes.json"
RELABEL_FILE: Final = "relabel.jsonl"
KILLS_FILE: Final = "kills.jsonl"
MUTANT_CLASSES_FILE: Final = "mutant_classes.json"
ALIGNMENT_FILE: Final = "alignment.jsonl"
VALIDITY_FILE: Final = "validity.jsonl"
BINDINGS_FILE: Final = "bindings.json"
PROCESSING_FILE: Final = "processing.jsonl"
DIFFERENTIAL_FILE: Final = "differential.jsonl"
MUTANTS_DIR: Final = "mutants"
TIMING_FILE: Final = "timing.json"
JUDGE_HOST: Final = "J-H"
ARM_MASKED: Final = "NAME-MASKED"
VALIDITY_JUDGES: Final = ("V-H1", "V-H2")
CALL_OK: Final = "ok"
RATER_OPENAI: Final = "R-OAI"
RATER_AUTHOR: Final = "R-AUTHOR"
KILLED: Final = "killed"
LABEL_CLASS: Final = re.compile(r"^([A-Z_]+)")
CLASS_GOOD: Final = "GOOD"
CLASS_GOOD_BROAD: Final = "GOOD_BROAD"
CLASS_WEAK: Final = "WEAK"
CLASS_VACUOUS: Final = "VACUOUS"
CLASS_MIS_OTHER: Final = "MIS_OTHER"
CLASS_OFFSPEC: Final = "OFFSPEC"
TRUTH_ALIGNED: Final = "ALIGNED"
ARM_P_SOURCE: Final = re.compile(r"^[A-Za-z]+-(\d+)")


@dataclass(frozen=True)
class TestLabel:
    __test__ = False

    nodeid: str
    cites: str
    label_class: str
    checks: tuple[str, ...]
    alignment_truth: str

    def k1_accepted(self, lenient: bool) -> frozenset[str]:
        """The answers to the criterion question that are correct (§3.3 table)."""
        if self.label_class in (CLASS_VACUOUS, CLASS_OFFSPEC):
            return frozenset({NONE_ANSWER, self.cites})
        accepted = {self.checks[0]}
        if lenient and self.label_class == CLASS_GOOD_BROAD and len(self.checks) > 1:
            accepted.add(self.checks[1])
        return frozenset(accepted)

    @property
    def k2_positive(self) -> bool | None:
        """ADEQUATE for GOOD and GOOD_BROAD, INADEQUATE for WEAK, VACUOUS and OFFSPEC; MIS_OTHER is no K2 item."""
        if self.label_class in (CLASS_GOOD, CLASS_GOOD_BROAD):
            return True
        if self.label_class in (CLASS_WEAK, CLASS_VACUOUS, CLASS_OFFSPEC):
            return False
        return None

    @property
    def k3_positive(self) -> bool:
        return self.alignment_truth == TRUTH_ALIGNED


def label_class_of(label: str) -> str:
    match = LABEL_CLASS.match(label.strip())
    if match is None:
        raise RecordError(f"unrecognised label {label!r}")
    return match.group(1)


def parse_labels(content: str) -> list[TestLabel]:
    labels: list[TestLabel] = []
    for index, raw in enumerate(sequence(yaml.safe_load(content), LABELS_FILE)):
        entry = mapping(raw, f"{LABELS_FILE}[{index}]")
        checks = entry.get("checks")
        refs = as_list(checks)
        labels.append(
            TestLabel(
                nodeid=text(entry.get("nodeid"), "label.nodeid"),
                cites=text(entry.get("cites"), "label.cites"),
                label_class=label_class_of(text(entry.get("label"), "label.label")),
                checks=tuple(text(ref, "label.checks") for ref in refs) if refs is not None else (),
                alignment_truth=text(entry.get("alignment_truth"), "label.alignment_truth"),
            )
        )
    return labels


def parse_arm_p_sources(content: str) -> dict[str, int]:
    """The source criterion n of `PARK-n_*` per planted mutant id (arm P clusters)."""
    sources: dict[str, int] = {}
    for mutant_id, raw in mapping(yaml.safe_load(content), ARM_P_LABELS_FILE).items():
        source = text(mapping(raw, mutant_id).get("source"), f"{mutant_id}.source")
        match = ARM_P_SOURCE.match(source)
        if match is None:
            raise RecordError(f"{mutant_id}: no criterion number in {source!r}")
        sources[mutant_id] = int(match.group(1))
    return sources


def parse_oracle(content: str) -> dict[str, frozenset[int]]:
    """`oracle_classes.json`: `{mutants: {id: {failed_criteria: [n]}}}`."""
    document = mapping(json.loads(content), ORACLE_FILE)
    return {
        mutant_id: frozenset(
            integer(number, "failed criterion")
            for number in sequence(mapping(raw, mutant_id).get("failed_criteria"), "failed_criteria")
        )
        for mutant_id, raw in mapping(document.get("mutants"), "oracle.mutants").items()
    }


def parse_kills(content: str) -> dict[str, dict[str, str]]:
    return {
        text(row.get("mutant_id"), "kills.mutant_id"): {
            nodeid: text(result, "kill result") for nodeid, result in mapping(row.get("results"), "results").items()
        }
        for row in parse_jsonl(content, KILLS_FILE)
    }


def parse_relabels(content: str) -> dict[str, dict[str, str]]:
    """`{nodeid: {rater: label class}}`."""
    raters: dict[str, dict[str, str]] = defaultdict(dict)
    for row in parse_jsonl(content, RELABEL_FILE):
        raters[text(row.get("nodeid"), "relabel.nodeid")][text(row.get("rater"), "relabel.rater")] = label_class_of(
            text(row.get("label"), "relabel.label")
        )
    return dict(raters)


def consensus_nodeids(labels: list[TestLabel], relabels: Mapping[str, Mapping[str, str]]) -> frozenset[str]:
    """The tests on which the author, R-OAI and R-AUTHOR agree on the six-class label (§3.3)."""
    agreed: set[str] = set()
    for label in labels:
        raters = relabels.get(label.nodeid, {})
        if (
            RATER_OPENAI in raters
            and RATER_AUTHOR in raters
            and raters[RATER_OPENAI] == raters[RATER_AUTHOR] == label.label_class
        ):
            agreed.add(label.nodeid)
    return frozenset(agreed)


def parse_alignment_votes(content: str) -> dict[str, dict[int, JudgeVote]]:
    """`{nodeid: {repetition: vote}}` of the answered J-H NAME-MASKED repetitions."""
    votes: dict[str, dict[int, JudgeVote]] = defaultdict(dict)
    for row in parse_jsonl(content, ALIGNMENT_FILE):
        if row.get("judge") != JUDGE_HOST or row.get("arm") != ARM_MASKED or row.get("status") != CALL_OK:
            continue
        confidence = row.get("confidence")
        votes[text(row.get("nodeid"), "alignment.nodeid")][int(str(row.get("rep")))] = JudgeVote(
            ref=text(row.get("verifies_ref"), "alignment.verifies_ref"),
            coverage=text(row.get("coverage"), "alignment.coverage"),
            confidence=float(confidence) if isinstance(confidence, int | float) else 0.0,
        )
    return dict(votes)


def parse_validity_votes(content: str) -> dict[str, dict[str, dict[str, ValidityVote]]]:
    """`{mutant_id: {judge: {criterion ref: vote}}}` of the answered V-H1 and V-H2 records."""
    votes: dict[str, dict[str, dict[str, ValidityVote]]] = defaultdict(lambda: defaultdict(dict))
    for row in parse_jsonl(content, VALIDITY_FILE):
        judge = row.get("judge")
        if judge not in VALIDITY_JUDGES or row.get("status") != CALL_OK:
            continue
        payload = as_mapping(row.get("payload"))
        requirements = as_mapping(payload.get("requirements")) if payload is not None else None
        if requirements is None:
            continue
        mutant_id = text(row.get("mutant_id"), "validity.mutant_id")
        for ref, raw in requirements.items():
            entry = mapping(raw, f"{mutant_id}.{ref}")
            confidence = entry.get("confidence")
            votes[mutant_id][str(judge)][ref] = ValidityVote(
                text(entry.get("verdict"), "validity.verdict"),
                float(confidence) if isinstance(confidence, int | float) else 0.0,
            )
    return {mutant: {judge: dict(refs) for judge, refs in judges.items()} for mutant, judges in votes.items()}


@dataclass(frozen=True)
class MutantMeta:
    mutant_id: str
    arm: str
    target: str | None
    generator_call_id: str | None


def parse_mutant_meta(content: str) -> MutantMeta:
    body = mapping(json.loads(content), "mutant document")
    generator = as_mapping(body.get("generator"))
    return MutantMeta(
        mutant_id=text(body.get("mutant_id"), "mutant.mutant_id"),
        arm=text(body.get("arm"), "mutant.arm"),
        target=optional_text(body.get("target"), "mutant.target"),
        generator_call_id=optional_text(generator.get("call_id"), "generator.call_id") if generator else None,
    )


@dataclass(frozen=True)
class MatrixInputs:
    """Arm-T VALID mutants and UNATTRIBUTED mutants of the pilot, as `pilot.domain.matrix` pools (§9)."""

    valid_pool: tuple[tuple[int, frozenset[str]], ...]
    unattributed: tuple[frozenset[str], ...]


def parse_matrix_inputs(
    classes_content: str, kills: Mapping[str, Mapping[str, str]], evaluated: frozenset[str]
) -> MatrixInputs:
    """The pilot's `matrix_pools`: arm-T evaluated mutants, a VALID mutant by its primary criterion with the tests
    that have a reproduced kill, an UNATTRIBUTED one by its killers."""
    document = json.loads(classes_content)
    listed = as_list(document)
    records: list[object] = (
        listed
        if listed is not None
        else [
            {"mutant_id": key, **mapping(value, key)} for key, value in mapping(document, MUTANT_CLASSES_FILE).items()
        ]
    )
    valid: list[tuple[int, frozenset[str]]] = []
    unattributed: list[frozenset[str]] = []
    for raw in sorted(records, key=lambda record: str(mapping(record, "class").get("mutant_id"))):
        record = mapping(raw, "mutant class")
        mutant_id = text(record.get("mutant_id"), "class.mutant_id")
        if record.get("arm") != "T" or mutant_id not in evaluated:
            continue
        killed = frozenset(nodeid for nodeid, result in kills.get(mutant_id, {}).items() if result == KILLED)
        primary = as_mapping(record.get("primary")) or {}
        criterion = optional_text(primary.get("criterion"), "primary.criterion") if primary.get("included") else None
        if criterion is not None:
            valid.append((criterion_number(criterion), killed))
        elif record.get("unattributed") is True:
            unattributed.append(killed)
    return MatrixInputs(tuple(valid), tuple(unattributed))


@dataclass(frozen=True)
class PilotTruth:
    """Everything of the pilot that only the analysis may read."""

    labels: tuple[TestLabel, ...]
    consensus: frozenset[str]
    arm_p_sources: Mapping[str, int]
    bindings: Mapping[str, str]
    processing: tuple[ProcessingRow, ...]
    differential: Mapping[str, Mapping[str, object]]
    mutants: Mapping[str, MutantMeta]
    oracle: Mapping[str, frozenset[int]]
    kills: Mapping[str, Mapping[str, str]]
    alignment_votes: Mapping[str, Mapping[int, JudgeVote]]
    validity_votes: Mapping[str, Mapping[str, Mapping[str, ValidityVote]]]
    matrix: MatrixInputs

    def f3_status(self, mutant_id: str) -> str:
        record = self.differential.get(mutant_id)
        return str(record.get("status")) if record is not None else DIFF_NO_DIFF_FOUND

    def bound_tests(self, ref: str) -> tuple[str, ...]:
        return tuple(sorted(nodeid for nodeid, bound in self.bindings.items() if bound == ref))

    def reproduced_kill(self, mutant_id: str, ref: str) -> bool:
        """Whether a test bound to the criterion has a reproduced kill of the mutant; a flaky result counts as a
        survival and an error is not a kill."""
        results = self.kills.get(mutant_id, {})
        return any(results.get(nodeid) == KILLED for nodeid in self.bound_tests(ref))


def read_pilot_truth(prereg_dir: Path, run_dir: Path) -> PilotTruth:
    """Reads the truth and judgment files from the snapshot's `prereg/` and from run-02."""
    labels = parse_labels((prereg_dir / LABELS_FILE).read_text(encoding=ENCODING))
    relabels = parse_relabels((run_dir / RELABEL_FILE).read_text(encoding=ENCODING))
    processing = parse_processing((run_dir / PROCESSING_FILE).read_text(encoding=ENCODING))
    kills = parse_kills((run_dir / KILLS_FILE).read_text(encoding=ENCODING))
    metas = {
        path.stem: parse_mutant_meta(path.read_text(encoding=ENCODING))
        for path in sorted((run_dir / MUTANTS_DIR).iterdir())
        if path.suffix == ".json"
    }
    evaluated = frozenset(row.mutant_id for row in processing if row.evaluated)
    return PilotTruth(
        labels=tuple(labels),
        consensus=consensus_nodeids(labels, relabels),
        arm_p_sources=parse_arm_p_sources((prereg_dir / ARM_P_LABELS_FILE).read_text(encoding=ENCODING)),
        bindings=parse_bindings((run_dir / BINDINGS_FILE).read_text(encoding=ENCODING)),
        processing=tuple(processing),
        differential=parse_differential((run_dir / DIFFERENTIAL_FILE).read_text(encoding=ENCODING)),
        mutants=metas,
        oracle=parse_oracle((run_dir / ORACLE_FILE).read_text(encoding=ENCODING)),
        kills=kills,
        alignment_votes=parse_alignment_votes((run_dir / ALIGNMENT_FILE).read_text(encoding=ENCODING)),
        validity_votes=parse_validity_votes((run_dir / VALIDITY_FILE).read_text(encoding=ENCODING)),
        matrix=parse_matrix_inputs((run_dir / MUTANT_CLASSES_FILE).read_text(encoding=ENCODING), kills, evaluated),
    )
