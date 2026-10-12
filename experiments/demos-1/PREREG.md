# Pre-registration: first scan demos (demos-1)

This document fixes, before any repository is cloned, which public repositories and
which commits `spec-proof scan` runs on for its first demos, which scans run on each,
what is recorded and what may be said about the results. It applies the demo protocol
of the `scan` verdict (`notes/debates/debate-scan-verdict.md`) under ADR 0014.

- Everything stays on this machine. Nothing is published. No issue, comment, review,
  reaction, star, fork, push or message reaches any third-party repository or its
  maintainers. The only network operations on third-party repositories are read-only
  GitHub API calls, clones, `uv python install` (§8.2) and the dependency downloads
  of §8.3.
- Every attempt is kept and reported, failures and zero results included.
- Ecosystems: pytest and Go.
- Target: 4 picks, 2 per ecosystem, from stratum B first (§6) and stratum A only when
  stratum B cannot fill its ecosystem (§7).

This document departs from the verdict's demo protocol on these points:

- **No publication.** The publication of every attempt, the notice to maintainers
  14 days before publication, and the publication of their replies do not apply.
  Every attempt is reported in the local summary (§12).
- **Ecosystems.** Only pytest and Go. The Vitest and Jest strata are not drawn.
- **Self-scan.** The scan of spec-proof itself is not part of this document.
- **Suite limit.** One limit of 60 s for both strata (§8.4), in place of 2 minutes
  for stratum A and 5 minutes for stratum B.
- **Stratum B.**
  - The requirement of a pull request body or linked issue is dropped.
  - The agent signals are the bot author of the pull request (Copilot, Claude) and
    a Codex task link in its body (§6.2), in place of commit trailers and
    "Generated with" lines.
  - The 10 pull requests per ecosystem are the latest merged in a fixed window
    (§6.3).
- **Stratum A.** The 5 most-starred repositories per ecosystem are attempted only
  once stratum B is exhausted for that ecosystem (§8.1).

Freeze: §15.

## 1. Layout and front door

Root: `experiments/demos-1/`.

| Path | Content |
|---|---|
| `PREREG.md` | this document |
| `freeze/` | the frozen commit hash and its RFC 3161 timestamp (§15) |
| `script/` | shebang'd entry points, one per verb |
| `Makefile` | thin targets over `script/<verb>` |

The verbs are `select` (§5 to §7), `attempt` (§8, §9), `summarize` and
`report-lint` (§12). Each runs as `script/<verb>` and as `make <verb>`. They are
tooling written after the freeze; their commit is recorded in the manifest (§11).

The committed summary lives under `notes/demos-1/` (§12.2).

The work directory `$DEMOS1_WORK`, default `$HOME/.spec-proof-demos-1`, lies outside
every git work tree. It holds every raw record, clone and scan output. Nothing under
it is committed.

## 2. Product and toolchains

- **Product.** The tag `demos-1-product` is set, before `select` runs, on a commit
  that descends from the tag `prereg-demos-1`. The tooling builds `spec-proof` and
  `spec-proof-dp` from that commit with
  `CGO_ENABLED=0 GOOS=linux GOARCH=<host architecture> go build -ldflags "-X main.version=<commit> -X main.commit=<commit>"`.
  Both binaries are mounted read-only in one directory of the container,
  `/opt/spec-proof/bin`, which is on `PATH`. The manifest records:
  - the commit;
  - the output of `spec-proof --version` run in the container;
  - the SHA-256 of both binaries;
  - the commits between the two tags that touch `discpower/`, `format/` or
    `umbrella/`.
- **A product change after `demos-1-product`** is a deviation (§14). Its reason is
  written to `DEVIATIONS.md` before any scan runs with it. It takes a new tag
  `demos-1-product-<k>`, and every pick scanned before it is scanned again with
  every scan of §9. The results of the new tag are reported beside those of
  `demos-1-product`. The headlines and the shown finding of §12.1 always come from
  the scans of `demos-1-product`.
- **No other access.** Between the tag `prereg-demos-1` and the last attempt, no
  third-party repository is cloned or scanned except through `attempt`.
- **Toolchains.** These are recorded in the manifest and in every attempt record:
  - Go 1.26.4, the version `go.work` names, with `GOTOOLCHAIN=local`;
  - uv 0.11.23;
  - the CPython version each pytest attempt selects (§8.2);
  - the git version of the image, gh 2.93.0 on the host, and the Docker engine
    version;
  - the colima version and the settings of the engine's virtual machine (§13);
  - the OS name, version and architecture of both the host and the image.

  A tool version or engine setting that differs from the one recorded at `select`
  is a deviation.
- **No host.** No scan runs with a host, a skill or a model. Each scan prints "no
  model used" and "promises not checked"; a scan output without both is a
  deviation.
- **Container.** Every command that runs code from a cloned repository, runs git on
  a clone, or installs what a repository needs runs inside a Linux container, never
  on the host (§13). This covers:
  - the clone of §8.2 and every later git command;
  - `uv python install` (§8.2);
  - the installation of §8.3;
  - the suite runs of §8.4;
  - the scans and replays of §9.
