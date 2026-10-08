from __future__ import annotations

import posixpath

from hypothesis import given, settings
from hypothesis import strategies as st

from specbench.domain.agent_events import ActionKind, AgentAction, EventPointer
from specbench.domain.contamination import FindingKind, ScanPolicy, scan_action
from specbench.domain.shell_words import simple_commands

ROOT = "/work/space"
POLICY = ScanPolicy(ROOT)
POINTER = EventPointer(0)
NETWORK_FINDINGS = {FindingKind.NETWORK_COMMAND, FindingKind.URL, FindingKind.NETWORK_TOOL}

Text = st.SearchStrategy[str]


def template(pattern: str, *parts: Text) -> Text:
    """Strategy of `pattern` with each `{}` filled from the matching part."""

    def fill(values: tuple[str, ...]) -> str:
        return pattern.format(*values)

    return st.tuples(*parts).map(fill)


def flat(*strategies: Text) -> Text:
    return st.one_of(*strategies)


segment: Text = st.from_regex(r"[a-z][a-z0-9_]{0,6}", fullmatch=True)
host = template("{}.{}", segment, st.sampled_from(["org", "com", "io", "dev"]))
remote_url = template("{}://{}/{}", st.sampled_from(["http", "https"]), host, segment)
relative_path: Text = st.lists(segment, min_size=1, max_size=4).map("/".join)
relative_file = template("{}.{}", relative_path, st.sampled_from(["py", "go", "txt", "md", "json"]))
fetch_verb = st.sampled_from(["fetch", "pull", "push"])

network_commands = flat(
    template("curl {} {}", st.sampled_from(["", "-s", "-sSL", "-fsSL -o out"]), remote_url),
    template("wget {}", remote_url),
    template("git clone {} {}", remote_url, relative_path),
    template("git {} {}", fetch_verb, segment),
    template("git -C {} {}", relative_path, fetch_verb),
    template("pip install {}", segment),
    template("python3 -m pip install {}", segment),
    template("uv pip install {}", segment),
    template("npm {} {}", st.sampled_from(["install", "i", "add"]), segment),
    template("go install {}/{}@{}", host, segment, st.sampled_from(["latest", "v1.2.3"])),
    template("go get {}/{}", host, segment),
    template("cargo install {}", segment),
    template("apt-get install {}", segment),
    template("ssh {} ls", host),
)
WRAPPERS = (
    "{}",
    "sudo {}",
    "env A=1 {}",
    "nice -n 10 {}",
    "time {}",
    "timeout 30 {}",
    "cd src && {}",
    "ls; {}",
    "true || {}",
    "echo start | {}",
    "bash -c '{}'",
    'sh -lc "{}"',
    "(cd x && {})",
)
network_lines = flat(*(template(wrapper, network_commands) for wrapper in WRAPPERS))

safe_commands = flat(
    template("pytest -q {}", relative_file),
    template("cat {}", relative_file),
    template("cp {} {}", relative_file, relative_file),
    template("diff -u {} {} > build/out.txt", relative_file, relative_file),
    template("go test ./{}/...", relative_path),
    template("ls -la {}", relative_path),
    template("python -m pytest {} -x", relative_file),
    template("git diff -- {}", relative_file),
    template("sed -n '1,20p' {}", relative_file),
    template("grep -rn needle {} | head", relative_file),
)


@settings(max_examples=300)
@given(network_lines)
def test_no_false_negative_on_known_network_commands(line: str) -> None:
    findings = scan_action(AgentAction(POINTER, ActionKind.COMMAND, "Bash", command=line), POLICY)
    assert any(f.kind in NETWORK_FINDINGS for f in findings), line


@settings(max_examples=300)
@given(safe_commands)
def test_no_finding_on_in_workspace_relative_paths(line: str) -> None:
    assert scan_action(AgentAction(POINTER, ActionKind.COMMAND, "Bash", command=line), POLICY) == [], line


@given(st.lists(relative_file, min_size=1, max_size=5))
def test_file_tools_on_relative_paths_are_clean(paths: list[str]) -> None:
    action = AgentAction(POINTER, ActionKind.FILE_ACCESS, "Edit", paths=tuple(paths))
    assert scan_action(action, POLICY) == []


@given(st.lists(relative_file, min_size=1, max_size=5))
def test_file_tools_on_absolute_paths_inside_the_root_are_clean(paths: list[str]) -> None:
    action = AgentAction(POINTER, ActionKind.FILE_ACCESS, "Write", paths=tuple(posixpath.join(ROOT, p) for p in paths))
    assert scan_action(action, POLICY) == []


@given(st.lists(segment, min_size=1, max_size=4).map("/".join), relative_file)
def test_a_path_outside_the_root_is_always_found(outside: str, name: str) -> None:
    for path in (f"/{outside}/{name}", f"../{name}", f"{ROOT}/../{outside}/{name}"):
        action = AgentAction(POINTER, ActionKind.FILE_ACCESS, "Read", paths=(path,))
        findings = scan_action(action, ScanPolicy(ROOT, allowed_prefixes=()))
        assert [f.kind for f in findings] == [FindingKind.OUTSIDE_PATH], path


@settings(max_examples=300)
@given(st.text(max_size=80))
def test_the_scanner_never_raises_on_arbitrary_command_text(text: str) -> None:
    scan_action(AgentAction(POINTER, ActionKind.COMMAND, "Bash", command=text), POLICY)
    simple_commands(text)
