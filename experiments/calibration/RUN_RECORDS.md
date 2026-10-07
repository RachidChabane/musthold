# Run records and implementation notes

Implementation notes of the harness. Each entry states the rule the harness follows where `PREREG.md` leaves a
detail open. None of them changes a number of the pre-registration. Paths are relative to
`experiments/calibration/`.

## Verbs and front door

- The package is `calibration` (`harness/src/calibration`), with the layers `domain`, `application`, `adapters`,
  `analysis` and `cli`. Import-linter contracts: `domain` imports no other layer; `application` imports only
  `domain`; `adapters` import neither `analysis` nor `cli`; `analysis` imports only `domain`; `domain`,
  `application` and `analysis` import no I/O framework (`subprocess`, `urllib`, `http`, `socket`).
- Each verb runs as `uv run --frozen --project harness python -m calibration.cli <verb>`, as `script/<verb>` and as a
  Makefile target. `snapshot`, `run-session`, `log-deviation`, `derive` and `report-lint` run under `nice`; `build-items`,
  `pin`, `canary`, `run` and `analyze` run under `script/mem-guard run --`. `run-session` launches every model block
  as its own `script/mem-guard run -- <harness> run --model <id>` job with `MEM_GUARD_MIN_FREE_PCT` removed from the
  child environment.
- Exit codes: 0 done, 1 failure, 2 a precondition or an invalid argument, 3 paused (window closed or another heavy
  job), 4 the guard refused to start, 5 the guard stopped the job for memory pressure.
- The nightly window is read from `CALIBRATION_RUN_WINDOW` (default `21:00-07:30`, `always` disables it).
- Tests and development builds compile with `go build -p 1`; the `build-items` verb compiles with `-p 2` (option
  `--go-procs`), as §4.1 states. `-p` does not change the bytes of a binary.

## Inputs and blindness

- `build-items` opens only `bindings.json`, `processing.jsonl`, `differential.jsonl`, `SHA256SUMS` and
  `mutants/<id>.json` of run-02, through a reader that refuses every other path before any I/O. It never names a truth
  or judgment file. The qualification of run-02's oracle records (§2.2) and the count of mutants without an oracle
  record are evaluated by `analyze` and written to `results/decisions.json` (`preconditions.qualification` and
  `excluded.without_oracle`); `excluded.json` of the run directory holds the masker rejections and the
  `witness_unmapped` mutants. `build-items` checks PC1 and PC2 from version control and from the existence of its own
  inputs.
- The pilot analysis commit is the earliest commit that descends from the a1 tag and changes
  `results/decisions.json` to a document holding the run id as a string value.
- PC1 holds when the tag exists, `freeze/prereg-commit.txt` equals the commit the tag peels to, and the message imprint
  of `freeze/prereg-commit.tsr` is the SHA-256 of that file. The certificate chain of the timestamp is not verified.
- The harness commit recorded in the manifest is `HEAD`. `build-items` refuses a working tree whose harness paths
  (`harness/src`, `harness/pyproject.toml`, `harness/uv.lock`, `builder`, `script`) are modified, unless
  `--allow-dirty-harness` is given, which logs a `protocol` entry with scope `all`.
- `analyze` compares the genTime of the tag's timestamp with the pilot analysis commit's committer date and with the
  earliest `started` of run-02's `timing.json`; a started time without a zone is read in local time.
- Shapes of the aggregate pilot files are read in both the list-of-records and the keyed-mapping form.
- The pilot's switches SW2a and SW3 are read from `results/decisions.json` of the pilot analysis commit: a switch has
  fired when a value under a key named after it, or an object named after it, is `true`, the word `FIRED`, or has
  `fired: true` or a `status` that reads `FIRED`. `analyze` records the result in `decisions.json`
  (`pilot_switches`), and `derive` reads it there.

## Builders and items

- The gateways run `shallnot gate --advisory --json-out` on a copy of the snapshot's `fixture/parkfee/`. The gate
  needs a test command, so the copy receives a `shallnot.yaml` (`version: 1`; `specs` and `focus` `spec`; `tests`
  `tests`; `results` `junit.xml`; a test command that runs pytest with `-p no:cacheprovider --timeout=10
  --junitxml=junit.xml`). The copy lies outside the product export and the snapshot, so no exported or snapshot file
  is edited. `pytest` and `pytest-timeout` are runtime dependencies of the harness for this command.