- **Image.** The tooling builds the image from `golang:1.26.4-bookworm`, with git
  from the base distribution and the `uv` binary copied from
  `ghcr.io/astral-sh/uv:0.11.23`. It adds the wrapper of §13. Before `select`, the
  tooling resolves both source images to digests and builds from those digests.
  The manifest records the two source digests and the built image's digest. Every
  attempt runs that image. A digest that changes between attempts is a deviation.

## 3. Exclusions

A repository is excluded when the lowercased `full_name` that the GitHub API returns
for it, after redirects, is one of the following:

- every `repository` of `bench/mining/20261007/candidates.jsonl` (48 repositories,
  SHA-256 `06d6cfcaeca245047f6b16d2e8c621400561f9c3f239002a465b2e7cf33c4cd7`). This
  file is the candidate pool of the benchmark sample. It holds:
  - every task and reserve of `bench/selection/20261007/selection.json`, which are
    the tasks ADR 0004 seals;
  - the 24 `not-selected` subjects of `bench/selection/20261007/attrition.jsonl`,
    which are the repository candidates of `experiments/validity-w/PREREG.md` §2.3;
- every repository owned by `RachidChabane`;
- every repository already placed on a list of this document (§6.5, §7.2).

`select` refuses to run when the digest of `candidates.jsonl` differs from the one
above.

## 4. Licence

A repository is permissive when its `license.spdx_id` in the API response is one of
`PERMISSIVE_SPDX` in `bench/src/specbench/domain/licence.py`: MIT, Apache-2.0,
BSD-2-Clause, BSD-3-Clause, ISC. A missing licence, `NOASSERTION` or any other
identifier is not permissive.

## 5. Snapshot and window

- **Selection date.** `select` runs once, after the timestamp of §15 exists and
  before any clone, within one UTC calendar date: the selection date. The summary
  states it.
- **Window W.** A pull request belongs to W when its `merged_at` lies in
  `[2026-07-01T00:00:00Z, 2026-09-30T23:59:59Z]`.
- **Default-branch commit for stratum A.** The commit a stratum A pick is scanned at
  is the first commit returned by
  `gh api "repos/<o>/<r>/commits?sha=<default_branch>&until=2026-09-30T23:59:59Z&per_page=1"`.
- **Authoritative record.** `select` saves every API response under
  `$DEMOS1_WORK/selection/responses/`, one file per request, with:
  - the request path and parameters;
  - the UTC fetch time;
  - the HTTP status;
  - the body;
  - the body's SHA-256.

  The lists of §6 and §7 are a pure function of these saved responses. Anyone who
  re-runs `select` on the selection date gets the same lists, except where the live
  API changed between the two runs. In that case the saved responses decide.
- **Search hygiene.**
  - A search response with `incomplete_results: true` is retried up to 3 times,
    60 s apart.
  - A pull request search interval (§6.1) whose `total_count` is 1000 or more, or
    that is still incomplete after the retries, is split into two halves at the
    middle second, recursively, until every part is under 1000 and complete.
  - A one-second interval that stays incomplete is used as returned and recorded as
    `incomplete` in the frame.
  - Pages are fetched with `per_page=100` until the items fetched reach
    `total_count` or a page comes back empty.
  - The repository search of §7.1 is never split (§7.1).
  - `select` waits at least 2.1 s between two search requests.
- **Failed requests.** A rate-limited response (HTTP 403 or 429 with a rate-limit
  header) is retried after the reset time it names, up to 3 times. After that, any
  non-2xx response has these consequences:
  - for a request a filter issued, the candidate fails that filter, recorded with
    the status;
  - for a search request, `select` stops. The stopped run is kept and logged as a
    deviation, and `select` runs again from the start, within one UTC date.
- **Order.** Every order of this document is computed by `select` from the saved
  responses, never taken from the API's order.

## 6. Stratum B: merged pull requests carrying an agent signal

Stratum B builds one list per ecosystem: pytest, searched under GitHub language
`Python`, and Go, searched under `Go`. `select` builds the lists in this order:
`B-pytest`, `B-go`, `A-pytest`, `A-go`. The exclusions of §3 that name lists read
the lists built before.

### 6.1 Search queries (candidates)

For each ecosystem with search language `<L>`, and for each day `<d>` of W:

```
gh api -X GET search/issues -f per_page=100 -f page=<p> \
  -f q='is:pr is:merged is:public author:app/copilot-swe-agent language:<L> merged:<d>T00:00:00Z..<d>T23:59:59Z'
gh api -X GET search/issues -f per_page=100 -f page=<p> \
  -f q='is:pr is:merged is:public author:app/claude language:<L> merged:<d>T00:00:00Z..<d>T23:59:59Z'
```

Once per ecosystem, over the whole window:

```
gh api -X GET search/issues -f per_page=100 -f page=<p> \
  -f q='is:pr is:merged is:public "chatgpt.com/codex/tasks" in:body language:<L> merged:2026-07-01T00:00:00Z..2026-09-30T23:59:59Z'
```

