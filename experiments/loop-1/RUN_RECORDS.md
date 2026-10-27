# Implementation notes of the loop-1 tooling

Each entry states the rule the tooling follows where `PREREG.md` and
`AMENDMENT-a1.md` leave a detail open. None of them changes a rule or a number of
those documents. Paths are relative to `experiments/loop-1/` unless they start
with `tools/`.

## Work directory and records

- An attempt directory `runs/<run>-<attempt>/` holds `records/` (every file of
  §8 and the analysis files), `project/` (the project the session works in) and
  `objects/` (the git object store of the stop trees, with the project's objects as
  an alternate).
- `hashes-before.json` and `hashes-after.json` map every file of the project to its
  SHA-256, git-ignored files included and `.git/` left out.
- The token scan of §7.5 covers every file under `records/`.

## Construction (§3.3, §3.4)

- A violation or a refactor is the whole edited source file, at its path relative
  to the directory of the task's source file.
- The oracle file of a criterion is `test_<id>.py`, the ID in lower case with its
  hyphen replaced by an underscore: `BOX-4` gives `test_box_4.py`.

## Sessions (§6, §7, §13.3)

- A CLI whose `--version` does not start with the word `2.1.295` is refused.
- An image whose digest differs from `inputs.json` before an attempt is recorded
  as that attempt's harness error, and the invocation ends.
- An `api_retry` event with status 429 ends the session as a quota stop. The
  reset time is read from the error's text when it names one.
- C1 records as observed every host the proxy saw a `CONNECT` to, allowed or
  failed. C2 runs `curl https://github.com` with the proxy variables unset, and
  reads `experiments/loop-1/PREREG.md` through its host path. C3 computes the
  held-out kill matrices of its own attempt before the detectors run, so that K3
  is evaluated.

## Oracle and held-out violations (§4)

- A test fails in a run when its JUnit case has a `failure` or `error` child, when
  the run's exit code is neither 0 nor 1, or when its JUnit report is missing,
  unreadable or empty. A test a run did not list counts as failed.
- The held-out command is
  `python -m pytest -c pytest.ini --rootdir . -p no:cacheprovider -q --junitxml=<out> tests`,
  under the same 300 s limit as the oracle.
- A test that fails on the reference tree in any of its three runs contradicts the
  spec.

## Analysis (§9, §10)

- An edit's time is the run's start plus the stream line's arrival time, and it is
  placed against each end-of-turn call's start time.
- The sign test over tasks reads per-task shares, as §10.5 states.
- A give-up is a hook call that exits 1 with the pinned Shallnot's give-up message.
- An oracle or held-out run on the reference tree killed for memory makes every
  test of that run fail on the reference, so each counts as contradicting.

## Canary and inputs (§13.3, §13.4)

- The digests of the canary's records are the SHA-256 of every file of each
  session's records directory, with that session's start time.
- Before the inputs freeze, accept, construct and the canary run the built image
  that the product manifest names; the proxy runs the same image.
- C3's held-out kill matrices cover its equipped and final test directories.
