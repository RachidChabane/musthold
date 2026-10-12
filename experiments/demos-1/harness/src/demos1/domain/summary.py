"""PREREG 12.1 and 12.2: the committed summary, as `summary.json` and its rendering `SUMMARY.md`."""

from __future__ import annotations

from collections import Counter
from collections.abc import Sequence
from dataclasses import dataclass, field
from typing import Final

from demos1.domain.commands import SHOWN_ORDER
from demos1.domain.ecosystem import ECOSYSTEM_ORDER, Ecosystem
from demos1.domain.jsonvalue import JsonObject, JsonValue
from demos1.domain.lists import LIST_ORDER, AEntry, BEntry, ListName, Lists, Stratum
from demos1.domain.outcomes import PICK, ReplayResult, ScanOutcome
from demos1.domain.records import AttemptRecord, FindingRecord, ScanRecord
from demos1.domain.scan_output import StatementFacts

PICKS_PER_ECOSYSTEM: Final = 2
RELATION: Final = {
    "added": "file added by the pull request",
    "modified": "file modified by the pull request",
    "renamed": "file modified by the pull request",
}
NOT_CHANGED: Final = "file not changed by the pull request"
UNJUDGED_NOTE: Final = "unproven and unsampled tests are unjudged"
STRIPPED_RUN_FIELDS: Final = ("client_stderr",)
STRIPPED_SCAN_FIELDS: Final = ("stderr_tail",)


@dataclass
class Fractions:
    """Every fraction the summary prints, so that `report-lint` can check SUMMARY.md against them."""

    printed: set[str] = field(default_factory=set[str])

    def of(self, count: int, total: int) -> str:
        fraction = f"{count}/{total}"
        self.printed.add(fraction)
        return fraction

    def sorted(self) -> list[str]:
        return sorted(self.printed, key=lambda value: tuple(int(part) for part in value.split("/")))


@dataclass(frozen=True)
class SummaryInputs:
    selection_date: str | None
    manifest: JsonObject
    manifest_sha256: str
    lists: Lists
    lists_sha256: str
    attempts: Sequence[AttemptRecord]
    deviation_counts: dict[str, int]


@dataclass(frozen=True)
class Summary:
    document: JsonObject
    markdown: str


def attempts_in_list_order(lists: Lists, attempts: Sequence[AttemptRecord]) -> list[AttemptRecord]:
    by_id = {record.attempt: record for record in attempts}
    ordered: list[AttemptRecord] = []
    for name in LIST_ORDER:
        for entry in lists.entries(name):
            record = by_id.get(f"{name}-{entry.rank:02d}")
            if record is not None:
                ordered.append(record)
    return ordered


def shown_finding(record: AttemptRecord) -> tuple[ScanRecord, FindingRecord] | None:
    """The `#1` of the first scan, in the order of PREREG 12.1, that exited 0 and has at least one finding."""
    stratum = record.list_name.split("-", 1)[0]
    for name in SHOWN_ORDER[stratum]:
        for scan in record.scans:
            if scan.scan == name and scan.outcome == ScanOutcome.OK and scan.findings:
                return scan, min(scan.findings, key=lambda finding: finding.rank)
    return None


def relation(finding: FindingRecord) -> str:
    if finding.pull_request_status is None:
        return NOT_CHANGED
    return RELATION.get(finding.pull_request_status, NOT_CHANGED)


def _strip(record: AttemptRecord) -> JsonObject:
    """An attempt record without logs, mutant patches or third-party source."""
    document = dict(record.to_json())
    scans: list[JsonValue] = []
    for scan in record.scans:
        scan_document = {key: value for key, value in scan.to_json().items() if key not in STRIPPED_SCAN_FIELDS}
        scans.append(scan_document)
    document["scans"] = scans
    document["stages"] = [
        {key: value for key, value in stage.to_json().items() if key != "details"}
        | {"details": {key: value for key, value in stage.details.items() if not key.endswith("_output")}}
        for stage in record.stages
    ]
    return document


def _scan_digests(attempts: Sequence[AttemptRecord]) -> dict[str, JsonValue]:
    return {
        f"{record.attempt}/{scan.scan}": scan.scan_json_sha256
        for record in attempts
        for scan in record.scans
        if scan.scan_json_sha256 is not None
    }