The items of the three queries are merged and deduplicated by `html_url`. Each
item is assigned to the day of its `pull_request.merged_at`, in UTC.

### 6.2 Signal (membership)

The search only proposes candidates. A candidate is in the frame when its search
item meets one of these:

- **copilot:** `user.type` is `Bot` and `user.login` is `Copilot`;
- **claude:** `user.type` is `Bot` and `user.login` is `claude[bot]`;
- **codex:** `body` contains the byte string `https://chatgpt.com/codex/tasks/`.

The record names every signal the candidate meets.

### 6.3 Processing order

- Days are processed one at a time, from 2026-09-30 back to 2026-07-01.
- Within a day, candidates are ordered by:
  1. `merged_at`, latest first;
  2. the repository's `full_name`, byte-wise ascending;
  3. the pull request number, ascending.
- Each candidate goes through the filters of §6.4 in that order. The record keeps
  the first filter it fails, or its rank on the list.
- A day is always processed whole.
- Processing of an ecosystem stops at the end of the first day after which its list
  holds at least 10 entries. The list is then cut to its first 10. If W runs out
  first, the list keeps what it holds.

### 6.4 Filters, in order

1. `signal`: §6.2.
2. `excluded`: §3, the repository already on a list included. The canonical
   `full_name` comes from `gh api repos/<o>/<r>`, fetched before this filter, once
   per repository, and cached for the run.
3. `repository`: from the same response:
   - `fork`, `archived` and `private` are all false;
   - `stargazers_count` is at least 1000;
   - the licence is permissive (§4).
4. `pull-request`: from `gh api repos/<o>/<r>/pulls/<n>`:
   - `merged` is true;
   - `merge_commit_sha` is set;
   - `changed_files` is at most 100.
5. `tests-changed`: in `gh api "repos/<o>/<r>/pulls/<n>/files?per_page=100"`, at
   least one file:
   - has status `added`, `modified` or `renamed`;
   - has no path segment `vendor`;
   - has a basename matching the scan's test globs for the ecosystem: `test_*.py` or
     `*_test.py` for pytest, `*_test.go` for Go.

   Only paths are read, never file contents.
6. `source-changed`: at least one file of the same list:
   - has status `added`, `modified` or `renamed`;
   - has no path segment `vendor`;
   - ends in `.py` for pytest or `.go` for Go;
   - is not a test file by filter 5;
   - is not named `conftest.py`.
7. `merge-diff`: from `gh api repos/<o>/<r>/commits/<merge_commit_sha>`:
   - the base `P` is `parents[0].sha`;
   - every `filename` of the pull request's file list appears among the commit's
     `files`.
8. `ecosystem`: the scan's own detection, applied at the root of the merge commit,
   through `gh api "repos/<o>/<r>/contents?ref=<merge_commit_sha>"`:
   - **pytest** when the root holds `pytest.ini` or `conftest.py`, or when one of
     these files at the root contains its section string:
     - `tox.ini` contains `[pytest]`;
     - `setup.cfg` contains `[tool:pytest]`;
     - `pyproject.toml` contains `[tool.pytest`;
   - **Go** when the root holds `go.mod`.

   pytest is tested first, as the scan's registry does. The candidate passes when
   the first ecosystem detected is the list's ecosystem. Only the root listing and
   the three configuration files are read. Of `conftest.py`, only its presence is
   read.
9. `listed`: the candidate passes and takes the next rank. Every later candidate
   from the same repository fails `excluded`.

### 6.5 The B lists

`B-pytest` and `B-go`, each up to 10 entries. Each entry records:

- the rank;
- the repository `full_name` and its stars at the fetch;
- the licence;
- the pull request number and URL;
- the signals;
- `merged_at`;
- the merge commit `M` and the base `P`;
- the pull request's file list, with each path's status and whether it is a test
  path by filter 5.

## 7. Stratum A: most-starred repositories

### 7.1 Query

For each ecosystem with search language `<L>`, pages 1 to 10, all fetched:

```
gh api -X GET search/repositories -f sort=stars -f order=desc -f per_page=100 -f page=<p> \
  -f q='language:<L> stars:>=1000 archived:false fork:false is:public'
```

This search is exempt from the splitting of §5. Its results are the items of
these 10 pages. Candidates are ordered by `stargazers_count` (highest first), then
by `full_name` (byte-wise ascending). Each is then filtered, in order:

1. `excluded`: §3, the B lists of both ecosystems included.
2. `repository`: `fork`, `archived` and `private` are all false, and the licence is
   permissive (§4).
3. `commit`: the default-branch commit `C` of §5 exists.
4. `ecosystem`: filter 8 of §6.4, applied at `C`.

Processing stops once the list holds 5 entries.

### 7.2 The A lists

`A-pytest` and `A-go`, each up to 5 entries. Each entry records:

- the rank;
- `full_name`, stars and licence;
- the default branch;
- `C` and its committer date.

