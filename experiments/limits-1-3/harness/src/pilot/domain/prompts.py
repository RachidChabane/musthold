"""Filling the pre-registered prompt templates by named-field substitution (PREREG 4.5)."""

from __future__ import annotations

from collections.abc import Mapping

SOURCE_FILE_HEADER = "=== {path} ==="
FILE_SEPARATOR = "\n\n"


def render_files(files: Mapping[str, str]) -> str:
    """Each file as a `=== path ===` header line followed by its content, in path order."""
    return FILE_SEPARATOR.join(
        f"{SOURCE_FILE_HEADER.format(path=path)}\n{files[path]}" for path in sorted(files)
    )


def generation_item(
    template: str, spec: str, target_ref: str, target_statement: str, source: Mapping[str, str], nonce: str
) -> str:
    return template.format(
        target_ref=target_ref,
        target_statement=target_statement,
        nonce=nonce,
        spec=spec,
        source=render_files(source),
    )


def validity_item(
    template: str, spec: str, original: str, mutant: str, diff: str, differential: str
) -> str:
    return template.format(
        spec=spec, original=original, mutant=mutant, diff=diff, differential=differential
    )


def alignment_item(template: str, criteria: str, test: str, signatures: str) -> str:
    return template.format(criteria=criteria, test=test, signatures=signatures)


def probe_item(template: str, test: str) -> str:
    return template.format(test=test)


def relabel_item(template: str, codebook: str, spec: str, source: Mapping[str, str], tests: str) -> str:
    return template.format(codebook=codebook, spec=spec, source=render_files(source), tests=tests)


def leak_audit_item(template: str, files: Mapping[str, str]) -> str:
    return template.format(files=render_files(files))