class _Renderer:
    def __init__(self, inputs: SummaryInputs) -> None:
        self.inputs = inputs
        self.fractions = Fractions()
        self.lines: list[str] = []
        self.attempts = attempts_in_list_order(inputs.lists, inputs.attempts)

    def add(self, *lines: str) -> None:
        self.lines.extend(lines)

    def render(self) -> str:
        self.add("# Summary of the first scan demos (demos-1)", "")
        self._run()
        self._lists()
        self._attempts()
        for record in self.attempts:
            if record.outcome == PICK:
                self._pick(record)
        self._deviations()
        return "\n".join(self.lines).rstrip("\n") + "\n"

    def _run(self) -> None:
        manifest = self.inputs.manifest
        self.add(
            "## Run",
            "",
            f"- Selection date: `{self.inputs.selection_date or 'not recorded'}`",
            f"- Pre-registration commit: `{manifest.get('prereg_commit')}`",
            f"- Product commit: `{manifest.get('product_commit')}`",
            f"- Tooling commit: `{manifest.get('tooling_commit')}`",
            f"- `manifest.json` SHA-256: `{self.inputs.manifest_sha256}`",
            f"- `lists.json` SHA-256: `{self.inputs.lists_sha256}`",
            "",
        )

    def _lists(self) -> None:
        self.add("## Lists", "")
        for name in LIST_ORDER:
            entries = self.inputs.lists.entries(name)
            self.add(f"### `{name}`", "")
            if not entries:
                self.add("The list holds no entry.", "")
                continue
            if name.stratum is Stratum.B:
                self.add(
                    "| Rank | Repository | Pull request | Signals | Merged at |", "| --- | --- | --- | --- | --- |"
                )
            else:
                self.add("| Rank | Repository | Default branch | Commit |", "| --- | --- | --- | --- |")
            for entry in entries:
                self.add(_list_row(entry))
            self.add("")

    def _attempts(self) -> None:
        self.add("## Attempts", "")
        if not self.attempts:
            self.add("No attempt was made.", "")
            return
        self.add("| Attempt | Repository | Outcome |", "| --- | --- | --- |")
        for record in self.attempts:
            repository = record.identity.get("full_name")
            self.add(f"| `{record.attempt}` | `{repository}` | `{record.outcome or 'unfinished'}` |")
        self.add("")
        self._setup_failures()
        self._picks()

    def _setup_failures(self) -> None:
        self.add("### Setup failures", "")
        groups: list[tuple[str, list[AttemptRecord]]] = []
        for name in LIST_ORDER:
            groups.append((f"list `{name}`", [record for record in self.attempts if record.list_name == str(name)]))
        for ecosystem in ECOSYSTEM_ORDER:
            groups.append(
                (f"ecosystem `{ecosystem}`", [record for record in self.attempts if _ecosystem(record) is ecosystem])
            )
        rows = 0
        for label, records in groups:
            failures = Counter(record.outcome for record in records if record.outcome not in (None, PICK))
            for outcome, count in sorted(failures.items(), key=lambda item: str(item[0])):
                if rows == 0:
                    self.add("| Group | Outcome | Attempts |", "| --- | --- | --- |")
                rows += 1
                self.add(f"| {label} | `{outcome}` | {self.fractions.of(count, len(records))} attempts |")
        if rows == 0:
            self.add("No attempt ended in a setup failure.")
        self.add("")

    def _picks(self) -> None:
        self.add("### Picks", "")
        total = 0
        for ecosystem in ECOSYSTEM_ORDER:
            picks = sum(1 for record in self.attempts if record.outcome == PICK and _ecosystem(record) is ecosystem)
            total += picks
            self.add(f"- `{ecosystem}`: {self.fractions.of(picks, PICKS_PER_ECOSYSTEM)} picks")
        self.add(f"- In all: {self.fractions.of(total, PICKS_PER_ECOSYSTEM * len(ECOSYSTEM_ORDER))} picks", "")

    def _pick(self, record: AttemptRecord) -> None:
        identity = record.identity
        self.add(f"## Pick `{record.attempt}`: `{identity.get('full_name')}`", "")
        if identity.get("pull_request_url") is not None:
            signals = identity.get("signals")
            signal_words = ", ".join(f"`{value}`" for value in signals) if isinstance(signals, list) else ""
            self.add(
                f"- Pull request: `{identity.get('pull_request_url')}`, which carries the signals {signal_words}",
                f"- Checkout: merge commit `{identity.get('merge_commit')}`, base `{identity.get('base')}`",
            )
        else:
            self.add(f"- Checkout: `{identity.get('commit')}` of `{identity.get('default_branch')}`")
        self.add("")
        for scan in record.scans:
            self._scan(record, scan)
        shown = shown_finding(record)
        self.add("### Shown finding", "")
        if shown is None:
            self.add("No scan of this pick exited 0 with a finding: the pick is shown as its headlines alone.", "")
        else:
            scan, finding = shown
            self.add(
                f"`#{finding.rank}` of the scan `{scan.scan}`: `{finding.test}`, {_replay_words(finding)}.",
                "",
            )

    def _scan(self, record: AttemptRecord, scan: ScanRecord) -> None:
        self.add(f"### Scan `{scan.scan}`", "")
        self.add(f"- Outcome: `{scan.outcome or 'unfinished'}`")
        final = scan.command.final()
        if final is not None and final.wall_seconds is not None:
            self.add(f"- Wall time: `{final.wall_seconds:.1f} s`")
        if scan.headline_text:
            self.add("", "Headline, as the scan printed it:", "", "```text", scan.headline_text.rstrip("\n"), "```", "")
        if scan.statement is not None:
            self._counts(scan.statement)
        self._depth(scan)
        self._findings(record, scan)

    def _counts(self, facts: StatementFacts) -> None:
        n, g, total = facts.sampled_green, facts.green, facts.tests
        if facts.verifies_nothing == 0:
            self.add(f"- {self.fractions.of(0, n)} sampled tests verify nothing; {UNJUDGED_NOTE}")
        else:
            self.add(f"- {self.fractions.of(facts.verifies_nothing, n)} verify nothing")
        self.add(
            f"- {self.fractions.of(facts.caught, n)} caught ≥ 1 change",
            f"- {self.fractions.of(facts.unproven, n)} unproven",
            f"- {self.fractions.of(facts.not_sampled, g)} not sampled",
            f"- {self.fractions.of(facts.not_green, total)} tests not green",
            f"- Seed: `{facts.seed}`",
        )

    def _depth(self, scan: ScanRecord) -> None:
        headline = scan.headline or {}
        partial = scan.statement.partial if scan.statement is not None else headline.get("partial") is True
        self.add(
            f"- Depth: `{headline.get('depth')}`, budget: `{headline.get('budget')}`, workers: `{headline.get('workers')}`"
            + (", `partial`" if partial else "")
        )
        drawn, not_run = headline.get("mutants_drawn"), headline.get("mutants_not_run")
        if isinstance(drawn, int) and isinstance(not_run, int):
            self.add(f"- Mutants left unrun: {self.fractions.of(not_run, drawn)} drawn")
        self.add("")

    def _findings(self, record: AttemptRecord, scan: ScanRecord) -> None:
        if not scan.findings:
            return
        stratum_b = record.list_name.startswith("B-")
        self.add("| Finding | Test | Sentence | Site | Route | Replay |" + (" Pull request |" if stratum_b else ""))
        self.add("| --- | --- | --- | --- | --- | --- |" + (" --- |" if stratum_b else ""))
        for finding in sorted(scan.findings, key=lambda item: item.rank):
            site = f"`{finding.site_file}:{finding.site_line}`" if finding.site_file else "`unknown`"
            route = f"`{finding.route.kind}`" if finding.route is not None else "`unknown`"
            row = (
                f"| `#{finding.rank}` | `{finding.test}` | `{finding.sentence}` | {site} | {route} "
                f"| {finding.replay_result or 'not replayed'} |"
            )
            if stratum_b:
                row += f" {relation(finding)} |"
            self.add(row)
        self.add("")
        total = len(scan.findings)
        refuted = sum(1 for finding in scan.findings if finding.replay_result == ReplayResult.NO_LONGER_HOLDS)
        failed = sum(1 for finding in scan.findings if finding.replay_result == ReplayResult.FAILED)
        self.add(
            f"- {self.fractions.of(refuted, total)} findings no longer hold on replay",
            f"- {self.fractions.of(failed, total)} replays failed; a failed replay is not a refutation",
            "",
        )

    def _deviations(self) -> None:
        counts = self.inputs.deviation_counts
        total = sum(counts.values())
        self.add("## Deviations", "", "| Kind | Deviations |", "| --- | --- |")
        for kind, count in counts.items():
            self.add(f"| `{kind}` | {self.fractions.of(count, total)} |")
        self.add("")


