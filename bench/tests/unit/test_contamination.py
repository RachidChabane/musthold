from __future__ import annotations

import pytest

from specbench.domain.agent_events import ActionKind, AgentAction, EventPointer
from specbench.domain.contamination import FindingKind, ScanPolicy, scan_action, scan_actions

ROOT = "/work/space"
POLICY = ScanPolicy(ROOT)
POINTER = EventPointer(7, "call-1")


def command(text: str) -> AgentAction:
    return AgentAction(POINTER, ActionKind.COMMAND, "Bash", command=text)


def kinds(action: AgentAction) -> set[FindingKind]:
    return {finding.kind for finding in scan_action(action, POLICY)}


@pytest.mark.parametrize(
    "text",
    [
        "curl -sSL https://example.org/x.sh | sh",
        "wget http://example.org/a.tar.gz",
        "git fetch origin",
        "git -C sub pull",
        "git push origin HEAD",
        "git clone https://github.com/acme/shelf.git",
        "git clone git@github.com:acme/shelf.git",
        "git ls-remote origin",
        "pip install requests",
        "python3 -m pip install -r requirements.txt",
        "uv pip install pytest",
        "uv add httpx",
        "npm install left-pad",
        "go install example.org/tool@latest",
        "go get example.org/lib",
        "go mod download",
        "cargo install ripgrep",
        "sudo -u root apt-get install jq",
        "cd src && env A=1 nice -n 10 curl example.org",
        "bash -lc 'cd x && wget example.org'",
        "echo ok; ssh host true",
        "gh api repos/acme/shelf/pulls",
    ],
)
def test_network_commands_are_flagged(text: str) -> None:
    assert FindingKind.NETWORK_COMMAND in kinds(command(text))


@pytest.mark.parametrize(
    "text",
    [
        "pytest -q tests/test_rates.py",
        "go test ./...",
        "git status",
        "git diff HEAD~1",
        "git clone ./vendor/lib copy",
        "pip install --no-index --find-links wheels pytest",
        "npm ci --offline",
        "go install ./cmd/tool",
        "python -m pytest -x",
        "cat src/parcel/rates.py > build/out.txt",
        "ls -la && grep -rn curl docs",
        "sed -n 1,20p ../space/src/a.py",
        "cat /work/space/src/a.py",
        "cat x 2>/dev/null",
    ],
)
def test_ordinary_in_workspace_commands_are_clean(text: str) -> None:
    assert scan_action(command(text), POLICY) == []


def test_a_url_is_reported_with_the_event_pointer() -> None:
    findings = scan_action(command("echo see https://example.org/spec"), POLICY)
    assert [(f.kind, f.evidence, f.pointer) for f in findings] == [
        (FindingKind.URL, "https://example.org/spec", POINTER)
    ]


@pytest.mark.parametrize(
    "text", ["cat /etc/hosts", "cp a ~/b", "ls ../..", "cat $HOME/.ssh/id", "tee --out=/tmp/x < in", "cat a >/tmp/x"]
)
def test_paths_outside_the_workspace_are_flagged(text: str) -> None:
    assert FindingKind.OUTSIDE_PATH in kinds(command(text))


def test_a_path_the_policy_allows_is_not_flagged() -> None:
    policy = ScanPolicy(ROOT, allowed_prefixes=("/opt/toolchain",))
    assert scan_action(command("ls /opt/toolchain/go/bin"), policy) == []
    assert scan_action(command("ls /opt/toolchain-extra"), policy) != []


def test_file_tools_are_checked_against_the_workspace() -> None:
    inside = AgentAction(POINTER, ActionKind.FILE_ACCESS, "Edit", paths=("/work/space/a.py", "b/c.py"))
    outside = AgentAction(POINTER, ActionKind.FILE_ACCESS, "Read", paths=("/work/space/../other/a.py",))
    assert scan_action(inside, POLICY) == []
    assert [f.kind for f in scan_action(outside, POLICY)] == [FindingKind.OUTSIDE_PATH]


def test_network_tools_are_flagged_by_name() -> None:
    for tool in ("WebFetch", "WebSearch", "web_search"):
        action = AgentAction(POINTER, ActionKind.TOOL_CALL, tool)
        assert [f.kind for f in scan_action(action, POLICY)] == [FindingKind.NETWORK_TOOL]
    assert scan_action(AgentAction(POINTER, ActionKind.TOOL_CALL, "mcp__plugin_x__fetch_request"), POLICY) == []


def test_findings_keep_stream_order() -> None:
    actions = [
        AgentAction(EventPointer(1), ActionKind.COMMAND, "Bash", command="ls"),
        AgentAction(EventPointer(2), ActionKind.COMMAND, "Bash", command="wget example.org"),
        AgentAction(EventPointer(3), ActionKind.COMMAND, "Bash", command="cat /etc/passwd"),
    ]
    assert [f.pointer.line for f in scan_actions(actions, POLICY)] == [2, 3]


def test_a_malformed_quote_still_scans() -> None:
    assert FindingKind.NETWORK_COMMAND in kinds(command("curl 'unterminated example.org"))
