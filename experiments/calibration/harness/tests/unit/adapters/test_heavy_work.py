from __future__ import annotations

import json

from calibration.adapters.heavy_work import SystemHeavyWorkProbe
from calibration.adapters.process_tree import PS_ARGV
from calibration.domain.ports import HttpReply, TransportFailure
from support.fakes import ScriptedRunner, ScriptedTransport, ok

OLLAMA_PS = "http://127.0.0.1:11434/api/ps"
OWNER = 5000

# pid ppid pgid command: launchd > zsh > mem-guard > nice/harness (owner) > kev.serve server; plus bystanders.
OWN_TREE = """\
    1     0     1 /sbin/launchd
  100     1   100 -zsh
  200   100   200 /bin/bash script/mem-guard run -- uv run calibration run --model M4
  201   200   200 /bin/bash script/mem-guard run -- uv run calibration run --model M4
 5000   200   200 /usr/bin/python3 -m calibration run --model M4
 5001  5000   200 uv run --frozen --extra serve python -m kev.serve --run a/b@c --host 127.0.0.1 --port 8008
 5002  5001   200 /usr/bin/python3 -m kev.serve --run a/b@c --host 127.0.0.1 --port 8008
  300     1   300 /Applications/Editor.app/Contents/MacOS/Editor
"""


def probe(ps_text: str, installed: HttpReply | TransportFailure) -> SystemHeavyWorkProbe:
    runner = ScriptedRunner({PS_ARGV: ok(ps_text)})
    return SystemHeavyWorkProbe(runner, ScriptedTransport({OLLAMA_PS: installed}), owner_pid=OWNER)


def ps_reply(models: list[str]) -> HttpReply:
    return HttpReply(200, json.dumps({"models": [{"name": name} for name in models]}).encode())


def test_the_blocks_own_tree_and_its_guard_ancestor_are_clear() -> None:
    assert probe(OWN_TREE, ps_reply([])).check().clear


def test_a_foreign_ollama_runner_is_flagged() -> None:
    text = OWN_TREE + " 7000     1  7000 /usr/local/bin/ollama runner --model /x/blob --port 5555\n"
    report = probe(text, ps_reply([])).check()
    assert not report.clear
    assert len(report.foreign_processes) == 1
    assert "ollama runner" in report.foreign_processes[0]
    assert "pid 7000" in report.foreign_processes[0]


def test_each_heavy_pattern_is_flagged() -> None:
    commands = ["kev.serve --port 1", "llama-server -m x", "python -m mlx_lm.server"]
    text = OWN_TREE + "".join(f" {7100 + i}     1  {7100 + i} {command}\n" for i, command in enumerate(commands))
    assert len(probe(text, ps_reply([])).check().foreign_processes) == len(commands)


def test_another_guard_run_is_flagged_but_not_the_owners_guard() -> None:
    text = OWN_TREE + "  800   100   800 /bin/bash script/mem-guard run -- go build -p 2 ./...\n"
    report = probe(text, ps_reply([])).check()
    assert len(report.other_guard_processes) == 1
    assert "go build" in report.other_guard_processes[0]


def test_a_guard_check_is_not_a_guard_run() -> None:
    text = OWN_TREE + "  800   100   800 /bin/bash script/mem-guard check\n"
    assert probe(text, ps_reply([])).check().clear


def test_an_installed_ollama_with_a_loaded_model_is_flagged() -> None:
    report = probe(OWN_TREE, ps_reply(["nimble:9b"])).check()
    assert report.installed_ollama_models == ("nimble:9b",)
    assert not report.clear


def test_an_installed_ollama_that_does_not_answer_holds_nothing() -> None:
    assert probe(OWN_TREE, TransportFailure("connection refused")).check().clear


def test_an_installed_ollama_that_errs_holds_nothing() -> None:
    assert probe(OWN_TREE, HttpReply(500, b"boom")).check().clear
    assert probe(OWN_TREE, HttpReply(200, b"not json")).check().clear


def test_the_probe_reads_the_installed_service_on_the_injected_url() -> None:
    transport = ScriptedTransport({"http://127.0.0.1:1/api/ps": ps_reply([])})
    SystemHeavyWorkProbe(
        ScriptedRunner({PS_ARGV: ok(OWN_TREE)}), transport, owner_pid=OWNER, installed_ollama_url="http://127.0.0.1:1"
    ).check()
    assert transport.urls == ["http://127.0.0.1:1/api/ps"]
