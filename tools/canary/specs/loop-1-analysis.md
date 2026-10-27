# loop-1 study tooling: analysis and report lint

The `analyze` and `report-lint` verbs of the loop-1 study
(`experiments/loop-1/PREREG.md`, §6.6, §8 to §12). `analyze` reads the records of
the run phase, computes the measures of each counted run, the decisions and the
endpoints, and writes the committed results; `report-lint` checks the rendered
report. Both read files only and make no network or model call.

## Run phase

- **SP-2580~1**: WHEN the state of a run of the order is derived from its attempts
  THE SYSTEM SHALL call it counted when one attempt has a counted outcome,
  harness-blocked after three harness errors, never count a quota stop toward the
  limit of three attempts, and, once the calendar cap is reached, call a run with
  no attempt not-run, a run whose last attempt was a quota stop quota-blocked and
  a run whose last attempt was a harness error harness-blocked.
- **SP-2581~1**: WHEN the end of the run phase is judged THE SYSTEM SHALL end it
  when every run of the order is counted or blocked, or when 28 days have passed
  since the earliest start of any attempt, and SHALL otherwise name the runs still
  open.
- **SP-2582~1**: WHEN the attempts of the work directory are read THE SYSTEM SHALL
  take every directory named after a run of the order and an attempt number, read
  its `run.json`, order the attempts by number, and ignore a directory whose
  suffix is not a number.
- **SP-2583~1**: WHEN `analyze` starts before the run phase ends THE SYSTEM SHALL
  refuse, naming the open runs, and write nothing.

## Deviation timestamps

- **SP-2584~1**: WHEN the deviation log is checked THE SYSTEM SHALL find, for each
  protocol and token-exposed entry, unshaped lines included, the
  `deviation-timestamps/*.txt` file whose content is the entry's line, and refuse
  when that file or its `.tsr` token is missing.
- **SP-2585~1**: WHEN a deviation token is checked THE SYSTEM SHALL refuse when
  its message imprint is not a SHA-256 or differs from the SHA-256 of the bytes
  of its `.txt` file.
- **SP-2586~1**: WHEN an RFC 3161 token is read THE SYSTEM SHALL take its hash
  algorithm, its message imprint and its time from the text that
  `openssl ts -reply -text` prints.

## Facts of a run

- **SP-2587~1**: WHEN the facts of a counted run are computed THE SYSTEM SHALL
  call its change accepted when `gate-after.json` passes, defective when
  `oracle.json` says so, and escaped when it is both, and SHALL list the
  violated criteria and the flaky oracle tests.
- **SP-2588~1**: WHEN the end-of-turn calls of a run are read THE SYSTEM SHALL
  number Shallnot's `hook claude-stop` records from 1, call one that exits 2 a
  block and one that exits 1 with the hook's give-up message a give-up, and give
  each block one kind from the blocking findings of its `stops/<n>/report.json`:
  a finding block when a proof finding other than `stale_proof` and the pending
  reasons blocks, a pending block when only those proof findings block, and a
  traceability block otherwise.
- **SP-2589~1**: WHEN a run's finding blocks are reported THE SYSTEM SHALL list
  the blocking proof findings of each finding block by code and criterion, and
  count them per code.
- **SP-2590~1**: WHEN the edits of a session are placed between its end-of-turn
  calls THE SYSTEM SHALL take every `Edit`, `Write`, `MultiEdit` and
  `NotebookEdit` tool call of the stream, the subagents' included, place it at
  the run's start plus the arrival time of its stream line, place each hook call
  at its recorded start, and count as a source edit one whose path lies under the
  project's `tests/` directory or is a Python file of the project.
- **SP-2591~1**: WHEN a finding block is judged THE SYSTEM SHALL call it corrected
  when a source edit falls between its call and the next call, a later
  end-of-turn call passes, the change is accepted and no detector has a hit; on
  target when a corrected block's edits touch a test file that cites a criterion
  of its blocking findings; strengthened when the kill share at the passing call
  is above the share at its call, and unknown when either share is missing; and
  left when no later call passes.
- **SP-2592~1**: WHEN the strength of a run's tests is reported THE SYSTEM SHALL
  give the kill share of the final tests, the kill share at the equipped commit
  and the count of final tests that contradict the spec.
- **SP-2593~1**: WHEN the cost of a run is reported THE SYSTEM SHALL give the
  distinct assistant messages of the main agent and of the subagents, apart and
  together, the executor subagents launched, the module `prove` commands, the
  notional `total_cost_usd`, the tokens per model of the `result` event, the wall
  time and the summed duration of the end-of-turn hook calls.
- **SP-2594~1**: WHEN a run's detector hits and voluntary proofs are reported THE
  SYSTEM SHALL count the hits per detector from `detectors.json`, and SHALL list
  the module `prove` commands and `prove-all` skill calls of the session with the
  findings their outputs print.