## 8. Attempts

### 8.1 Order and replacement

- **Ecosystem order.** pytest is attempted first, then Go. One attempt runs at a
  time.
- **Within an ecosystem**, the candidates are attempted in this order:
  1. the B list, in rank order;
  2. the A list, in rank order, only once the B list is exhausted.

  Attempts stop as soon as the ecosystem holds 2 picks.
- **Picks.** An attempt that passes every setup stage of §8.2 to §8.4 is a **pick**.
  Its scans (§9) then run.
- **Setup failure.** An attempt that fails a setup stage is recorded with:
  - the outcome name (§10);
  - the command;
  - the exit status;
  - the wall time;
  - the log.

  The next candidate is attempted in its place. A setup failure is never retried,
  and no recipe is adjusted by hand. The waiting, interruption and container rules
  of §9.3 apply to every setup command. Only a stage that ends in one of the
  outcomes of §10 counts as a setup failure.
- **A pick is final.** Whatever its scans produce, a pick is never replaced. This
  holds for:
  - zero tests that verify nothing;
  - every test unproven;
  - a partial scan;
  - a scan that exits non-zero;
  - a scan killed at its wall-time cap;
  - a scan stopped by its memory limit or by the memory guard (§9.3).
- **Results.** The outcome is between 0 and 4 picks. Fewer than 4 picks, or fewer
  than 3, is a result and is reported as such. No candidate outside the four lists
  is ever attempted.

### 8.2 Clone and toolchain

- **Home.** Before the clone, the tooling writes `<attempt>/home/.gitconfig`,
  holding only `[safe] directory = <clone>`.
- **Clone.** One container command, with the network enabled and
  `GIT_ALLOW_PROTOCOL=https`, fetches the full repository into the attempt's
  directory and checks out the commit detached:

  ```
  git clone --no-checkout https://github.com/<o>/<r>.git <clone>
  git -C <clone> checkout --detach <M or C>
  ```

  - When `.gitmodules` exists, `git -C <clone> submodule update --init --recursive`
    runs next, in the same command.
  - For a B pick, `git -C <clone> cat-file -e P^{commit}` must succeed, in the same
    command.
  - The outcome `setup-clone` follows any non-zero exit, or 15 minutes.
- **Go toolchain.** The tooling reads the root `go.mod` of the checkout as a file.
  The outcome is `setup-toolchain` when either of these exceeds 1.26.4:
  - its `go` directive;
  - its `toolchain` directive, when present.
- **Python version.** The tooling reads the files of the checkout to pick the
  version request. The first applicable rule decides:
  1. When the root holds `.python-version`, its first non-empty line is the version
     request.
  2. Otherwise, take the `requires-python` of the root `pyproject.toml`, else the
     `python_requires` of the root `setup.cfg`. The request is the highest of 3.13,
     3.12, 3.11 and 3.10 that satisfies it. A minor version satisfies it when the
     CPython release that uv 0.11.23 selects for that minor does.
  3. Otherwise, 3.13.

  `uv python install <request>` then runs in a container command with the network
  enabled. The outcome is `setup-toolchain` in either case:
  - no listed version satisfies the specifier;
  - the command exits non-zero.

### 8.3 Dependency installation

The first applicable recipe runs, once, as one container command with the network
enabled. Its commands run from the clone's root, under a 20-minute limit for the
whole recipe.

- **Go:** `go mod download`.
- **pytest, `uv.lock` at the root:**
  1. `uv sync --frozen --all-extras --all-groups --python <request>`;
  2. when `.venv/bin/python -c "import pytest"` fails:
     `uv pip install --python .venv/bin/python pytest`.
- **pytest, otherwise:**
  1. `uv venv .venv --python <request>`.
  2. When the root holds `pyproject.toml`, `setup.py` or `setup.cfg`:
     `uv pip install --python .venv/bin/python -e '.[<extras>]'`, where `<extras>`
     are, comma-separated, the keys of `[project.optional-dependencies]` among
     `test`, `tests`, `testing` and `dev`, in that order. With no such key the
     command installs `-e .`.
  3. For each `[dependency-groups]` group among `test`, `tests`, `testing` and
     `dev`, in that order: `uv pip install --python .venv/bin/python --group <name>`.
  4. For each of the following files present, in this order:
     `uv pip install --python .venv/bin/python -r <file>`.
     - `requirements.txt` (only when step 2 did not run);
     - `requirements-dev.txt`, `requirements_dev.txt`, `dev-requirements.txt`;
     - `requirements-test.txt`, `requirements_test.txt`, `test-requirements.txt`;
     - `requirements/dev.txt`, `requirements/test.txt`, `requirements/tests.txt`.
  5. When `.venv/bin/python -c "import pytest"` fails:
     `uv pip install --python .venv/bin/python pytest`.

The outcome `setup-install` follows any non-zero exit, or the limit.

**Clean tree.** `git -C <clone> status --porcelain --untracked-files=no` runs in an
offline container command at two points:

- after the recipe;
- after the second suite run of §8.4.

