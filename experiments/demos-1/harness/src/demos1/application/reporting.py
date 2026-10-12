"""The `summarize` and `report-lint` verbs of PREREG 12.2 and 12.3."""

from __future__ import annotations

from dataclasses import dataclass
from pathlib import PurePosixPath
from typing import Final

from demos1.application.ports import Files, Repository
from demos1.application.select_run import Refused
from demos1.domain.deviations import counts_by_kind
from demos1.domain.jsonvalue import as_list, as_object, encode, optional_text
from demos1.domain.layout import ATTEMPTS, WorkLayout
from demos1.domain.lists import Lists
from demos1.domain.records import AttemptRecord
from demos1.domain.report_lint import Violation, lint
from demos1.domain.summary import SummaryInputs, build_summary

PRIVATE: Final = "PRIVATE"
HISTORY_REPORT: Final = "notes/public-history-report.md"
NOTES_PATH_FILTER: Final = "--invert-paths --path notes/"
SUMMARY_DIRECTORY: Final = "notes/demos-1"
SUMMARY_JSON: Final = "summary.json"
SUMMARY_MARKDOWN: Final = "SUMMARY.md"


@dataclass(frozen=True)
class SummaryPaths:
    repo_root: PurePosixPath

    @property
    def directory(self) -> PurePosixPath:
        return self.repo_root / SUMMARY_DIRECTORY

    @property
    def document(self) -> PurePosixPath:
        return self.directory / SUMMARY_JSON

    @property
    def markdown(self) -> PurePosixPath:
        return self.directory / SUMMARY_MARKDOWN


class SummarizeVerb:
    def __init__(self, files: Files, repository: Repository) -> None:
        self._files = files
        self._repository = repository

    def run(self, work: WorkLayout, paths: SummaryPaths) -> None:
        self._refuse_unless_private(paths.repo_root)
        manifest = self._files.read_json(work.manifest)
        lists = self._files.read_json(work.lists)
        if manifest is None or lists is None:
            raise Refused("select has not run: the manifest or the lists are missing")
        attempts_dir = work.root / ATTEMPTS
        records: list[AttemptRecord] = []
        for name in sorted(self._files.list_names(attempts_dir)):
            document = self._files.read_json(work.attempt(name).record)
            if isinstance(document, dict):
                records.append(AttemptRecord.from_json(as_object(document)))
        manifest_object = as_object(manifest)
        summary = build_summary(
            SummaryInputs(
                selection_date=optional_text(manifest_object, "selection_date"),
                manifest=manifest_object,
                manifest_sha256=self._files.sha256(work.manifest) or "",
                lists=Lists.from_json(as_object(lists)),
                lists_sha256=self._files.sha256(work.lists) or "",
                attempts=records,
                deviation_counts=counts_by_kind(self._files.read_text(work.deviations) or ""),
            )
        )
        self._files.ensure_directory(paths.directory)
        self._files.write_text(paths.document, encode(summary.document))
        self._files.write_text(paths.markdown, summary.markdown)

    def _refuse_unless_private(self, repo_root: PurePosixPath) -> None:
        """PREREG 12.2: the origin is private and the public export leaves `notes/` out of every revision."""
        visibility = self._repository.origin_visibility()
        if visibility != PRIVATE:
            raise Refused(f"the origin's visibility is {visibility!r}, not {PRIVATE}")
        report = self._files.read_text(repo_root / HISTORY_REPORT) or ""
        if NOTES_PATH_FILTER not in report:
            raise Refused(f"{HISTORY_REPORT} does not record the path filter {NOTES_PATH_FILTER!r}")


def report_lint(files: Files, paths: SummaryPaths) -> list[Violation]:
    markdown = files.read_text(paths.markdown)
    document = files.read_json(paths.document)
    if markdown is None or document is None:
        raise Refused(f"{paths.markdown} and {paths.document} must both exist: run summarize")
    fractions = [str(value) for value in as_list(as_object(document).get("fractions") or [], "fractions")]
    return lint(markdown, fractions)
