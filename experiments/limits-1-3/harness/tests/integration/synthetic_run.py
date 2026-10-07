"""Builds a SYNTHETIC run directory (harness/RUN_RECORDS.md) over the real 38 node ids.

Everything here is generated: fake mutants, kill vectors, judgments, oracle results. It
exercises the analysis code only and says nothing about any model.
"""

from __future__ import annotations

import hashlib
import json
import shutil
from collections.abc import Mapping, Sequence
from pathlib import Path
from typing import Any

import yaml

from pilot.adapters.prereg_files import load_labels
from pilot.domain.alignment_aggregate import AlignmentAggregate, AlignmentVerdict
from pilot.domain.execution_validation import valid_x_criteria
from pilot.domain.mutant_classes import CriterionClass, MutantClass
from pilot.domain.outcomes import KillResult
from pilot.domain.labels import TestClass, TestLabel
from pilot.domain.refs import criterion_ref

EXPERIMENT_ROOT = Path(__file__).resolve().parents[3]
PREREG_FILES = ("labels.yaml", "label_matrix.csv", "models.lock.json", "REPORT.tmpl.md", "verify_log.txt", "isolation_table.json")
RUN_ID = "synthetic"
CRITERIA = tuple(range(1, 11))
MUTANTS_PER_CRITERION = 4
SLOTS = ("S1", "S2", "S3")
ALIGNMENT_REPS = 5
PROBE_REPS = 3
TIMING_SECONDS = 3600
JUDGES_FOR_ALIGNMENT = (("J-H", "NAME-MASKED"), ("J-H", "NAME-SHOWN"), ("J-X", "NAME-MASKED"))
PLANTED_LETTERS = ("a", "b")
CLAUDE_COST = 0.05


def mutant_id(label: str) -> str:
    return hashlib.sha256(label.encode()).hexdigest()[:16]


def dump(path: Path, document: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(document, sort_keys=True, indent=1), encoding="utf-8")