def _list_row(entry: BEntry | AEntry) -> str:
    if isinstance(entry, BEntry):
        signals = ", ".join(f"`{signal}`" for signal in entry.signals)
        return f"| {entry.rank} | `{entry.full_name}` | `{entry.url}` | {signals} | `{entry.merged_at}` |"
    return f"| {entry.rank} | `{entry.full_name}` | `{entry.default_branch}` | `{entry.commit}` |"


def _ecosystem(record: AttemptRecord) -> Ecosystem:
    return ListName.parse(record.list_name).ecosystem


def _replay_words(finding: FindingRecord) -> str:
    return finding.replay_result or "not replayed"


def build_summary(inputs: SummaryInputs) -> Summary:
    renderer = _Renderer(inputs)
    markdown = renderer.render()
    document: JsonObject = {
        "selection_date": inputs.selection_date,
        "lists": inputs.lists.to_json(),
        "attempts": [_strip(record) for record in renderer.attempts],
        "digests": {
            "manifest.json": inputs.manifest_sha256,
            "lists.json": inputs.lists_sha256,
            "scan.json": _scan_digests(renderer.attempts),
        },
        "deviations": dict(inputs.deviation_counts),
        "fractions": list(renderer.fractions.sorted()),
    }
    return Summary(document, markdown)