- `build-items` builds the pinned Shallnot with the exported `script/shallnot-build`, with `SHALLNOT_SRC` set to an
  absolute path (`SHALLNOT_SRC`, default `../shallnot-proof-gate`), and refuses to run when `script/shallnot.pin` names
  a commit that source does not hold.
- A test's node id is `<file>::<Classname>::<name>`: the module path is removed from the JUnit classname and the class
  part is left out when empty.
- Masker rejection reasons are the module's constants: `masked_input_leaks` and `unreadable_test_source`.
- Item files hold `closed` as a parsed object and `closed_digest`, the SHA-256 of the verbatim closed text the builder
  rendered; `bodies` maps a model identifier to the verbatim request body as a string. Go escapes `<`, `>` and `&`, so
  bodies and closed texts are never re-serialized. An alignment item also carries `declared`, the binding D(t). A
  screen item carries `witness_status`: `mapped`, `unmapped` or `none`.
- The discpower builder reads one input per mutant (`mutant_id`, `edits`, `witnesses`) and takes the statements from
  the module's Shallnot gateway itself. The question `q<i>` follows the module's criterion order, which sorts the
  criterion identifiers as text (`PARK-1`, `PARK-10`, `PARK-2`, ...); `question_to_ref` records the mapping.
- A witness whose differential value maps to no row of the table of §4.3 is dropped as a whole, and the mutant is
  listed in `excluded.json` under `witness_unmapped`. A null return is the literal type `null` with an empty value.
- The builder check of §16 step 3 is a step of `build-items` (and a pytest): it builds the frozen alignment binary,
  drives it on `s1relay/testdata/alignment/project` to its first `awaiting_host` wave, reads `systemone export`, and
  asks the alignment builder to rebuild every exported closed set. The exported and rebuilt texts must be equal after
  insignificant whitespace is removed. The first wave exports only the closed sets that the module puts to System One;
  tests the product decides without System One are not exported. The result is `builder_check.json`.
- A run directory is built once: `build-items` refuses a directory whose `schedule.json` exists.
- The template digest of a source-(2) key is the SHA-256 of the pilot prompt digest, a NUL byte and the pilot schema
  digest of the votes (`prompts/alignment.md` with `schemas/alignment.tmpl.schema.json`, `prompts/validity.md` with
  `schemas/validity.schema.json`).
- `check-threshold` takes the module default as an argument; the defaults come from the builders' own `defaults` mode
  (`alignment`: `tauRoute`; `discpower`: `tauScreen`), not from a Python literal.

## Requests, records and servers

- A score answer's `probabilities` are keyed by the level index (`"0"` to `"K-1"`) or by the level name; both are
  accepted as the request's levels.
- A reply is server-valid when it passes §4.6. A Kev refusal is STATE_TOO_LARGE only when it is HTTP 422 and its detail
  reads `state is N tokens, over the M-token limit`; any other 422 is a failed attempt.
- A request that fails twice is MISSING and still leaves its record: `request.bin`, `meta.json` and `response.bin`
  (empty after a transport failure), written in that order, so an interrupted request leaves no response file and is
  sent again. `meta.json` holds the keys of §15.1, `prefix_cache_hit` (Kev) and `transport_error`. When an attempt is
  retried, the bytes of the first attempt are kept in `response.attempt1.bin`.
- Kev's prefix-cache outcome of a request is read from the hit counter of `GET /v1/models` before and after the
  request. Ollama's `prompt_eval_cached_count` stays in the stored response.
- The server card is written at the start of a block and extended with the card as it stands once the model is loaded
  (`card_after_load`), because `/api/ps` reports the context length only then. The Ollama model digest is the digest
  `GET /api/tags` lists.
- The canary state-size probe repeats the canary state, separated by a space, until it holds more than four characters
  per token of the context length the server reports once the model is loaded.
- A canary that failed is run again only when the deviation log holds a `protocol` entry naming the model.
- The block checks for other heavy work before it starts and waits in five-minute steps within the window, and checks
  again every 25 requests sent. Its own process group, its `mem-guard` ancestors and its server tree are excluded from
  the check.
- A block that stops (window, heavy work) exits with 3. After every block, whatever its exit code, `run-session` stops
  the model's server through its own cleanup (unload, empty `/api/ps`, port free) and records the result in
  `safety.jsonl`.