## Statistics

- **SP-2595~1**: WHEN the stratified permutation test runs THE SYSTEM SHALL
  compute exactly, by convolving each task's hypergeometric distribution with its
  escape total and its arm sizes fixed, `p_less`, `p_more` and `p_min`, the last
  at every task's escapes put in the control arm as far as its counts permit.
- **SP-2596~1**: WHEN H1 is decided THE SYSTEM SHALL keep the tasks with at least
  two counted runs in each arm, decide INCONCLUSIVE `blocked` with fewer than five
  such tasks, hold guard G when the treatment arm's false rejection is at most
  1/3 and it has a run whose change is not defective, and decide SUPPORTED,
  REJECTION-ONLY, CONTRADICTED or INCONCLUSIVE with its reason by the table of
  §10.2 at α = 0.05.
- **SP-2597~1**: WHEN an exact interval is computed THE SYSTEM SHALL give the
  Clopper–Pearson 95 % interval of k successes in n trials by inverting the
  binomial distribution function by bisection, with a lower bound of 0 at k = 0
  and an upper bound of 1 at k = n.
- **SP-2598~1**: WHEN the effect of H1 is reported THE SYSTEM SHALL give each
  arm's escapes over its counted runs with their exact interval, the risk
  difference, and the escapes per arm of each task.
- **SP-2599~1**: WHEN H2 is decided THE SYSTEM SHALL take the treatment runs with
  a finding block, decide INCONCLUSIVE with fewer than six, SUPPORTED when the
  one-sided exact binomial test of a rate above 0.5 gives p ≤ 0.05, NOT SUPPORTED
  when the exact interval's upper bound is below 0.5, and INCONCLUSIVE otherwise,
  and SHALL report the on-target, strengthened and left shares and the correction
  of traceability blocks in both arms.
- **SP-2600~1**: WHEN S2 is tested THE SYSTEM SHALL take the treatment arm's total
  of killed violations, enumerate every assignment of the arm labels to each
  task's counted runs, convolve the per-task distributions exactly, give the
  probability of at least the observed total, and give the exact one-sided sign
  test over tasks on the per-task difference of mean kill shares.
- **SP-2601~1**: WHEN the other secondary endpoints are computed THE SYSTEM SHALL
  give S1 by the test of H1 on the defect indicator, the yield and false
  rejection of each arm with exact intervals, the blocks by kind and by finding
  code per arm, the runs with a hit per detector and arm, the median and maximum
  cost per run and per accepted change with the per-task difference, the
  voluntary proofs of the control arm and the violated criteria per task and arm.
- **SP-2602~1**: WHEN the sensitivity analyses run THE SYSTEM SHALL repeat H1, H2,
  S1 and S2 without `boxoffice`, without the runs with a detector hit and without
  the capped runs, and give the exact sign test over tasks on the per-task
  difference of escape and defect shares and on the per-task majority of
  corrected runs.

## Results

- **SP-2603~1**: WHEN `analyze` has run THE SYSTEM SHALL write `facts.json` in each
  counted attempt's directory and `results/summary.json` holding every run of the
  order with every attempt, its outcome and facts, the state of each run without
  a counted attempt, the decisions, every fraction the report shows, and the
  SHA-256 of `inputs.json` and of every record read.
- **SP-2604~1**: WHEN `REPORT.md` is rendered THE SYSTEM SHALL render it from
  `summary.json` in the order of §11.2, fill the conclusion templates of §11.2
  and quote the limits paragraph of §11.3 verbatim, and end with an empty
  exploratory section for a human to write.
- **SP-2605~1**: WHEN the results are written THE SYSTEM SHALL copy each counted
  attempt's records of §11.1 to `results/runs/<run>/`, the raw records under
  `raw/`, with the run directory, the repository and the home directory written
  as `<run>`, `<repo>` and `~`, render its transcript, copy the deviation log and
  its timestamps, and write `results/SHA256SUMS` over the raw files.

## Report lint

- **SP-2606~1**: WHEN `REPORT.md` is linted THE SYSTEM SHALL skip fenced blocks,
  inline code spans and lines that start with `>`, and fail on the words proves,
  proven, guarantee, guarantees, safe, validated, confirms and works as whole
  words in any case outside a filled conclusion template.
- **SP-2607~1**: WHEN `REPORT.md` is linted THE SYSTEM SHALL fail on a fraction
  `k/n` that `summary.json` does not hold, and on a `%` with no fraction before it
  on its line.
- **SP-2608~1**: WHEN `REPORT.md` is linted THE SYSTEM SHALL fail on a `$` or
  `USD` with no word `notional` on its line.
- **SP-2609~1**: WHEN `REPORT.md` is linted THE SYSTEM SHALL fail on a line that
  holds the text of an entry of the deviation log: its line without the list
  marker, or what it says happened.
