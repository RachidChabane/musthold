# Review of PREREG.md before the freeze

The pre-registration should not be frozen yet. Five problems are blocking, and four of them come from the container amendment. Nothing was edited, cloned or run beyond read-only checks.

**What I checked against the code and live API:**
- **Search queries (§6.1, §7.1):** I ran them read-only and they work as written:
  - the Copilot and Claude author filters return `Copilot`/Bot and `claude[bot]`/Bot;
  - `pull_request.merged_at` is present on items;
  - the Codex query works, and its body check is needed, because one of the two items it returned lacked the URL;
  - the repository search returns HTTP 422 past page 10.
- **Exclusion file (§3):** the digest matches, it holds 48 distinct repositories, and the licence list in §4 matches the code.
- **Scan behaviour:**
  - pytest is detected before Go, using the same strings as §6.4 filter 8;
  - every flag in §9.1 exists;
  - the scan prints "no model used" and "promises not checked";
  - the Go scan uses `go test -json`, not gotestsum;
  - nothing in the product uses cgo, so building for Linux from the Mac works.

## Blocking

**1. §8.2 and §13: the blobless clone breaks change mode offline.** Change mode runs `git diff --unified=0 -M <P> HEAD` (`discpower/internal/adapters/gitsource/gitsource.go`, `ChangedLines`). A `--filter=blob:none` clone checked out at M has no file contents for P, so git tries to download them, which fails under `--network none`. Every B `change` and `change-full` scan would end in `scan-error`.
*Fix:* "The clone is `git clone --no-checkout https://github.com/<o>/<r>.git <dir>` (no blob filter)." Or keep the filter and add a `setup-clone` step that runs `git diff -M P M >/dev/null` while the network is still on.

**2. §8.2 and §13: git runs on the host over a clone that third-party code can write.** The attempt directory, `.git/` included, is mounted read-write. Repository code in the container can set `core.fsmonitor` or `core.hooksPath` in `.git/config`. The next git command the host runs in that clone would then execute it. §8.2 and §8.3 don't say where `git status --porcelain`, `cat-file` and the submodule update run. The recursive submodule update runs on the host.
*Fix (§13):* "Every git command on a clone after `git clone` itself (checkout, submodule update, `cat-file`, `status`) runs in the container. The network is enabled for `setup-clone` with `GIT_ALLOW_PROTOCOL=https` and no credentials. The host runs no git command under `$DEMOS1_WORK/attempts/`."

**3. §13: caches are shared and writable across attempts.** `GOCACHE`, `GOMODCACHE`, the uv cache and `UV_PYTHON_INSTALL_DIR` all live under `$DEMOS1_WORK/cache/`, mounted read-write. `.venv/bin/python` points into the shared CPython install. One repository's install or tests can therefore tamper with the next repository's dependencies, build cache or interpreter.
*Fix:* "`GOPATH`, `GOMODCACHE`, `GOCACHE`, `UV_CACHE_DIR` and `UV_PYTHON_INSTALL_DIR` are under `<attempt>/cache/`. No cache is shared between attempts."

**4. §9.3 and §13: `--rm` makes `scan-oom` impossible to detect.** With `--rm` the container is deleted before its `OOMKilled` state can be read. Also, if the memory limit kills one child test process, the container can keep running, and that killed test reads as a mutant kill.
*Fix:* "Containers run without `--rm` and with `--memory 8g --memory-swap 8g`. After exit, the verb records `docker inspect -f '{{.State.ExitCode}} {{.State.OOMKilled}}'`, then runs `docker rm`. `OOMKilled=true` with any exit status is `scan-oom` (in setup, `setup-oom`)."

**5. §8.4 and §10: `/usr/bin/time -l` is the macOS form.** In a bookworm image it is GNU `time`, which uses `-v`, and the image may not include it at all. §10 also asks for peak memory of setup stages and scans without saying how it is measured.
*Fix:* "Peak memory of every container command is the cgroup's `memory.peak`, read inside the container at exit." Or: "the image installs `time`; commands run under `/usr/bin/time -v`."

## Should-fix

**6. §9.2: replay as written can't give per-finding results.**
- `--root` defaults to the current directory, and replay checks that directory's HEAD against the commit in the finding record.
- One replay of a whole scan folder returns a single exit status, but §10 records one per finding.
- Exit 2 is a tool failure, not a refutation.

*Fix:* "For each finding folder `<n>`: `spec-proof scan replay --root <clone> <moved>/<n>`, in the container, offline. Exit 1 is 'no longer holds on replay'. Any other non-zero exit is 'replay failed', which is not counted as refuted. The shown finding is reported with its replay status."

