"""What a scan leaves: its statement `scan.json`, its printed headline and its finding folders (PREREG 10, 12.1)."""

from __future__ import annotations

import re
from dataclasses import dataclass
from typing import Final

from demos1.domain.jsonvalue import (
    JsonObject,
    JsonValue,
    as_list,
    as_object,
    child,
    integer,
    optional_boolean,
    optional_text,
    text,
)

NO_MODEL: Final = "no model used"
PROMISES_NOT_CHECKED: Final = "promises not checked"
FINDING_SENTENCE_START: Final = " passes on all "
SEPARATOR: Final = " · "
DEPTH_NAMES: Final = {"quick scan": "quick", "full scan": "full"}
NO_BUDGET: Final = "no budget"
BUDGET_SUFFIX: Final = " budget"
PARTIAL_LINE: Final = "partial: the budget stopped the scan before every mutant ran"
_FINDING_HEAD = re.compile(r"^#(\d+) (.+)$")
_SAMPLED = re.compile(r"^(\d+) of (\d+) green tests sampled$")
_WORKERS = re.compile(r"^(\d+) workers?\b")
_FOOTER = re.compile(r"^(\d+) mutants \((\d+) proven(.*?)\)" + re.escape(SEPARATOR))
_NOT_RUN = re.compile(r", (\d+) not run within the budget")
_DID_NOT_BUILD = re.compile(r", (\d+) did not build or start")
_SITE = re.compile(r"^(.+):(\d+)$")


@dataclass(frozen=True)
class StatementFacts:
    """The facts PREREG 10 reads from `scan.json`."""

    tests: int
    verifies_nothing: int
    caught: int
    unproven: int
    not_sampled: int
    not_green: int
    partial: bool
    seed: str
    mode: str
    mutants_ran: int
    mutants_proven: int

    @property
    def sampled_green(self) -> int:
        """The sampled green tests: the denominator of the three verdicts."""
        return self.verifies_nothing + self.caught + self.unproven

    @property
    def green(self) -> int:
        return self.sampled_green + self.not_sampled

    def to_json(self) -> JsonObject:
        return {
            "tests": self.tests,
            "verifies_nothing": self.verifies_nothing,
            "caught": self.caught,
            "unproven": self.unproven,
            "not_sampled": self.not_sampled,
            "not_green": self.not_green,
            "sampled_green": self.sampled_green,
            "partial": self.partial,
            "seed": self.seed,
            "mode": self.mode,
            "mutants_ran": self.mutants_ran,
            "mutants_proven": self.mutants_proven,
        }

    @classmethod
    def from_json(cls, document: JsonObject) -> StatementFacts:
        return cls(
            tests=integer(document, "tests"),
            verifies_nothing=integer(document, "verifies_nothing"),
            caught=integer(document, "caught"),
            unproven=integer(document, "unproven"),
            not_sampled=integer(document, "not_sampled"),
            not_green=integer(document, "not_green"),
            partial=document.get("partial") is True,
            seed=text(document, "seed"),
            mode=text(document, "mode"),
            mutants_ran=integer(document, "mutants_ran"),
            mutants_proven=integer(document, "mutants_proven"),
        )


def statement_facts(statement: JsonValue) -> StatementFacts:
    predicate = child(as_object(statement, "scan.json"), "predicate")
    counts = child(predicate, "counts")
    mutants = [as_object(mutant) for mutant in as_list(predicate.get("mutants") or [], "mutants")]
    proven = sum(1 for mutant in mutants if as_list(mutant.get("routes") or [], "routes"))
    return StatementFacts(
        tests=integer(counts, "tests"),
        verifies_nothing=integer(counts, "verifiesNothing"),
        caught=integer(counts, "caught"),
        unproven=integer(counts, "unproven"),
        not_sampled=integer(counts, "notSampled"),
        not_green=integer(counts, "notGreen"),
        partial=optional_boolean(predicate, "partial") is True,
        seed=text(predicate, "seed"),
        mode=text(predicate, "mode"),
        mutants_ran=len(mutants),
        mutants_proven=proven,
    )


@dataclass(frozen=True)
class HeadlineFinding:
    rank: int
    test: str
    sentence: str
    site_file: str | None
    site_line: int | None