The outcome is `setup-dirty-tree` when it prints anything.

### 8.4 Suite run and runtime

The suite runs twice in a row, each run as one offline container command, from the
clone's root, with one fixed command:

- **pytest:**

  ```
  .venv/bin/python -m pytest -p no:cacheprovider -q --junitxml=<attempt>/suite-<k>.xml
  ```
- **Go:**

  ```
  go test -count=1 -p 2 -json ./... > <attempt>/suite-<k>.jsonl
  ```

The wrapper of §13 runs each command under `timeout -s KILL`, and measures both its
wall time and its peak memory inside the container. The start-up of `docker run`
is never counted.

- **First run.** It warms the build and bytecode caches. Its outcome is recorded.
  It is killed at 15 minutes, with the outcome `setup-suite-first-run-timeout`.
- **Second run.** Its wall time is the suite runtime. It is killed at 60 s.

Outcomes of the second run:

- `setup-suite-too-slow`: it reached 60 s;
- `setup-suite-does-not-run`:
  - pytest: an exit status other than 0 or 1;
  - Go: no `pass` event that names a test;
- `setup-too-few-passing`: fewer than 12 passing tests:
  - a pytest test passes when its JUnit test case has no `failure`, `error` or
    `skipped` child;
  - a Go passing test is a distinct (`Package`, `Test`) pair with an
    `"Action": "pass"` event, subtests included.

### 8.5 Pick

An attempt that passes §8.2 to §8.4 is a pick. The scans of §9 run on the same
clone, in their listed order. Between two scans, only the moves and replays of §9.2
touch it.

## 9. Scans

### 9.1 Commands

Every scan is one offline container command (§13) that runs this command, with
the added flags of the table:

```
spec-proof scan --root <clone> --seed 7 --workers 2 --format text
```

| Pick | Scan, in order | Added flags |
|---|---|---|
| B | `change`: change mode at the default depth | `--base <P>` |
| B | `quick`: the default repository scan | none |
| B | `change-full`: change mode at full depth | `--base <P> --full --budget 60m` |
| A | `quick`: the default repository scan | none |
| A | `full` | `--full --budget 60m` |

- The checkout is `M` for a B pick and `C` for an A pick.
- No other flag is given. `--sample`, `--mutants` and `--coverage-timeout` keep
  their defaults.

On the host, the full command line of a scan is:

```
script/with-slot 2 --wait 3600 -- script/mem-guard run -- \
  docker run --name <id>-<scan>-r<k> <options of §13> <image> \
  spec-proof scan --root <clone> --seed 7 --workers 2 --format text <added flags>
```

### 9.2 After each scan

Each run of a scan gets its own new, empty `XDG_CACHE_HOME`, `XDG_STATE_HOME` and
`TMPDIR` under the scan's record directory. After each scan, and before the next
one starts:

1. **Move.** On the host, `.spec-proof/scan/<commit>/` is moved, as plain files,
   out of the clone into the scan's record directory, `<moved>`.
2. **Replay.** For each finding folder `<n>` of `<moved>`, in number order:

   ```
   spec-proof scan replay --root <clone> <moved>/<n>
   ```

   - Each replay is one offline container command, with the wrapper and the rules
     of a scan (§9.1, §9.3).
   - Container names are `<id>-<scan>-replay-<n>-r<k>`.
   - Its exit status reads:
     - 0: "holds on replay";
     - 1: "no longer holds on replay";
     - any other exit, a memory-limit stop or a third guard interruption:
       "replay failed".

### 9.3 Machine budget and stops

- **Slots.**
  - Scans and replays hold two slots: `script/with-slot 2 --wait 3600`.
  - Setup commands hold one: `script/with-slot 1 --wait 3600`.
  - `script/mem-guard run` and `nice` wrap the `docker run` client on the host.
    The CPU limit of the container is its `--cpus 2`.
- **Container exits.** After every container exits or is stopped, the verb:
  1. records `docker inspect -f '{{.State.ExitCode}} {{.State.OOMKilled}}' <name>`;
  2. then runs `docker rm <name>`.
- **Memory limit.** `OOMKilled` true, whatever the exit status, is `scan-oom` for a
  scan and `setup-oom` for a setup command. It is never rerun.
- **Wall-time cap.** A scan still running after 4 hours is stopped with
  `docker stop -t 10 <name>` and recorded as `scan-killed`. It is never rerun.
- **Memory guard.** A command that `mem-guard` interrupts (exit 5) gets
  `docker stop -t 10` on its container. It leaves no result. It is rerun from the
  start, under a new container name `-r<k+1>`, with fresh XDG and `TMPDIR`
  directories, up to 3 runs in all. Each interrupted run is recorded. After 3
  interruptions:
  - a scan is recorded as `scan-interrupted`;
  - a setup stage as `setup-interrupted`;
  - a replay as "replay failed".
- **Waiting.** These refusals are neither a run nor an attempt, and the verb waits
  and tries again:
  - `with-slot` exit 75;
  - `mem-guard` exit 4;
  - an engine refusal: `docker info` fails, or `docker run` exits 125.
