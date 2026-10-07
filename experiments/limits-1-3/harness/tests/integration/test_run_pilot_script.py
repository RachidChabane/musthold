"""script/run-pilot: independent streams, markers and exit codes, over stub verbs."""

from __future__ import annotations

import os
import shutil
import subprocess
from pathlib import Path

import pytest

pytestmark = pytest.mark.integration
SCRIPT = Path(__file__).resolve().parents[3] / "script" / "run-pilot"
VERBS = ("canary", "import-run-01", "relabel", "run-core", "run-blindspot", "oracle-classify", "audit-sample",
         "run-secondary", "run-luna", "amendment-check", "analyze", "report-lint", "run-window")


def build(tmp_path):
    exp = tmp_path / "root" / "experiments" / "limits-1-3"
    (exp / "script").mkdir(parents=True)
    (tmp_path / "root" / "script").mkdir()
    shutil.copy(SCRIPT, exp / "script" / "run-pilot")
    guard = tmp_path / "root" / "script" / "mem-guard"
    guard.write_text('#!/usr/bin/env bash\nshift 2\nexec "$@"\n')
    guard.chmod(0o755)
    for verb in VERBS:
        stub = exp / "script" / verb
        stub.write_text(f'#!/usr/bin/env bash\nname={verb}\nif [[ "$name" != run-window ]]; then echo "$name" >> "{tmp_path}/calls"; fi\n'
                        f'code_var="EXIT_$(echo "$name" | tr - _)"\nexit "${{!code_var:-0}}"\n')
        stub.chmod(0o755)
    return exp


def run(exp, tmp_path, **env):
    result = subprocess.run([str(exp / "script" / "run-pilot"), "run-02"], cwd=exp, capture_output=True, text=True,
                            env={**os.environ, **env})
    calls = (tmp_path / "calls").read_text().split() if (tmp_path / "calls").exists() else []
    return result.returncode, calls


def test_a_full_run_follows_the_order_and_a_second_invocation_does_nothing(tmp_path):
    exp = build(tmp_path)
    code, calls = run(exp, tmp_path)
    assert code == 0 and calls == ["canary", "import-run-01", "relabel", "run-core", "run-blindspot", "oracle-classify",
                                   "audit-sample", "run-secondary", "run-luna", "oracle-classify", "amendment-check", "analyze", "report-lint"]
    (tmp_path / "calls").unlink()
    assert run(exp, tmp_path) == (0, [])


def test_a_paused_secondary_stream_does_not_keep_luna_from_starting_and_stops_the_later_steps(tmp_path):
    exp = build(tmp_path)
    code, calls = run(exp, tmp_path, EXIT_run_secondary="3")
    assert code == 3 and calls[-3:] == ["run-secondary", "run-luna", "oracle-classify"]
    assert "amendment-check" not in calls and "analyze" not in calls


def test_a_paused_luna_stream_does_not_keep_the_secondary_stream_from_finishing(tmp_path):
    exp = build(tmp_path)
    code, calls = run(exp, tmp_path, EXIT_run_luna="3")
    assert code == 3 and calls[-2:] == ["run-secondary", "run-luna"] and "amendment-check" not in calls
    (tmp_path / "calls").unlink()
    code, calls = run(exp, tmp_path)
    assert code == 0 and calls[0] == "run-luna" and calls[-3:] == ["amendment-check", "analyze", "report-lint"]
    assert "run-secondary" not in calls


def test_a_failure_before_the_streams_stops_at_once(tmp_path):
    exp = build(tmp_path)
    code, calls = run(exp, tmp_path, EXIT_run_core="1")
    assert code == 1 and calls[-1] == "run-core"


def test_outside_the_run_window_a_model_step_is_not_started(tmp_path):
    exp = build(tmp_path)
    code, calls = run(exp, tmp_path, EXIT_run_window="3")
    assert code == 3 and calls == []