@dataclass(frozen=True)
class Headline:
    """What the printed headline states that `scan.json` does not hold: depth, budget, workers and the draw."""

    depth: str | None
    budget: str | None
    workers: int | None
    sampled: int | None
    green: int | None
    partial: bool
    mutants_ran: int | None
    mutants_proven: int | None
    mutants_not_run: int | None
    mutants_did_not_build: int | None
    no_model: bool
    promises_not_checked: bool
    findings: tuple[HeadlineFinding, ...]

    @property
    def mutants_drawn(self) -> int | None:
        if self.mutants_ran is None:
            return None
        return self.mutants_ran + (self.mutants_not_run or 0) + (self.mutants_did_not_build or 0)

    def to_json(self) -> JsonObject:
        return {
            "depth": self.depth,
            "budget": self.budget,
            "workers": self.workers,
            "sampled": self.sampled,
            "green": self.green,
            "partial": self.partial,
            "mutants_drawn": self.mutants_drawn,
            "mutants_ran": self.mutants_ran,
            "mutants_proven": self.mutants_proven,
            "mutants_not_run": self.mutants_not_run,
            "mutants_did_not_build": self.mutants_did_not_build,
            "no_model": self.no_model,
            "promises_not_checked": self.promises_not_checked,
        }


def _depth_line(line: str) -> tuple[str | None, str | None, int | None, int | None, int | None]:
    parts = line.strip().split(SEPARATOR)
    depth = DEPTH_NAMES.get(parts[0])
    if depth is None:
        return None, None, None, None, None
    budget: str | None = None
    if len(parts) > 1:
        budget = parts[1] if parts[1] == NO_BUDGET else parts[1].removesuffix(BUDGET_SUFFIX)
    sampled = green = workers = None
    for part in parts[2:]:
        if match := _SAMPLED.match(part):
            sampled, green = int(match.group(1)), int(match.group(2))
        elif match := _WORKERS.match(part):
            workers = int(match.group(1))
    return depth, budget, workers, sampled, green


def _site(line: str) -> tuple[str | None, int | None]:
    location = line.strip().split("  ", 1)[0]
    match = _SITE.match(location)
    if match is None:
        return None, None
    return match.group(1), int(match.group(2))


def parse_headline(output: str) -> Headline:
    lines = output.splitlines()
    depth = budget = None
    workers = sampled = green = None
    partial = False
    findings: list[HeadlineFinding] = []
    ran = proven = not_run = did_not_build = None
    no_model = promises = False
    for index, line in enumerate(lines):
        stripped = line.strip()
        if depth is None and any(stripped.startswith(name + SEPARATOR) for name in DEPTH_NAMES):
            depth, budget, workers, sampled, green = _depth_line(stripped)
        elif stripped == PARTIAL_LINE:
            partial = True
        elif (match := _FINDING_HEAD.match(line)) and FINDING_SENTENCE_START in match.group(2):
            test, _, rest = match.group(2).partition(FINDING_SENTENCE_START)
            site_file, site_line = _site(lines[index + 1]) if index + 1 < len(lines) else (None, None)
            findings.append(
                HeadlineFinding(
                    int(match.group(1)), test, FINDING_SENTENCE_START.strip() + " " + rest, site_file, site_line
                )
            )
        elif match := _FOOTER.match(line):
            ran, proven = int(match.group(1)), int(match.group(2))
            inner = match.group(3)
            not_run = int(found.group(1)) if (found := _NOT_RUN.search(inner)) else 0
            did_not_build = int(found.group(1)) if (found := _DID_NOT_BUILD.search(inner)) else 0
            no_model = NO_MODEL in line
            promises = PROMISES_NOT_CHECKED in line
    return Headline(
        depth=depth,
        budget=budget,
        workers=workers,
        sampled=sampled,
        green=green,
        partial=partial,
        mutants_ran=ran,
        mutants_proven=proven,
        mutants_not_run=not_run,
        mutants_did_not_build=did_not_build,
        no_model=no_model,
        promises_not_checked=promises,
        findings=tuple(findings),
    )


@dataclass(frozen=True)
class FindingRoute:
    kind: str
    test: str | None
    operation: str | None
    witness: str | None

    def to_json(self) -> JsonObject:
        return {"kind": self.kind, "test": self.test, "operation": self.operation, "witness": self.witness}

    @classmethod
    def from_json(cls, document: JsonObject) -> FindingRoute:
        return cls(
            text(document, "kind"),
            optional_text(document, "test"),
            optional_text(document, "operation"),
            optional_text(document, "witness"),
        )


@dataclass(frozen=True)
class FindingFile:
    """The fields of a finding folder's `finding.json` the record keeps: the test and the route, never the patch."""

    test: str
    route: FindingRoute


def finding_file(document: JsonValue) -> FindingFile:
    record = as_object(document, "finding.json")
    route = child(record, "route")
    return FindingFile(
        test=text(record, "test"),
        route=FindingRoute(
            text(route, "kind"),
            optional_text(route, "test"),
            optional_text(route, "operation"),
            optional_text(route, "witness"),
        ),
    )


def file_of_test(test_id: str) -> str:
    """The file a scan test id names: the part before `::`."""
    return test_id.split("::", 1)[0]