def dump_lines(path: Path, rows: list[dict[str, Any]]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text("".join(json.dumps(r, sort_keys=True) + "\n" for r in rows), encoding="utf-8")


def killers_for(labels: list[TestLabel], criterion: int, variant: int) -> list[str]:
    """Tests that kill the `variant`-th synthetic mutant of `criterion`, following the labels."""
    found = []
    for label in labels:
        if label.test_class in (TestClass.GOOD, TestClass.GOOD_BROAD) and criterion in _checks(label):
            found.append(label.nodeid)
        elif label.test_class is TestClass.WEAK and label.cites == criterion and variant == 0:
            found.append(label.nodeid)
        elif label.test_class is TestClass.MIS_OTHER and label.other == criterion:
            found.append(label.nodeid)
    return found


def _checks(label: TestLabel) -> set[int]:
    checks = {label.cites}
    if label.other is not None:
        checks.add(label.other)
    return checks


def noisy_verdict(label: TestLabel, flip: bool) -> str:
    truthful = "ALIGNED" if label.test_class.is_gold_good else "MISALIGNED"
    if not flip:
        return truthful
    return "MISALIGNED" if truthful == "ALIGNED" else "ALIGNED"


def class_record(mid: str, arm: str, target: int | None, valid: bool, undetermined: bool = False,
                 unattributed: bool = False, killed: Sequence[str] = (), bindings: Mapping[str, str] | None = None) -> dict[str, Any]:
    """A mutant class written by the pipeline's own serializer (`MutantClass.to_mapping`)."""
    per = {criterion_ref(c): CriterionClass.NOT_VIOLATING for c in CRITERIA}
    confidence: dict[str, float] = {}
    if target is not None and valid:
        per[criterion_ref(target)] = CriterionClass.VALID_W
        confidence[criterion_ref(target)] = 0.9
    elif target is not None and undetermined:
        per[criterion_ref(target)] = CriterionClass.UNDETERMINED
    target_ref = criterion_ref(target) if target else None
    included = bool(target and valid)
    results = {nodeid: KillResult.KILLED if nodeid in killed else KillResult.SURVIVED for nodeid in (bindings or {})}
    valid_x = valid_x_criteria([target_ref] if included and target_ref else [], True, results, bindings or {})
    return MutantClass(
        mutant_id=mid, arm=arm, target=target_ref, per_criterion=per, valid_x=valid_x, valid_confidence=confidence, equivalent=False,
        unattributed=unattributed, off_target=False, collateral=False, primary_included=included,
        primary_criterion=target_ref if included else None, discarded=False, survived_all=False,
    ).to_mapping()


def payload_for(target: int | None, violated: bool) -> dict[str, Any]:
    requirements = {}
    for c in CRITERIA:
        is_target_violated = violated and c == target
        requirements[criterion_ref(c)] = {
            "verdict": "VIOLATED" if is_target_violated else "NOT_VIOLATED",
            "confidence": 0.9,
            "witness": {"function": "quote", "entry": None, "exit": None, "lost_ticket": None,
                        "validated": None, "amount": None, "expected": {"kind": "RETURNS", "value": "0"}}
            if is_target_violated else None,
        }
    return {"equivalent": {"value": False, "confidence": 0.9}, "requirements": requirements}


def build_root(root: Path, *, list_alignment: bool = False, flaky_judge: bool = False) -> Path:
    """Copy the pre-registration files into `root` and write the synthetic run; returns the run dir."""
    prereg = root / "prereg"
    prereg.mkdir(parents=True)
    for name in PREREG_FILES:
        shutil.copy(EXPERIMENT_ROOT / "prereg" / name, prereg / name)
    labels = load_labels(prereg / "labels.yaml")
    run = root / "runs" / RUN_ID
    run.mkdir(parents=True)
    builder = _RunBuilder(root, run, labels, list_alignment, flaky_judge)
    builder.write_all()
    return run


class _RunBuilder:
    def __init__(self, root: Path, run: Path, labels: list[TestLabel], list_form: bool, flaky: bool) -> None:
        self.root, self.run, self.labels, self.list_form, self.flaky = root, run, labels, list_form, flaky
        self.mutants: list[dict[str, Any]] = []
        self.processing: list[dict[str, Any]] = []
        self.kills: list[dict[str, Any]] = []
        self.classes: list[dict[str, Any]] = []
        self.differential: list[dict[str, Any]] = []
        self.validity: list[dict[str, Any]] = []
        self.witness: list[dict[str, Any]] = []
        self.oracle: dict[str, Any] = {}
        self.calls: dict[str, dict[str, Any]] = {}
        self.planted_labels: dict[str, Any] = {}
        self.nodeids = [label.nodeid for label in labels]
        self.bindings = {label.nodeid: criterion_ref(label.cites) for label in labels}

    def write_all(self) -> None:
        self._arm_t()
        self._arm_p()
        self._arm_o()
        self._stillborn()
        self._blind_spot()
        dump_lines(self.run / "processing.jsonl", sorted(self.processing, key=lambda r: r["mutant_id"]))
        dump_lines(self.run / "kills.jsonl", sorted(self.kills, key=lambda r: r["mutant_id"]))
        dump_lines(self.run / "differential.jsonl", sorted(self.differential, key=lambda r: r["mutant_id"]))
        dump_lines(self.run / "validity.jsonl", self.validity)
        dump_lines(self.run / "witness.jsonl", self.witness)
        dump(self.run / "mutant_classes.json", self.classes)
        dump(self.run / "oracle_classes.json", {"oracle_digest": "synthetic", "mutants": self.oracle})
        (self.root / "prereg" / "arm_p").mkdir()
        (self.root / "prereg" / "arm_p" / "labels.yaml").write_text(yaml.safe_dump(self.planted_labels), encoding="utf-8")
        self._reference_bindings_traceability()
        self._alignment()
        self._probe_relabel()
        self._raw_and_meta()

    # ---- mutants
    def _add_mutant(self, mid: str, arm: str, target: int | None, slot: str | None, call_id: str | None,
                    killed: list[str], failed_oracle: list[int], status: str = "OK", diff: str = "DIFFERS") -> None:
        generator = {"model": "claude-opus-5-5", "cli": "claude", "call_id": call_id, "nonce": "n"} if call_id else None
        self.mutants.append({"mutant_id": mid, "arm": arm, "target": criterion_ref(target) if target else None,
                             "slot": slot, "generator": generator, "edits": [], "normalized_edits": [],
                             "mechanism_summary": None, "merged_from": []})
        dump(self.run / "mutants" / f"{mid}.json", self.mutants[-1])
        self.processing.append({"mutant_id": mid, "stillborn": False, "stillborn_reason": None, "status": status,
                                "duplicate_of": None, "changed_lines": 1})
        if status == "OK":
            results = {n: ("killed" if n in killed else "survived") for n in self.nodeids}
            self.kills.append({"mutant_id": mid, "results": results})
            self.differential.append({"mutant_id": mid, "status": diff, "function": "quote", "args": None,
                                      "original_out": None, "mutant_out": None, "reason": None})
        self.oracle[mid] = {"arm": arm, "failed_criteria": failed_oracle, "source": "executed", "criterion_failed": {}}

    def _arm_t(self) -> None:
        for criterion in CRITERIA:
            for index in range(MUTANTS_PER_CRITERION):
                mid = mutant_id(f"T-{criterion}-{index}")
                call_id = f"gen-{criterion}-{index % 2}"
                killed = killers_for(self.labels, criterion, index)
                wrong = index == MUTANTS_PER_CRITERION - 1
                undetermined = criterion == 2 and index == 1
                self._add_mutant(mid, "T", criterion, SLOTS[index % 3], call_id, killed,
                                 [] if wrong else [criterion], diff="NO_DIFF_FOUND" if wrong else "DIFFERS")
                self.classes.append(class_record(mid, "T", criterion, valid=not wrong and not undetermined,
                                                 undetermined=undetermined, killed=killed, bindings=self.bindings))
                self._validity_rows(mid, criterion, not wrong and not undetermined)
                self.calls[call_id] = _call(cost=CLAUDE_COST)
        unattributed = mutant_id("T-unattributed")
        self._add_mutant(unattributed, "T", 1, "S3", "gen-1-0", [self.nodeids[0]], [])
        self.classes.append(class_record(unattributed, "T", 1, valid=False, unattributed=True, bindings=self.bindings))
        self._validity_rows(unattributed, 1, False)
        duplicate = mutant_id("T-duplicate")
        self.mutants.append({"mutant_id": duplicate, "arm": "T", "target": criterion_ref(1), "slot": "S1",
                             "generator": None, "edits": [], "normalized_edits": [], "mechanism_summary": None,
                             "merged_from": []})
        dump(self.run / "mutants" / f"{duplicate}.json", self.mutants[-1])
        self.processing.append({"mutant_id": duplicate, "stillborn": False, "stillborn_reason": None,
                                "status": "DUPLICATE", "duplicate_of": mutant_id("T-1-0"), "changed_lines": 1})
        self.oracle[duplicate] = {"arm": "T", "failed_criteria": [1], "source": f"inherited:{mutant_id('T-1-0')}"}

    def _validity_rows(self, mid: str, target: int | None, violated: bool) -> None:
        for judge in ("V-H1", "V-H2"):
            cid = f"val-{judge}-{mid}"
            self.validity.append({"mutant_id": mid, "judge": judge, "call_id": cid, "status": "ok",
                                  "payload": payload_for(target, violated)})
            self.calls[cid] = _call(cost=CLAUDE_COST if judge == "V-H1" else None)
            if violated and target is not None:
                self.witness.append({"mutant_id": mid, "judge": judge, "criterion": criterion_ref(target),
                                     "confirmed": True, "reference_out": "0", "mutant_out": "1", "error": None})

    def _arm_p(self) -> None:
        for criterion in CRITERIA:
            for letter in PLANTED_LETTERS:
                mid = mutant_id(f"P-{criterion}-{letter}")
                valid = not (criterion == 10 and letter == "b")
                self._add_mutant(mid, "P", None, None, None, killers_for(self.labels, criterion, 1), [criterion])
                self.classes.append(class_record(mid, "P", criterion, valid=valid,
                                                 killed=killers_for(self.labels, criterion, 1), bindings=self.bindings))
                self._validity_rows(mid, criterion, valid)
                self.planted_labels[mid] = {"source": f"PARK-{criterion}_{letter}", "declared_target": criterion_ref(criterion), "kind": "r"}
            neutral = mutant_id(f"P-{criterion}-n1")
            self._add_mutant(neutral, "P", None, None, None, [], [], diff="NO_DIFF_FOUND")
            self.classes.append(class_record(neutral, "P", criterion, valid=False, bindings=self.bindings))
            self._validity_rows(neutral, criterion, False)
            self.planted_labels[neutral] = {"source": f"PARK-{criterion}_n1", "declared_target": criterion_ref(criterion), "kind": "n1"}

    def _arm_o(self) -> None:
        for index in range(6):
            mid = mutant_id(f"O-{index}")
            self._add_mutant(mid, "O", None, None, None, [], [index % 3 + 1] if index < 2 else [], diff="NO_DIFF_FOUND")

    def _stillborn(self) -> None:
        mid = mutant_id("T-stillborn")
        self.mutants.append({"mutant_id": mid, "arm": "T", "target": criterion_ref(1), "slot": "S1", "generator": None,
                             "edits": [], "normalized_edits": [], "mechanism_summary": None, "merged_from": []})
        dump(self.run / "mutants" / f"{mid}.json", self.mutants[-1])
        self.processing.append({"mutant_id": mid, "stillborn": True, "stillborn_reason": "unparseable",
                                "status": "OK", "duplicate_of": None, "changed_lines": 0})

    # ---- blind spot
    def _blind_spot(self) -> None:
        sources: dict[str, list[str]] = {"S": [], "O": [], "op": [], "terra": [], "luna": []}
        for criterion in CRITERIA:
            for index in range(3):
                for arm, key, label in (("X", "O", "X"), ("O", "op", "OP")):
                    mid = mutant_id(f"{label}-{criterion}-{index}")
                    self._add_mutant(mid, arm, None, None, None, [], [criterion], diff="NO_DIFF_FOUND")
                    doc = {**json.loads((self.run / "mutants" / f"{mid}.json").read_text()),
                           "edits": [{"file": "src/parkfee/tariff.py", "qualname": f"definition_{(criterion + index) % 4}", "new_source": "x"}]}
                    dump(self.run / "mutants" / f"{mid}.json", doc)
                    sources[key].append(mid)
        sources["S"] = [m["mutant_id"] for m in self.mutants if m["arm"] == "T" and m["mutant_id"] != mutant_id("T-stillborn")
                        and m["mutant_id"] != mutant_id("T-duplicate")]
        dump(self.run / "mutant_sources.json", {k: sorted(v) for k, v in sources.items()})
        for writer in ("S", "O"):
            suites = {criterion_ref(c): {"writer": writer, "criterion": criterion_ref(c), "call_id": f"write_tests__{criterion_ref(c)}__{writer}",
                                         "status": "ok", "reference_checked": True,
                                         "tests": [{"name": n, "source": "def x(): pass", "status": "kept", "reason": None} for n in ("test_a", "test_b")]}
                      for c in CRITERIA}
            dump(self.run / "suites" / f"{writer}.json", suites)
        evaluated = [r["mutant_id"] for r in self.processing if r["status"] == "OK" and not r["stillborn"]]
        rows = []
        for writer in ("S", "O"):
            for mid in sorted(evaluated):
                draw = int(hashlib.sha256(f"{writer}{mid}".encode()).hexdigest(), 16) % 5
                results = {f"tests/test_park_{c}.py::{n}": "killed" if draw >= 2 and n == "test_a" else ("error" if draw == 0 and n == "test_b" else "survived")
                           for c in CRITERIA for n in ("test_a", "test_b")}
                rows.append({"writer": writer, "mutant_id": mid, "results": results})
        dump_lines(self.run / "writer_kills.jsonl", rows)
        dump(self.run / "exploratory_status.json", {"luna": {f"write_tests__{criterion_ref(c)}__L": "blocked" for c in CRITERIA}})

    # ---- run-level records
    def _reference_bindings_traceability(self) -> None:
        dump(self.run / "reference.json", {"runs": [{n: "passed" for n in self.nodeids}] * 3, "passed_all": True})
        dump(self.run / "bindings.json", {l.nodeid: criterion_ref(l.cites) for l in self.labels})
        dump(self.run / "traceability.json", {
            "verdict": "pass",
            "summary": {"requirements": {"in_focus": 10, "covered": 10}, "tests": {"total": 38, "bound": 38}},
            "findings": [],
            "requirements": [{"ref": criterion_ref(c), "tests": [{"name": l.nodeid} for l in self.labels if l.cites == c]}
                             for c in CRITERIA],
        })

    def _alignment(self) -> None:
        calls, aggregate = [], []
        for judge, arm in JUDGES_FOR_ALIGNMENT:
            for index, label in enumerate(self.labels):
                flip = index % 5 == 0 if judge == "J-H" else index % 3 == 0
                verdict = noisy_verdict(label, flip)
                undetermined = self.flaky and judge == "J-H" and arm == "NAME-MASKED" and index == 1
                for rep in range(ALIGNMENT_REPS):
                    cid = f"al-{judge}-{arm}-{index}-{rep}"
                    ref = criterion_ref(label.cites) if verdict == "ALIGNED" else "NONE"
                    calls.append({"judge": judge, "arm": arm, "nodeid": label.nodeid, "rep": rep, "call_id": cid,
                                  "status": "ok", "verifies_ref": ref, "coverage": "FULL" if ref != "NONE" else "NONE",
                                  "confidence": 80, "rationale": "synthetic"})
                    self.calls[cid] = _call(cost=CLAUDE_COST if judge == "J-H" else None)
                final = AlignmentVerdict.UNDETERMINED if undetermined else AlignmentVerdict(verdict)
                row = AlignmentAggregate(
                    modal_ref=criterion_ref(label.cites) if verdict == "ALIGNED" else "NONE", modal_share=1.0,
                    modal_coverage="FULL" if verdict == "ALIGNED" else "NONE", mean_confidence=80.0,
                    verdict=final, verdict_s1=final, missing_reps=0,
                ).to_mapping()
                aggregate.append({"judge": judge, "arm": arm, "nodeid": label.nodeid, **row})
        dump_lines(self.run / "alignment.jsonl", calls)
        if self.list_form:
            dump(self.run / "alignment_aggregate.json", aggregate)
            return
        nested: dict[str, Any] = {}
        for row in aggregate:
            fields = {k: v for k, v in row.items() if k not in ("judge", "arm", "nodeid")}
            nested.setdefault(row["judge"], {}).setdefault(row["arm"], {})[row["nodeid"]] = fields
        dump(self.run / "alignment_aggregate.json", nested)

    def _probe_relabel(self) -> None:
        probes, relabels = [], []
        for judge in ("J-H", "J-X"):
            for index, label in enumerate(self.labels):
                for rep in range(PROBE_REPS):
                    guess = "TARGETED" if (index + rep) % 2 == 0 else "OTHER"
                    probes.append({"judge": judge, "nodeid": label.nodeid, "rep": rep, "call_id": f"pr-{judge}-{index}-{rep}",
                                   "status": "ok", "guess": guess, "confidence": 60})
        for rater in ("R-OAI", "R-AUTHOR"):
            for index, label in enumerate(self.labels):
                shown = "WEAK" if rater == "R-OAI" and index % 7 == 0 and label.test_class is TestClass.VACUOUS else label.test_class.value
                relabels.append({"rater": rater, "nodeid": label.nodeid, "label": shown, "checks": []})
        dump_lines(self.run / "probe.jsonl", probes)
        dump_lines(self.run / "relabel.jsonl", relabels)

    def _raw_and_meta(self) -> None:
        for call_id, record in self.calls.items():
            dump(self.run / "raw" / call_id / "result.json", record)
        dump(self.run / "timing.json", {"run-core": {"started": "t0", "finished": "t1", "seconds": TIMING_SECONDS}})
        dump(self.run / "manifest.json", {"run_id": RUN_ID, "prereg_commit": "synthetic", "harness_digest": "h",
                                           "fixture_digest": "f", "cli_versions": {"claude": "x", "codex": "y"},
                                           "models": ["claude-opus-5-5", "claude-sonnet-5-5", "gpt-5.6-terra"]})
        (self.run / "DEVIATIONS.md").write_text("", encoding="utf-8")
        lock = json.loads((self.root / "prereg" / "models.lock.json").read_text(encoding="utf-8"))
        for model in lock["models"]:
            dump(self.run / "canary" / f"{model['model']}__{model['cli']}__alignment.json",
                 {"input_tokens": 1, "num_turns": 1, "cli_version": "x"})
        dump(self.root / "results" / "runner_crosscheck.json", {"differences": []})


def _call(cost: float | None) -> dict[str, Any]:
    return {"status": "ok", "payload": {}, "input_tokens": 100, "output_tokens": 20, "total_cost_usd": cost,
            "num_turns": 1, "attempt": 1, "provenance": "reported"}