- **Bounded waits.** When refusals add up to 6 hours within one invocation of
  `attempt`, the verb stops and logs a deviation. The next invocation resumes at
  the same command. No candidate is replaced for it.
- **Other exits.** Any other non-zero exit of a scan is recorded as `scan-error`,
  with its stderr.

## 10. What is recorded per attempt

`$DEMOS1_WORK/attempts/<id>/attempt.json`, where `<id>` is
`<stratum>-<ecosystem>-<rank>`, for example `B-go-03`. It holds:

- **Identity:**
  - the list and the rank;
  - the repository, its stars and its licence;
  - for B, the pull request URL, the signals, `merged_at`, `M`, `P`, and the
    changed paths with their status and test flag;
  - for A, the default branch and `C`;
- **Product and toolchain:** the product tag and commit, the binary digests, the
  image digest, every toolchain version of §2, the Python request and the resolved
  CPython version;
- **Every container command** (setup, suite, scan, replay):
  - its container name;
  - the exact command line and environment;
  - whether the network was enabled;
  - the start and end times in UTC;
  - the wall time and the peak memory of §13;
  - the `docker inspect` exit code and `OOMKilled`;
  - the paths of its stdout, stderr and log;
- **Setup:** the stages reached, and the outcome of the first failing stage, if any;
- **Suite:** the passing, failing, erroring and skipped counts of both runs, and the
  suite runtime;
- **Per scan:** the SHA-256 of `scan.json`, and:
  - from `scan.json`: the five class counts (verifies nothing, caught at least
    one change, unproven, not sampled and not green), the sampled green total,
    the total number of tests, the seed and the mutants it lists;
  - from the scan's printed headline (§12.1), as printed: the depth, the budget,
    the workers, whether the scan is partial, and the mutant line's counts (ran,
    proven, identical when compiled, did not build or start, not run within the
    budget). No count the headline does not print is derived from the others;
- **Per finding** (each test that verifies nothing):
  - the test id;
  - the scan's literal sentence;
  - the site, as file and line;
  - the route;
  - the finding folder;
  - the replay result of §9.2;
  - for B: whether the test's file is among the pull request's changed test paths,
    and with which status (added, modified or renamed);
- **Outcome:** `pick`, or one of these, with the log path:
  - `setup-clone`;
  - `setup-toolchain`;
  - `setup-install`;
  - `setup-dirty-tree`;
  - `setup-suite-first-run-timeout`;
  - `setup-suite-too-slow`;
  - `setup-suite-does-not-run`;
  - `setup-too-few-passing`;
  - `setup-oom`;
  - `setup-disk`;
  - `setup-interrupted`.

  For each scan of a pick, the outcome is `scan-ok` (exit 0), or one of:
  - `scan-error`;
  - `scan-killed`;
  - `scan-oom`;
  - `scan-disk`;
  - `scan-interrupted`.

## 11. Raw data

`$DEMOS1_WORK` holds:

| Path | Content |
|---|---|
| `manifest.json` | the commits tagged `prereg-demos-1` and `demos-1-product`, the tooling commit, the binary digests, the image and source-image digests, the engine's virtual-machine settings, the toolchain versions, the selection date |
| `selection/responses/` | every API response of §5 |
| `selection/frame.jsonl` | every candidate examined, with its first failing filter or its rank |
| `selection/lists.json` | the lists of §6.5 and §7.2 |
| `attempts/<id>/` | `attempt.json`, the logs, `suite-<k>.*`, and one directory per scan holding its outputs, its moved scan directory and its replay outputs; the clone, `home/` and `cache/` while the attempt runs (§13) |
| `DEVIATIONS.md` | §14 |

Finding folders stay until the summary is committed and the owner decides on
publication. A clone is deleted at the end of its attempt (§13); the recorded commit
recreates it. Nothing under `$DEMOS1_WORK` is published or committed.

## 12. Analysis and reporting

### 12.1 What is reported

- **Every attempt** in list order, with its outcome. Setup failures are counted per
  outcome, list and ecosystem, each as `k/N attempts`.
- **Per pick and per scan:** the scan's headline as it printed it. A quick-depth
  scan whose budget ran out before its mutants ran is reported with that headline,
  its reference and coverage times and its unrun mutant count, like any other:
  - the five counts, each over its own denominator:
    - `k/n verify nothing`, `c/n caught ≥ 1 change` and `u/n unproven`, where n is
      the sampled green tests;
    - `s/g not sampled`, where g is the green tests;
    - `x/T tests not green`, where T is the total number of tests;
  - the depth, the budget, `partial` when the budget stopped the scan, and the
    mutants left unrun;
  - the wall time, the seed and the workers.
- **Per finding:**
  - the test id;
  - the literal sentence of the scan;
  - the site;
  - the route;
  - the replay result of §9.2;
  - for B, the relation of the test's file to the pull request:
    - "file added by the pull request";
    - "file modified by the pull request";
    - "file not changed by the pull request".
