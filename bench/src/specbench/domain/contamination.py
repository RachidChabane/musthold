"""A deterministic scan of an agent's actions for network use, URLs and paths outside its workspace (ADR 0004, isolation).

A finding voids the trunk as `contaminated`. The scanner errs towards flagging: a command that may reach the network
is reported, and the report names the event so a reviewer can judge it.
"""

from __future__ import annotations

import posixpath
import re
from collections.abc import Iterable, Mapping
from dataclasses import dataclass
from enum import Enum
from typing import Final

from specbench.domain.agent_events import ActionKind, AgentAction, EventPointer
from specbench.domain.shell_words import SimpleCommand, simple_commands

NETWORK_PROGRAMS: Final = frozenset(
    {
        "curl", "wget", "aria2c", "http", "https", "ssh", "scp", "sftp", "ftp", "telnet", "nc", "ncat", "netcat",
        "socat", "nmap", "ping", "traceroute", "dig", "nslookup", "gh", "npx", "uvx", "pipx",
    }
)  # fmt: skip
NETWORK_TOOL_NAMES: Final = frozenset({"webfetch", "websearch", "web_fetch", "web_search"})
OFFLINE_FLAGS: Final = frozenset({"--offline", "--no-index"})
PIP_PROGRAMS: Final = frozenset({"pip", "pip3"})
PYTHON_PROGRAM: Final = re.compile(r"^python[0-9.]*$")
PACKAGE_COMMANDS: Final[Mapping[str, frozenset[str]]] = {
    "pip": frozenset({"install", "download"}),
    "pip3": frozenset({"install", "download"}),
    "poetry": frozenset({"install", "add", "update"}),
    "uv": frozenset({"add", "sync", "lock"}),
    "npm": frozenset({"install", "i", "add", "ci", "update", "exec"}),
    "yarn": frozenset({"add", "install", "up"}),
    "pnpm": frozenset({"add", "install", "i", "update"}),
    "bun": frozenset({"add", "install"}),
    "go": frozenset({"get"}),
    "cargo": frozenset({"install", "fetch", "add", "update"}),
    "gem": frozenset({"install"}),
    "bundle": frozenset({"install", "update"}),
    "apt": frozenset({"install", "update", "upgrade"}),
    "apt-get": frozenset({"install", "update", "upgrade"}),
    "apk": frozenset({"add", "update"}),
    "yum": frozenset({"install", "update"}),
    "dnf": frozenset({"install", "update"}),
    "brew": frozenset({"install", "update", "upgrade"}),
    "conda": frozenset({"install", "create"}),
    "mamba": frozenset({"install", "create"}),
    "docker": frozenset({"pull", "push", "login"}),
}
GIT_NETWORK_SUBCOMMANDS: Final = frozenset({"fetch", "pull", "push", "clone", "ls-remote"})
GIT_OPTIONS_WITH_VALUE: Final = frozenset({"-C", "-c", "--git-dir", "--work-tree", "--namespace", "--exec-path"})
GIT_LOCAL_SOURCE_PREFIXES: Final = ("./", "../", "/", "file://")
GO_VERSIONED_INSTALL_MARKER: Final = "@"
URL_PATTERN: Final = re.compile(
    r"""\b(?:https?|ftps?|wss?|ssh|git)://[^\s'"<>`)\]]+|\b[\w.-]+@[\w-]+(?:\.[\w-]+)+:[\w./~-]+"""
)
HOME_PREFIXES: Final = ("~", "$HOME", "${HOME}")
PARENT_SEGMENT: Final = ".."
DEFAULT_ALLOWED_PREFIXES: Final = (
    "/bin",
    "/sbin",
    "/usr/bin",
    "/usr/sbin",
    "/usr/local/bin",
    "/opt/homebrew/bin",
    "/dev/null",
    "/dev/stdin",
    "/dev/stdout",
    "/dev/stderr",
)


class FindingKind(Enum):
    NETWORK_COMMAND = "network-command"
    NETWORK_TOOL = "network-tool"
    URL = "url"
    OUTSIDE_PATH = "outside-path"


@dataclass(frozen=True)
class Finding:
    kind: FindingKind
    pointer: EventPointer
    evidence: str


@dataclass(frozen=True)
class ScanPolicy:
    """The workspace root the agent works in, and the absolute prefixes it may touch anyway (system binaries, toolchains)."""

    workspace_root: str
    allowed_prefixes: tuple[str, ...] = DEFAULT_ALLOWED_PREFIXES


def _is_within(path: str, root: str) -> bool:
    return path == root or path.startswith(root.rstrip("/") + "/")


def _outside_workspace(path: str, policy: ScanPolicy) -> bool:
    if path.startswith(HOME_PREFIXES):
        return True
    root = posixpath.normpath(policy.workspace_root)
    resolved = posixpath.normpath(path if path.startswith("/") else posixpath.join(root, path))
    if _is_within(resolved, root):
        return False
    return not any(_is_within(resolved, posixpath.normpath(prefix)) for prefix in policy.allowed_prefixes)


