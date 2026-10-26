#!/usr/bin/env python3
"""Play the key moments of the committed agent-loop recording in a terminal.

usage: scenes.py [--pause SECONDS] TRANSCRIPT

Reads the rendered transcript of demo/loop/recording and prints, in order, the
task, the gate's first verdict, the discriminating-power block with its
counterexamples, the test the agent added, the proof passing, the publication of
the proofs and the gate allowing the stop. Every line printed is a line of the
transcript, folded to the terminal width; nothing is written that the run did not
print. demo/loop/gif/loop.tape films it.
"""

from __future__ import annotations

import argparse
import re
import sys
import textwrap
import time
from dataclasses import dataclass, field
from pathlib import Path

WIDTH = 100
INDENT = "  "
CONTINUATION = "      "
DEFAULT_PAUSE = 2.5
HEADING = re.compile(r"^### (?P<clock>\d+:\d\d) · (?P<kind>.+)$")
FENCE = re.compile(r"^```(?P<lang>\w*)$")
TASK_HEADING = "## Task"

BOLD, DIM, RED, GREEN, CYAN, RESET = "\033[1m", "\033[2m", "\033[31m", "\033[32m", "\033[36m", "\033[0m"


@dataclass
class Section:
    clock: str
    kind: str
    blocks: list[tuple[str, list[str]]] = field(default_factory=list)

    def block(self, lang: str) -> list[str]:
        return next((lines for kind, lines in self.blocks if kind == lang), [])

    @property
    def command(self) -> str:
        console = self.block("console")
        return console[0].removeprefix("$ ") if console else ""

    @property
    def output(self) -> list[str]:
        return self.block("text")


def parse(text: str) -> tuple[str, list[Section]]:
    lines = text.splitlines()
    task, sections = "", []
    current: Section | None = None
    fence: tuple[str, list[str]] | None = None
    in_task = False
    for line in lines:
        if fence is not None:
            if line == "```":
                if in_task and not task:
                    task = " ".join(fence[1])
                elif current is not None:
                    current.blocks.append(fence)
                fence = None
            else:
                fence[1].append(line)
            continue
        if opened := FENCE.match(line):
            fence = (opened["lang"], [])
            continue
        if line == TASK_HEADING:
            in_task = True
        elif heading := HEADING.match(line):
            in_task = False
            current = Section(heading["clock"], heading["kind"])
            sections.append(current)
    return task, sections


def wrapped(line: str) -> list[str]:
    """The line folded to the width, its continuation lines indented."""
    lead = line[: len(line) - len(line.lstrip())]
    return textwrap.wrap(line, WIDTH, initial_indent="", subsequent_indent=lead + CONTINUATION, drop_whitespace=True) or [""]


def added(diff: list[str]) -> list[str]:
    """The lines a diff adds, without blank ones and those it removes as they are."""
    removed = {line[1:] for line in diff if line.startswith("-")}
    return [line for line in diff if line.startswith("+") and line[1:].strip() and line[1:] not in removed]


def first(sections: list[Section], start: int, test) -> int:
    return next(index for index in range(start, len(sections)) if test(sections[index]))


def highlighted(lines: list[str], word: str, color: str) -> list[str]:
    """The lines, those holding the word in the color."""
    return [f"{color}{line}{RESET}" if word in line else line for line in lines]


def ran(section: Section) -> str:
    """The title of a section that ran a command: its time and the command."""
    return f"{DIM}{section.clock}{RESET} $ {section.command}"


def blocks(section: Section) -> bool:
    return section.command.startswith("musthold-dp prove") and any("BLOCK" in line for line in section.output)


def proves_clean(section: Section) -> bool:
    return section.command.startswith("musthold-dp prove") and bool(section.output) and not blocks(section)


def gate_fails(section: Section) -> bool:
    return section.command.startswith("shallnot gate") and any(line.startswith("shallnot: FAIL") for line in section.output)


def scenes(task: str, sections: list[Section]) -> list[tuple[str, list[str]]]:
    gate = first(sections, 0, gate_fails)
    block = first(sections, gate, blocks)
    edit = first(sections, block, lambda s: s.kind == "Edit")
    passed = first(sections, edit, proves_clean)
    publish = first(sections, passed, lambda s: s.command.startswith("musthold-verify"))
    allowed = first(sections, publish, lambda s: s.kind.startswith("Gate: allowed"))
    verdict = [line for line in sections[gate].output if line.startswith(("shallnot: FAIL", "requirements:", "findings:"))]
    blocking = [line for line in sections[block].output if line.startswith(("BOX-", "  survivor"))]
    edited = sections[edit].block("text")[0].rsplit("/project/", 1)[-1]
    additions = [f"{GREEN}{line}{RESET}" for line in added(sections[edit].block("diff"))]
    return [
        (f"{BOLD}Task{RESET}", [task]),
        (ran(sections[gate]), highlighted(verdict, "FAIL", RED)),
        (ran(sections[block]), highlighted(blocking, "BLOCK", RED)),
        (f"{DIM}{sections[edit].clock}{RESET} agent edits {edited}", additions),
        (ran(sections[passed]), highlighted(sections[passed].output, "PASS", GREEN)),
        (ran(sections[publish]), sections[publish].output),
        (f"{DIM}{sections[allowed].clock}{RESET} {GREEN}{BOLD}gate: allowed{RESET}", []),
    ]


def play(scenes_to_play: list[tuple[str, list[str]]], pause: float) -> None:
    for title, lines in scenes_to_play:
        print(f"{CYAN}▶{RESET} {title}")
        for line in lines:
            for part in wrapped(line):
                print(INDENT + part)
        print()
        sys.stdout.flush()
        time.sleep(pause)


def main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--pause", type=float, default=DEFAULT_PAUSE)
    parser.add_argument("transcript", type=Path)
    args = parser.parse_args(argv)
    task, sections = parse(args.transcript.read_text(encoding="utf-8"))
    play(scenes(task, sections), args.pause)
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