- **Shown finding.** The shown finding of a pick is the `#1` of the first of these
  scans of `demos-1-product`, in this order, that exited 0 and has at least one
  finding:
  - B: `change-full`, `change`, `quick`;
  - A: `full`, `quick`.

  The shown finding is reported with its replay result. When no such scan exists,
  the pick is shown as its headlines alone.
- **Refuted findings.** A finding that "no longer holds on replay" is reported as
  such and counted. A "replay failed" is reported and counted apart, never as a
  refutation.

### 12.2 Committed summary

After the last attempt, `summarize` writes two files from `$DEMOS1_WORK`:

- `notes/demos-1/summary.json`, which holds:
  - the lists;
  - every attempt record, without logs or mutant patches; the only third-party
    text it holds is what the headlines of §12.1 print verbatim (test ids, file
    paths, the changed token and a differential input and its results);
  - the digests of `manifest.json`, `lists.json` and every `scan.json`;
  - every fraction the summary prints;
- `notes/demos-1/SUMMARY.md`: the same, rendered.

They are the only material of this run committed to the repository. `summarize`
refuses to write them unless both of these hold:

- `gh repo view --json visibility` reports `PRIVATE` for the repository's `origin`;
- `notes/public-history-report.md` records the public export's path filter as
  `--invert-paths --path notes/`, which leaves `notes/` out of every exported
  revision.

### 12.3 What is never claimed

- **Banned words.** No result is called a bug, a PASS, clean or safe, and no
  repository, pull request or suite gets a score.
- **No gain figure.** No figure about time saved, review saved or any other gain.
- **Denominators.** A fraction never appears without its denominator. A count never
  stands without its denominator.
- **No pooling.** No rate is pooled across picks, strata, ecosystems or agents. No
  result is extrapolated to a repository's whole suite, to the repositories of a
  stratum, to agent-written tests, or to a coding agent or tool. No comparison is
  drawn between stratum A and stratum B, or between the signals.
- **Authorship.** A test is never said to be written by an agent. The most that may
  be said is the relation of §12.1: "its file was added (or modified) by pull
  request #n, which carries signal s".
- **Unjudged tests.** An unproven, unsampled or not-green test is never said to
  verify nothing, nor to verify something. A zero is stated as
  "0/n sampled tests verify nothing; unproven and unsampled tests are unjudged".
- **No verdict on the code.** Nothing is said about the correctness of a pull
  request or of a repository's code.

`report-lint` checks `SUMMARY.md`, outside quoted scan output: fenced blocks,
inline code spans and lines starting with `>`. It fails on:

- **Words**, as whole words:
  - `bug`, `bugs`, `buggy`, `clean`, `safe`, `score`, `authored`, `wrote`,
    `agent-written`, `AI-written`, and the phrases `written by` and `generated by`,
    in any case;
  - `PASS` in upper case;
  - the pattern `by (Copilot|Claude|Codex)`, in any case;
- **The character `%`;**
- **Fractions:** any fraction `k/n` that is not in the list of fractions of
  `summary.json`.

## 13. Environment and machine safety

- **Host.** The host runs only these:
  - the GitHub API calls of `select`;
  - the `docker` client;
  - plain file reads and moves under the attempt directory (§8.2, §9.2).

  It runs no git command under `$DEMOS1_WORK/attempts/`, and no code from a clone.
- **Container options.** Every container command runs as `docker run`, never with
  `--rm`, with:
  - `--name <id>-<stage>-r<k>`, where `<k>` counts the runs of that command; for
    scans and replays, `<stage>` is the name §9.1 and §9.2 give;
  - `--user <host uid>:<host gid>`, `--cap-drop ALL` and
    `--security-opt no-new-privileges`;
  - `--cpus 2`, `--memory 2560m` and `--memory-swap 2560m` (2.5 GiB);
  - two mounts and no other host path:
    - the attempt directory, read-write, at the same path;
    - the product binaries, read-only, at `/opt/spec-proof/bin`.

    No home directory, SSH agent, keychain, Docker socket or credential is
    mounted;
  - the network enabled only for:
    - the clone of §8.2;
    - `uv python install` (§8.2);
    - the installation of §8.3.

    Every other command runs with `--network none`.
- **Environment.** Inside the container, every command runs through the wrapper,
  under `env -i`, with only these variables:
  - `PATH`, holding `/opt/spec-proof/bin`, the image's Go and uv, and the system
    directories;
  - `HOME=<attempt>/home`;
  - a `TMPDIR` new for each run;
  - `GOPATH`, `GOMODCACHE` and `GOCACHE` under `<attempt>/cache/go/`, with
    `GOTOOLCHAIN=local` and `GOFLAGS=-p=2`;
  - `UV_CACHE_DIR` and `UV_PYTHON_INSTALL_DIR` under `<attempt>/cache/uv/`;
  - the XDG variables of §9.2;
  - `LANG=C.UTF-8` and `TZ=UTC`;
  - `GIT_ALLOW_PROTOCOL=https`, for the clone only;
  - `GOPROXY=off` and `UV_OFFLINE=1`, for offline commands.

  No credential or token variable is passed, and no cache is shared between
  attempts.