- The dynamic fit rule is evaluated by `run-session` before it launches a block; the number of prior deferrals is the
  number of earlier `defer` decisions of the model in `safety.jsonl`.
- `pin` writes the lock once per server family and never rewrites an existing entry; models absent from the lock are
  the only ones it fills. The Kev `base` and `base_revision` are read from the checkpoint's `head.pt`; a null
  `base_revision` is resolved to the current revision of the base repository and pinned.
- A model identifier sent to Kev is echoed by the server; the `<model>` of a Kev `confidenceSource` is the identifier
  (`kev-0.8b`, `kev-4b`, `kev-9b`). The Kev experiment endpoint (`http://127.0.0.1:8008/v1/systemone`) is the
  canonical default address, so its two entries of §12 coincide: `derive` writes one entry with the endpoint
  `experiment and default`.
- A Kev base repository that the checkpoint names without a revision is downloaded from its default branch, so that
  the offline load of `<base>@` resolves through the recorded reference; the pinned sha is the one the Hub reports for
  that branch at pin time.
- A registry manifest digest that differs from the digest `GET /api/tags` lists stops `pin` with an error after the
  bare hexadecimal digests are compared.
- `analyze` appends the `protocol` entries its checks produce (a failed screen regression check) to the run's
  `DEVIATIONS.md` when they are not already there, and applies them in the same run.
- `derive` rewrites `results/REPORT.md` with the proposals in section 6; `analyze` writes it with that section empty.

## Measures

- K1 to K3 clusters are the declared criterion D(t); K4 clusters are `T:<generator call id>` and `P:<n>`.
- Source-(2) agreement shares use the first `k` J-H repetitions; a missing repetition counts against agreement.
- The top-label confidence of K2 to K4 and K1 is the cell's primary confidence; the admission test uses it against
  correctness for every kind. The item set can supply the fit half when the units of the fit-half clusters, fall-throughs
  included, number at least 200 in at least 10 clusters.
- The UNDETERMINED rate of source (3) counts every decision of the cell, the ones without a prediction included, and
  excludes fall-throughs. The VH2 operating point decides a pair when both votes agree and the smaller stated
  confidence reaches the threshold.
- A cell whose model has no recorded answer has every unit counted as MISSING and reads INCONCLUSIVE (data). A cell is
  BLOCKED when its model is blocked in the lock or by a `blocked` entry in scope, or when the pilot's arm records are
  missing (K4).
- CS1 (§12): with no tau kept the entry is 0.00; when some tau is kept but every kept tau is above 0.20 and the volume
  gate does not hold, the rule proposes nothing and the module's default stays.
- Mutants without an oracle record are not items; they are counted per model in `decisions.json`.
- The false-ALIGNED rate is reported at confidence 0.90 for every K3 cell, and over every distinct confidence in
  `results/risk_coverage/`.
- Pass 2 in place of pass 1 appears in `decisions.json` under each S1 cell's `sensitivity.pass2`.
- Statistics are computed with `math.fsum`, content-ordered cluster rows and elementwise accumulation; the bootstrap
  draws whole clusters with `numpy.random.default_rng(20261007)` once per cell, so the statistics of a cell share
  their draws. Floats of a result are rounded to nine decimals at one place.

## Deviation log

- The scopes are `none`, `all` or identifiers of K1 to K4, M1 to M6, JH5, JH3, VH2 and CS1 to CS3. A cell is in the
  scope of an identifier when it names its kind, its model or its source; the S1 cells have no source identifier of
  their own.
- `analyze` and `derive` write no entry other than the `protocol` entries named above.

## Thresholds and counts

- A test labelled MIS_OTHER counts in the K1 and K3 units and not in K2.
- The VH2 population is every measured mutant with an oracle record; a mutant without a V-H record counts as
  `units_without_votes` for each of its pairs.
- A screen pair whose answer document does not answer the question is a `RELAY_REFUSED` fall-through without a
  decision.
- The blindness comparison is strict on both the analysis-commit date and the earliest start of run-02: equal times
  fail it.
- "More than 0.10" fall-throughs is strict: a share of exactly 0.10 does not read INCONCLUSIVE (data). A two-signal
  test whose `none_share` is exactly 0.20 is not off-spec.
- Fewer than 200 fit-half units reads UNREACHABLE when the item set cannot supply them and INCONCLUSIVE when it could.