def _path_candidate(word: str) -> str | None:
    """The path a command word names, or `None` when the word does not look like one."""
    value = word.split("=", 1)[1] if word.startswith("-") and "=" in word else word
    if not value or "://" in value or any(char.isspace() for char in value):
        return None
    is_path = value.startswith(("/", *HOME_PREFIXES)) or PARENT_SEGMENT in value.split("/")
    return value if is_path else None


def _git_is_networked(args: tuple[str, ...]) -> bool:
    index = 0
    while index < len(args) and args[index].startswith("-"):
        index += 2 if args[index] in GIT_OPTIONS_WITH_VALUE else 1
    if index >= len(args):
        return False
    subcommand, rest = args[index], [a for a in args[index + 1 :] if not a.startswith("-")]
    if subcommand == "remote":
        return bool(rest) and rest[0] == "update"
    if subcommand == "submodule":
        return bool(rest) and rest[0] in {"update", "init", "sync"}
    if subcommand not in GIT_NETWORK_SUBCOMMANDS:
        return False
    return not (rest and rest[0].startswith(GIT_LOCAL_SOURCE_PREFIXES))


def _subcommand_in(args: tuple[str, ...], subcommands: frozenset[str]) -> bool:
    positionals = [arg for arg in args if not arg.startswith("-")]
    return bool(positionals) and positionals[0] in subcommands


def _go_is_networked(args: tuple[str, ...]) -> bool:
    positionals = [arg for arg in args if not arg.startswith("-")]
    if not positionals:
        return False
    head, rest = positionals[0], positionals[1:]
    if head == "install":
        return any(GO_VERSIONED_INSTALL_MARKER in arg for arg in rest)
    if head == "mod":
        return bool(rest) and rest[0] == "download"
    return head in PACKAGE_COMMANDS["go"]


def _package_manager_is_networked(command: SimpleCommand) -> bool:
    if OFFLINE_FLAGS & set(command.args):
        return False
    program = command.program
    if program == "go":
        return _go_is_networked(command.args)
    if PYTHON_PROGRAM.match(program):
        return _python_module_is_networked(command.args)
    if program == "uv" and _subcommand_in(command.args, frozenset({"pip", "tool"})):
        return any(arg in {"install", "sync", "compile"} for arg in command.args)
    return _subcommand_in(command.args, PACKAGE_COMMANDS.get(program, frozenset()))


def _python_module_is_networked(args: tuple[str, ...]) -> bool:
    if len(args) >= 3 and args[0] == "-m" and args[1] == "pip":
        return args[2] in PACKAGE_COMMANDS["pip"]
    return False


def _command_is_networked(command: SimpleCommand) -> bool:
    if command.program in NETWORK_PROGRAMS:
        return True
    if command.program == "git":
        return _git_is_networked(command.args)
    return _package_manager_is_networked(command)


def _scan_command(action: AgentAction, policy: ScanPolicy) -> list[Finding]:
    findings: list[Finding] = []
    pointer = action.pointer
    for url in URL_PATTERN.findall(action.command):
        findings.append(Finding(FindingKind.URL, pointer, url))
    commands = simple_commands(action.command)
    for command in commands:
        if _command_is_networked(command):
            findings.append(Finding(FindingKind.NETWORK_COMMAND, pointer, " ".join(command.words)))
        for word in command.words[1:]:
            candidate = _path_candidate(word)
            if candidate is not None and _outside_workspace(candidate, policy):
                findings.append(Finding(FindingKind.OUTSIDE_PATH, pointer, candidate))
        program_path = _path_candidate(command.program_word)
        if program_path is not None and _outside_workspace(program_path, policy):
            findings.append(Finding(FindingKind.OUTSIDE_PATH, pointer, program_path))
    return findings


def _scan_paths(action: AgentAction, policy: ScanPolicy) -> list[Finding]:
    return [
        Finding(FindingKind.OUTSIDE_PATH, action.pointer, path)
        for path in action.paths
        if _outside_workspace(path, policy)
    ]


def _scan_tool_call(action: AgentAction) -> list[Finding]:
    if action.tool.lower() in NETWORK_TOOL_NAMES:
        return [Finding(FindingKind.NETWORK_TOOL, action.pointer, action.tool)]
    return []


def scan_action(action: AgentAction, policy: ScanPolicy) -> list[Finding]:
    match action.kind:
        case ActionKind.COMMAND:
            return _scan_command(action, policy) + _scan_paths(action, policy)
        case ActionKind.FILE_ACCESS:
            return _scan_paths(action, policy)
        case ActionKind.TOOL_CALL:
            return _scan_tool_call(action) + _scan_paths(action, policy)


def scan_actions(actions: Iterable[AgentAction], policy: ScanPolicy) -> tuple[Finding, ...]:
    """Every finding over the actions, in stream order. An empty result means the trunk is clean."""
    return tuple(finding for action in actions for finding in scan_action(action, policy))