- **Wrapper.** The wrapper, built into the image, runs the command under
  `timeout -s KILL <limit>` when it has a limit. It measures the command's wall
  time on a monotonic clock. At exit, it reads the container cgroup's
  `memory.peak`, which is the peak memory this document records. It writes both
  next to the command's log.
- **Limits.** Work is limited to 2 workers: `--workers 2` for scans, `-p 2` for Go,
  and `--cpus 2` for every container. The slots and the guard of §9.3 wrap every
  container command. One container runs at a time: the verb runs attempts, stages
  and scans strictly one after another.
- **Engine.** The engine is a colima virtual machine of its own profile,
  `spec-proof-demos`, started with 2 CPUs, 3 GiB of memory and a 40 GiB disk. Its
  only mounts are `$DEMOS1_WORK`, read-write, and the directory of the product
  binaries, read-only; no other host directory is visible to it. Every docker
  command of the tooling names that profile's socket through `DOCKER_HOST`; the
  tooling changes no other Docker configuration of the host and leaves the current
  docker context as it found it. `attempt` starts the machine before its first
  container and stops it (`colima stop --profile spec-proof-demos`) when it ends,
  whatever its exit; the machine never runs while no command of this document
  runs.
- **Disk.**
  - An attempt starts only when the volume of `$DEMOS1_WORK` has at least
    50 GiB free; otherwise the verb waits, as for a slot refusal (§9.3).
  - After every container command, the size of the attempt directory is
    measured. Above 15 GiB, the command's stage ends as `setup-disk` or
    `scan-disk`, and the attempt stops there.
  - Once an attempt's records are written (§10) and every replay of §9.2 has
    run, the tooling deletes the clone, `<attempt>/cache/`, `<attempt>/home/` and
    every `TMPDIR`. It keeps `attempt.json`, the logs, the suite records and the
    directory of each scan with its moved scan directory and replay outputs.
- **Cleanup.** Every container is inspected and then removed (§9.3). At the end of
  each attempt, `docker ps -a --filter name=<id>-` lists nothing, and no host
  process the attempt started is left running.

## 14. Deviations

Each of these is a deviation, appended to `$DEMOS1_WORK/DEVIATIONS.md` with:

- the UTC time;
- the attempt;
- what this document says;
- what happened.

The summary gives the deviation counts per kind.

- Any departure from §5 to §13.
- A tool version, engine setting or image digest that differs from §2.
- A product change (§2), with its reason written before any scan uses it.
- A `select` run stopped by a failed search (§5).
- Waits that add up to the bound of §9.3.
- A stage this document does not define.

A deviation never removes an attempt from the report.

## 15. Freeze

1. **The tag.** The tag `prereg-demos-1` is set on the commit that adds this
   document.
2. **The timestamp.** `freeze/prereg-commit.txt` holds that commit's hash.
   `freeze/prereg-commit.tsr` is an RFC 3161 timestamp of that file from
   `http://timestamp.digicert.com`, with its query `freeze/prereg-commit.tsq`.
3. **Later changes.** Any change to this document after the tag is a separate
   pre-registration with its own tag, `prereg-demos-1-a<k>`. Attempts already made
   under this one are still reported.
4. **The tooling.** The verbs of §1 and the image of §2 are built after the tag.
   Before `select` runs, their tests pass on recorded API responses and on two
   local fixture repositories, one pytest and one Go. On the fixtures, the tests
   check that:
   - git in the container accepts the mounted clone;
   - a scan and a per-finding replay run offline;
   - `docker inspect` reports `OOMKilled` for a container that exceeds its memory
     limit;
   - the wrapper reports `memory.peak`.
5. **The run order:**
   1. the tag `demos-1-product`, and the image;
   2. `select`, within one UTC date;
   3. the attempts of §8, pytest first, then Go;
   4. `summarize`;
   5. `report-lint`;
   6. the commit of the summary.

## 16. Dry run of the queries

Date: 2026-10-12. The queries of §6.1 and §7.1 ran over the last days of W, with
the filters of §6.4 applied from API metadata and paths. Counts only:

| Stratum B, ecosystem | Days of W covered | Search hits | Pass filters 1–3 (PRs, repositories) | Pass filters 1–8 (PRs, distinct repositories) |
|---|---|---|---|---|
| Go | 2026-09-01 to 2026-09-30 | 1922 | 514, 31 | 206, 9 |
| pytest | 2026-09-17 to 2026-09-30 | 4765 | 93, 23 | 14, 8 |

- **Per-day searches:** between 5 and 292 items each, none with
  `incomplete_results`.
- **Signal (filter 1):** 4 search items failed it.
- **Stratum A, first page of §7.1 (100 repositories):**
  - `total_count` was 11014 for Python and 3658 for Go;
  - 60 of the Python repositories and 75 of the Go repositories carried a
    permissive licence.