**7. §9.3: the scan command is still written in host form.**
*Fix:* "`script/with-slot 2 --wait 3600 -- script/mem-guard run -- docker run --name … <image> spec-proof scan …`. Replays use the same wrapper; setup commands use `with-slot 1`. 'Engine refuses' means `docker info` fails or `docker run` exits 125." Also note that host `nice` on the docker client does not reach processes inside the container; `--cpus 2` is the real CPU limit.

**8. §8.1 against §9.3: a memory-guard stop during setup is undefined.** Under "never retried", an interruption would replace a candidate because of the laptop's memory state, not the repository.
*Fix:* "§9.3's refusal and interruption rules apply to setup commands and suite runs. After 3 interruptions the stage is `setup-interrupted`."

**9. §9.3: the waits have no bound.** An 8 GB container plus the 40 % free floor (9.6 GB) on a 24 GB laptop leaves little room, and Docker Desktop's VM rarely gives memory back to macOS. Refusals (exit 4) can then repeat forever.
*Fix:* "The Docker Desktop VM's memory and CPU settings are recorded in the manifest. After 6 h of accumulated refusals, the run stops and logs a deviation. No candidate is replaced for it."

**10. §8.3 and §8.4: the clean-tree check runs only after install.** A suite that rewrites tracked files (golden or snapshot files) leaves a pick that can't be replaced and whose every scan is refused, because the scan requires a clean tree.
*Fix:* "`setup-dirty-tree` is checked again, in the container, after the second suite run."

**11. §8.4: three gaps a second person would fill differently.**
*Fix:*
- "Runtime is measured inside the container (`timeout -s KILL 60`), excluding `docker run` start-up."
- "A first run past 15 min fails stage `suite-first-run-timeout`."
- "A Go passing test is a distinct (`Package`, `Test`) with an `Action: pass` event, subtests included."
- Use one naming scheme for stages: `setup-<stage>` everywhere, or list the full outcome names in §10.

**12. §12.3 against §12.1: the report linter conflicts with the report.**
- The linter's check on fractions would reject the denominators §12.1 requires for setup failures and not-green counts.
- Its banned words would match legitimate paths in plain text, such as `pkg/safe/x_test.go`.
- Its authorship list is too narrow.

*Fix:*
- "Fractions are checked against the set `summarize` emits."
- "Inline code spans are exempt."
- "Not-green is reported as `x/T tests`."
- Add `authored`, `wrote`, `generated by`, `AI-written`, and "by (Copilot|Claude|Codex)" to the banned terms.
- Rename §6 "merged pull requests carrying an agent signal (§6.2)". The Codex signal is only a URL in the body, and a hit in my check was authored by a person (`mikeflynn`).

**13. §2 and §12.1: results can still steer the product and the shown finding.**
*Fix:*
- "The shown finding and headlines always come from `demos-1-product`. Later product tags are reported beside them, with the reason for the change recorded before any re-scan."
- "Between this tag and the last attempt, no third-party repository is cloned or scanned except through `attempt`." This matters because the §16 dry run already surfaced 17 repositories that pass filters 1–8.
- Also in §12.1: "else" should read "the first of these scans, in this order, that exited 0 and has at least one finding."

**14. Header: the departures from the verdict's demo protocol are not stated.** List them:
- publication and the 14-day maintainer step are dropped;
- the suite limits of 2 and 5 minutes become 60 s;
- the "PR body or linked issue" requirement is dropped;
- Vitest/Jest and "self-scan first" are dropped;
- the trailer signals become the app-author signals.

**15. §5 against §7.1: the split rule can't apply to the repository search.** §7.1's query matches 11 014 Python and 3 658 Go repositories, but it has no date range to split, and the API stops at page 10.
*Fix:* "§7.1 is exempt from splitting: pages 1–10 are all fetched, then sorted and filtered." Fetching all ten pages also removes the ambiguity of "as needed".
Also add: "A non-2xx response after rate-limit retries fails the filter that issued it, recorded with its status."

## Minor

- **§12.2:** checking once that the export lacks `experiments/demos-1/` does not stop a later export from copying it. Add the directory to the export's exclusion rules, and have `summarize` check the rule exists.
- **§13 container names:** use `<id>-<stage>-r<k>` so reruns don't collide.
- **§2 binaries:** build both with `-ldflags "-X main.version=<commit>"`. Otherwise `--version` prints a development placeholder (or an empty version) instead of the commit.
- **§2 image:** take uv 0.11.23 from an image pinned by digest. Say the two binaries are mounted in the same directory, since `spec-proof` looks for `spec-proof-dp` beside itself first.
- **§9.3 reruns:** each rerun gets fresh `XDG_*` and `TMPDIR` directories.
- **§15.4 fixtures:** check on the fixtures that git inside the container accepts the mounted clone. If the container user's uid differs from the host's, git refuses it as "dubious ownership". If so, set `safe.directory` in `<attempt>/home/.gitconfig`.
- **§5:** say what happens to a one-second search interval that stays incomplete after its retries.