# Pre-registration: confidence calibration per source (limit 4)

Measurement of limit 4 of the proof chain for each confidence source of the System
One chain of ADR 0005: (1) a `/v1/systemone` endpoint, (2) k blind votes of the host
model, whose confidence is the share of agreement, and (3) UNDETERMINED below the
threshold. Source (4), `host-classify`, answers a closed question set from the
host's own classify call, and a decider comparator answers the same sets by blind
votes. A closed answer carries a confidence whose relation to correctness is unknown.
Per decision kind, per source and per model, the experiment asks:

- whether a confidence of 0.9 is right about 9 times out of 10;
- how much of the decision stream a source answers at 5 % error or less;
- how often the chain ends UNDETERMINED;
- whether ADR 0003's admission test, keyed by `(kind, confidenceSource, model)` as
  ADR 0005 keys it, can be met;
- which per-source entries the threshold keys `tauScreen` (discpower) and
  `tauRoute` (alignment) take on this evidence.

Source (1) is measured by offline replay of closed-question requests through local
models of at most 9B parameters. The requests are produced by the product's own
builders and rendering and archived before any model call. Source (2) is measured on
host votes that the amended limits 1–3 pilot recorded in its run `run-02`, and no
host model is called. Source (3) is derived from them. Source (4), `host-classify`,
and the decider subagents are measured by remote calls under §17. Sources (1) to (3)
call no remote model.

Items are decisions on the pilot's artefacts: one author-built fixture, `parkfee`,
with 10 criteria, 38 tests and one Python domain. Each item is paired with truth that
exists without any model: author labels verified by execution, and executed oracle
classes. No human labels any item.

Claim power is outside this experiment. Whatever its outcome, a System One answer
keeps the powers the claim-power table of ADR 0005 grants it: pool membership
through the screen, counted in `screenExclusions`, and ALIGNED only jointly with a
SPECIFIC or OVERBROAD matrix. It never blocks and never grants pass credit alone. A
host-vote judgment never blocks alone. §12 states what an outcome can and cannot
change.

Freeze: §16.

## 1. Layout and front door

Root: `experiments/calibration/`.

| Path | Content |
|---|---|
| `PREREG.md` | this document |
| `prompts/canary.json` | the server canary question set (§5.4), in the host protocol's `closed` form |
| `builder/` | Go sources of the request builders and of the reply decoder (§4.1) |
| `harness/` | uv project, package `calibration` with `domain`, `application`, `adapters`, `analysis`, `cli` |
| `script/` | shebang'd entry points, one per verb |
| `Makefile` | thin targets over `script/<verb>` |
| `freeze/` | the frozen commit hash and its RFC 3161 timestamp (§16) |
| `.cache/` | the pilot snapshot, the product export and the built builders (§2, §4.1); excluded from git |
| `runs/<run-id>/` | pilot input digests, items, schedule, raw HTTP records, safety log, manifest, deviations |
| `results/` | decisions, tables, reliability data, policy inputs, report |

Python 3.13.14. Go is the toolchain the frozen commit's `go.work` names. Each verb
runs as `uv run --frozen --project harness python -m calibration.cli <verb>`, as
`script/<verb>` and as a Makefile target. The verbs are `snapshot`, `build-items`,
`pin`, `canary`, `run`, `run-session`, `log-deviation`, `analyze`, `derive` and
`report-lint`. `script/run-session` is the nightly entry point (§6.5).

Ground-truth isolation:

- **Truth and judgment files:**
  - the snapshot's `prereg/labels.yaml`, `prereg/arm_p/labels.yaml` and
    `prereg/arm_p/items/planted_map.json`;
  - run-02's `oracle_classes.json`, `relabel.jsonl`, `kills.jsonl`,
    `mutant_classes.json`, `alignment.jsonl`, `alignment_aggregate.json` and
    `validity.jsonl`.

  Only `analyze` and `derive` read them.
- **What `build-items` reads from run-02:** `bindings.json`, `processing.jsonl`,
  `differential.jsonl`, and the `arm` and `normalized_edits` fields of
  `mutants/<mutant_id>.json`.
- `pin`, `canary`, `run` and `run-session` read no pilot file.
- A harness test asserts that no module reachable from `build-items`, `pin`,
  `canary`, `run` or `run-session` names a truth or judgment file.
- An import-linter contract forbids `domain`, `application` and `adapters` from
  importing `analysis`.

The harness posts the request bytes that `build-items` archived to the server with
the Python standard library, and records the bytes sent and received verbatim.
`analyze` makes no network call and recomputes every figure in `results/` from those
bytes.

## 2. Inputs

### 2.1 Product source

The product is the tree of the commit tagged `prereg-calibration` (§16). The
experiment uses from it:

- the modules `format`, `alignment`, `discpower` and `s1relay`, resolved through the
  tree's own `go.work`;
- `script/shallnot.pin`, `script/shallnot-build` and `script/mem-guard`.

`build-items` exports the whole tree except `experiments/`, with
`git archive prereg-calibration -- . ':(exclude)experiments'`, into
`.cache/product-<commit>/`. It records the SHA-256 of every exported file. No
exported file is edited. The builders are added beside the module code (§4.1).
§17's plans and tickets come from the tree tagged `prereg-calibration-a1`.

### 2.2 Pilot freeze and pilot run

- **Pilot freeze:** the commit tagged `prereg-limits-1-3-a1`, equal to the hash in
  `experiments/limits-1-3/freeze/a1-commit.txt`.
- **Pilot run:** `run-02`, the decisional run of the amended pilot (ADR 0006). It
  qualifies when both of these hold:
  - `experiments/limits-1-3/results/decisions.json` names `run-02` in a commit that
    descends from the a1 tag (the pilot analysis commit);
  - its `oracle_classes.json` holds a record for every non-stillborn, status-OK
    mutant of arms T and P.
- **No writes:** nothing under `experiments/limits-1-3/` is written by this
  experiment.

### 2.3 Blindness

- The tag `prereg-calibration`, with this document, `prompts/` and the product
  source, exists before the pilot analysis commit. `analyze` checks two dates: the
  tag's timestamp genTime must precede the pilot analysis commit's committer date, and
  it must precede the earliest `started` in `timing.json`.
- No file under `experiments/limits-1-3/runs/` is opened for this experiment before
  the pilot analysis commit exists. `build-items` refuses to start unless PC1 and
  PC2 hold (§14.1).
- This experiment writes nothing that the pilot reads.
- Every run-02 file read is recorded in `runs/<run-id>/pilot_inputs.json` with its
  SHA-256. When `runs/run-02/SHA256SUMS` exists, each digest must match it.

### 2.4 Pilot snapshot

`snapshot` runs `git archive prereg-limits-1-3-a1` over
`experiments/limits-1-3/fixture`, `experiments/limits-1-3/prereg` and
`experiments/limits-1-3/harness/src/pilot`. It unpacks the result into
`.cache/pilot-<commit>/` and records the SHA-256 of every extracted file in
`runs/<run-id>/snapshot.json`.

Item construction imports only these `pilot.domain` modules from the snapshot, with
their own `pilot.domain` imports: `normalization`, `definitions`, `ids`, `edits` and
`mutants`.
`analyze` also imports `pilot.domain.matrix`. The working tree of
`experiments/limits-1-3/` is never imported, built or synced.

## 3. Sources, kinds and items

### 3.1 Confidence sources

| Id | Chain step | `confidenceSource` | Data | Model calls |
|---|---|---|---|---|
| S1:M1 … S1:M6 | (1) endpoint | `system-one:<model>@<endpointDigest>` | replay of the archived requests (§4) through M1 to M6 (§5) | local only |
| JH5 | (2) host votes | `host-votes(5)` | the pilot's J-H NAME-MASKED repetitions 1 to 5 (`alignment.jsonl`) | none |
| JH3 | (2) host votes | `host-votes(3)` | the same repetitions 1 to 3 | none |
| VH2 | (2) host votes | `host-votes(2)` | the pilot's V-H1 and V-H2 votes (`validity.jsonl`) | none |
| S3 | (3) UNDETERMINED | derived per source and threshold (§7.3) | the above | none |
| HC-H, HC-S, HC-P | (1b) host classify, source (4) | `host-classify(5,rot1):<voters>` | §17 | remote |
| HV-S, HV-O | (2) host votes | `host-votes(3)` | §17.3 | remote |

A source-(1) string is the one the product records for a relayed answer:
`system-one:<model>@<endpointDigest>`, built by `format.SystemOneEndpointSource`
and carried by the relay's `--source` flag, which the module parses with
`format.ParseSystemOneSource` and records unchanged. Here `<model>` is the model
the reply names, and `<endpointDigest>` is the SHA-256, in hex, of the endpoint's
canonical address: scheme and host in lower case, the path cleaned, and no
credentials, query or fragment (s1relay SP-603). The digest of each endpoint the
experiment uses is recorded in the manifest.

The host votes of JH5, JH3 and VH2 were cast by the pilot's host model,
`claude-sonnet-5-5`.

### 3.2 Decision kinds

| Id | Decision | Product question | Unit | Truth | Sources |
|---|---|---|---|---|---|
| K1 | which criterion a test's assertions verify | alignment `decide.closed`, question `criterion`: `choice` over the ten criterion tokens in tokenized order, then `NONE` | test | author labels | S1, JH5, JH3, HC-*, HV-* |
| K2 | whether the coverage of that criterion is adequate | alignment `decide.closed`, question `coverage`: `score` over NONE, WEAK, PARTIAL, FULL, lowest first | test | author labels | S1, JH5, JH3, HC-*, HV-* |
| K3 | ALIGNED to the declared criterion | derived from the K1 and K2 answers by the alignment module's rule (§7) | test | author labels | S1, JH5, JH3, HC-*, HV-* |
| K4 | mutant m breaks criterion c | discpower `decide.closed` screen, question `q<i>` for the criterion at window position i: `choice` VIOLATES, CONSISTENT | (m, c) pair | oracle classes | S1, VH2, HC-*, HV-* |

K3 and K4 are the primary kinds, because `tauRoute` and `tauScreen` act on them. K1
and K2 are secondary.

The S1 sources answer the product's closed question sets. The source-(2) votes
answered other questions: J-H answered the pilot's alignment question
(`prereg/prompts/alignment.md`) on the pilot's masked input, and V-H1 and V-H2
answered the pilot's validity question (`prereg/prompts/validity.md`). Their figures
measure agreement among blind host votes on the same decision, under the prompt
digests recorded in §15.1, not under the product's closed question sets.

### 3.3 Alignment items (K1–K3)

**Item set.** The items are the pilot tests that the alignment builder can show
blind (§4.2). A test that the module's masker rejects is not an item. It is listed
with the module's reason (`masked_input_leaks` or `unreadable_source`), and the
alignment module decides such a test UNDETERMINED for that reason.

**Truth sets.**

- **PRIMARY:** the author labels of `labels.yaml`, which passed execution
  verification (pilot §3.2).
- **CONSENSUS:** the tests on which the author, R-OAI and R-AUTHOR agree on the
  six-class label (pilot §3.4, read from run-02's `relabel.jsonl`).

K1 truth comes from the label's `checks`:

| Label | Correct answer | Lenient variant |
|---|---|---|
| GOOD(A) | A | A |
| GOOD_BROAD(A, B) | A (`checks[0]`) | A or B |
| WEAK(A) | A | A |
| MIS_OTHER(A→B) | B | B |
| VACUOUS | NONE or the declared criterion D(t) | NONE or D(t) |
| OFFSPEC(A) | NONE or A | NONE or A |

For VACUOUS and OFFSPEC tests, both NONE and the declared criterion are correct. The
coverage definitions name mocks, tautologies and expected values that cannot be
derived from the requirement as WEAK coverage of a verified criterion. "D(t) with
WEAK coverage" is therefore a correct reading of these tests, as is NONE.

**K2 truth** is binary:

- ADEQUATE (FULL or PARTIAL) for GOOD and GOOD_BROAD;
- INADEQUATE (WEAK or NONE) for WEAK, VACUOUS and OFFSPEC.

MIS_OTHER tests are not K2 items. The codebook does not fix how completely a
MIS_OTHER test checks B, so no coverage level for B follows from its label.

**K3 truth** is `alignment_truth`: ALIGNED for GOOD and GOOD_BROAD, NOT_ALIGNED
otherwise.

**Declared criterion.** D(t) is the Shallnot binding the alignment builder reads
(§4.2). PC3 requires it to equal run-02's `bindings.json`, and `analyze` checks that
it equals the label's `cites`. D(t) never enters a request.

### 3.4 Mutant items (K4)

**Population.** Every mutant of arms T and P that is non-stillborn with status OK in
run-02's `processing.jsonl`:

- DUPLICATE and TRIVIALLY_EQUIVALENT mutants are not items;
- a mutant shared across arms is one item under its earliest arm (pilot §6.2);
- each item yields 10 decisions, one per criterion PARK-1 to PARK-10.

**Truth.** For c = PARK-n~1, the pair (m, c) is VIOLATES when n is in m's
`failed_criteria` in `oracle_classes.json`, and CONSISTENT otherwise. A mutant
without an oracle record is not an item and is counted.

**Strata** (`analyze` only):

- arm: T or P;
- F3 status from `differential.jsonl`: DIFFERS, NO_DIFF_FOUND or SKIPPED;
- whether c is m's declared target (`mutants/<mutant_id>.json`, `target`);
- kill status for c: whether a test bound to c in `bindings.json` has a reproduced
  kill of m in `kills.jsonl`. A flaky result counts as a survival, and an error is
  not a kill.

**Screen stratum.** The pairs (m, c) where m survives every test bound to c and m's
F3 status is not NO_DIFF_FOUND. These are the pairs that discpower's screen asks
about for a survivor of c.

### 3.5 Not measured

- **Arm O operator mutants**, which form the product's screen population. Arms X,
  R and L are not measured either. On run-02 these arms have no F3 witness for the
  screen state and no host votes.
- **Completeness `decide.closed` questions, diff justification and testability.**
  They have no executed truth on the pilot.

## 4. Requests (source 1)

### 4.1 Builders

**Packages.** `builder/` holds three Go `main` packages:

- `alignment/`, copied to `alignment/cmd/calibration-build/`;
- `discpower/`, copied to `discpower/cmd/calibration-build/`. Besides building
  requests (§4.3), it has two more modes. `read` applies `prompt.ParseScreen` and
  `power.ViolationProbability` to an answer document. `check-threshold` checks a
  threshold entry (§12): it reads the entry as a `format.ThresholdOverride` from
  YAML, applies it with `ApplyTo` to the module's default threshold, and prints
  `For(source)` for each source it names;
- `s1relay/`, copied to `s1relay/cmd/calibration-decode/` (§4.6).

**Build.** `build-items` copies each package into the product export at the path
above. These are the only places from which a module's `internal` packages may be
imported. It builds them with `go build -p 2 -trimpath` under `script/mem-guard run`
(§6.1). Each builder imports only packages of its own module and of `format`. The
manifest records the SHA-256 of every builder source file and binary.

**Rendering.** Every request body is
`hostproto.ParseClosedSet(closed).SystemOneRequest(model)` of the frozen
`format/hostproto`. These are the bytes the relay posts for that question set: its
own rendering, `ClosedSetOf` (s1relay SP-604), reaches the same function. Per item,
the builder writes:

- the `closed` member;
- the module's `decide.closed` template digest;
- one body per model identifier of §5.1.

### 4.2 Alignment requests

The alignment builder runs once over the fixture and yields one request per test. It
works on a copy of the snapshot's `fixture/parkfee/`, in four steps:

1. **Criteria and bound tests.** They come from the module's Shallnot gateway
   (`alignment/internal/adapters/shallnot`), which runs
   `shallnot gate --advisory --json-out` on the copy. The `shallnot` binary is built
   by the frozen `script/shallnot-build` at the commit of `script/shallnot.pin`.
2. **Masked input.** The module's Python masker
   (`alignment/internal/adapters/masking/python`) masks the test, and
   `masking.Ensure` checks the unit and the signatures, as `prove` builds an input.
   A rejected test is recorded with its reason and is not an item (§3.3).
3. **Question set.**
   - `masking.Tokenize(criteria, test.Label())`;
   - `tournament.Groups` over the tokens. Ten criteria form one group, which is the
     final round;
   - `prompt.BuildClosed(ClosedInput{Test, Group, Final: true})`.
4. **Output:** the node id, the label, the binding D(t), the token map (`Refs`), the
   closed set, the template digest and the bodies.

The product sends one request per test, so no other presentation is built:

- the questions are `criterion`, then `coverage`;
- the options and levels are in the order the module builds them: tokens in
  tokenized order and NONE last, and the coverage levels from NONE to FULL.

### 4.3 Screen requests

The discpower builder builds one request per mutant item. It reads one JSON input per
mutant, which `build-items` writes from the snapshot and run-02:

- **`statements`.** The blind statements, `prompt.Scrub(statement, refs)`, of the
  ten criteria in the module's criterion order. They come from the module's Shallnot
  gateway (`discpower/internal/adapters/shallnot`) on the same copy of the fixture.
  The window is the ten criteria for every mutant, because discpower computes a
  mutant's scope only for its own operator mutants.
- **`edits`.** One entry `{file, qualname, before, after}` per entry of the
  mutant's `normalized_edits`:
  - `file` and `qualname` are as recorded. `file` is relative to `fixture/parkfee/`,
    the project root;
  - `before` is `normalize(definition_source(source, file, qualname))` on the
    snapshot's original source of `file`;
  - `after` is the same expression on that source after
    `apply_edit_set(sources, normalized_edits)`.

  `normalize` is `pilot.domain.normalization.normalize` (comments and docstrings
  removed, `ast.unparse`), and `definition_source` and `apply_edit_set` come from
  `pilot.domain.definitions`. Both sides are therefore the normalized definitions the
  V-H votes saw. The builder orders the entries with `mutation.EditSet.Canonical()`.
- **`witnesses`.** One witness when the mutant's F3 status is DIFFERS, and none
  otherwise. It is mapped as below.

The builder calls `prompt.BuildScreen(ScreenInput{Statements, Edits, Witnesses})`.
Question `q<i>` asks about the criterion at position i of the window.

| Pilot F3 record | Product witness value |
|---|---|
| `function` | `spec.Call.Operation` |
| an argument whose parameter is annotated `datetime` (an ISO 8601 string) | `datetime` literal, the recorded string |
| an argument annotated `Decimal` (a decimal string) | `decimal` literal, the recorded string |
| an argument annotated `bool` (a JSON boolean) | `boolean` literal, `true` or `false` |
| `original_out` or `mutant_out` `{ok: ["decimal", t]}` | `returns`, `decimal`, t in positional notation (`format(Decimal(t), "f")`) |
| `{ok: ["str", t]}` | `returns`, `string`, t |
| `{ok: ["repr", "None"]}` | `returns`, `null` |
| `{ok: ["repr", "True"]}` or `{ok: ["repr", "False"]}` | `returns`, `boolean`, `true` or `false` |
| `{ok: ["repr", t]}` with t an integer literal | `returns`, `integer`, t |
| `{exc: name}` | `raises`, name |

Parameter annotations are read with `ast` from the snapshot's
`fixture/parkfee/src/parkfee/`; the pilot's differential draws arguments only for
the annotations `datetime`, `Decimal` and `bool`. A value that no row maps gives a
request with no witness, and the mutant is counted as `witness_unmapped`.

### 4.4 Build checks

`build-items` aborts on any failure of PC3 (§14.1). Its records are the items, the
masker rejections, the `witness_unmapped` count and the template digests.

### 4.5 Schedule and passes

- `build-items` writes every item to `runs/<run-id>/items/`, with the digest of each
  body, and writes the schedule to `runs/<run-id>/schedule.json`. No model is called
  before the archive is complete.
- Request ids are `A|<nodeid-slug>` and `M|<mutant_id>`.
- **Pass 1** sends every request in ascending order of
  `sha256("20261007|" + request_id)`. The order is the same for every model.
- **Pass 2** sends every alignment request again, together with every mutant request
  for which `int(sha256("20261007|pass2|" + request_id)[:8], 16) mod 4 = 0`. It sends
  them in the reverse of their pass-1 order, and starts once pass 1 of the same model
  is complete.
- Pass-1 answers are the primary data. Pass-2 answers serve only determinism (§8.6)
  and one sensitivity analysis.

### 4.6 Answers, errors, state size

**Server validity.** An answer is server-valid when:

- the response is HTTP 200 and every question is answered with its type;
- the `probabilities` of a choice or a score have exactly the request's options or
  levels as keys, are finite and sum to within 0.02 of 1.

An answer that is not server-valid, or a transport failure, is retried once after
5 s. A request that fails twice is MISSING.

**State size.** A request is STATE_TOO_LARGE when the server answers in one of these
forms:

- **Ollama:** HTTP 400 with the message
  `prompt <i> has <n> tokens; expected 1–<m> (input is never truncated)` or
  `prompt <i> requires <n> context tokens for scoring; model has <m>`, or HTTP 413
  for body size;
- **Kev:** HTTP 422 for state length, with `KEV_TRUNCATE_STATES` unset.

A STATE_TOO_LARGE request is not retried.

**Product reading.** `analyze` reads each recorded reply the way the product would,
in the order the product does:

1. **Relay checks.** `calibration-decode` reads the closed set with
   `systemonewire.DecodeClosed`, decodes the reply with `systemonewire.DecodeReply`,
   and checks the answers with `question.Set.Check`, as `drain` does before an
   import (s1relay SP-605, SP-606). An answer the relay refuses is RELAY_REFUSED.
2. **Module import.** For an accepted reply, `calibration-decode` builds the answer
   document that `systemone import` stores with
   `hostproto.ClosedSet.AnswersFromSystemOne`. A choice keeps its option, its
   `confidence` and its `probabilities` within [0,1]. A score becomes the level
   nearest to the returned score, with the `confidence`. A refusal at this step is
   RELAY_REFUSED.
3. **Module reading.** The answer document is read with `hostproto.ParseClosedAnswers`.
   For the screen, the discpower builder's `read` mode applies `prompt.ParseScreen`
   and `power.ViolationProbability` (§7.1).

**Fall-throughs.** MISSING, STATE_TOO_LARGE and RELAY_REFUSED decisions are
fall-throughs: in the product's chain, the request goes on to the host votes. They
are excluded from every source-(1) metric and counted per (kind, model). They are
reported as the fall-through rate, never as UNDETERMINED.

At most one request is in flight. The request timeout is 300 s.

## 5. Models and servers (source 1)

### 5.1 Models

| Id | Model | Server | Identifier sent |
|---|---|---|---|
| M1 | nimble 9B | Ollama | `nimble:9b` |
| M2 | tev1 4B | Ollama | `tev1:4b` |
| M3 | tev1 0.8B | Ollama | `tev1:0.8b` |
| M4 | Kev 0.8B | Kev | `kev-0.8b`, server started with `--run jaredpalmer/kev-0.8b@<sha>` |
| M5 | Kev 4B | Kev | `kev-4b`, `--run jaredpalmer/kev-4b@<sha>` |
| M6 | Kev 9B | Kev | `kev-9b`, `--run jaredpalmer/kev-9b@<sha>` |

Each Ollama identifier is the library's default tag of that size, pinned by manifest
digest. Each Kev checkpoint is served at its stored precision. No other
quantization, GGUF or format replaces a model, and no model above 9B parameters is
loaded. A model that cannot be served under §6 is BLOCKED.

### 5.2 Servers

**Ollama 0.35.1.** The block itself starts the server (§6.2).

**Kev.** The repository `https://github.com/jaredpalmer/kev` at commit
`5e42a7a03f28134853dd3ff77461457e921e5ec1`, set up with
`uv sync --frozen --extra serve`. The block itself starts the server (§6.3) as:

`uv run --frozen --extra serve python -m kev.serve --run <repo>@<sha> --host 127.0.0.1 --port 8008`

- `HF_HUB_OFFLINE=1` is set once `pin` has downloaded the revisions.
- `KEV_TEMPERATURE`, `KEV_DTYPE`, `KEV_TRUNCATE_STATES` and `KEV_API_KEY` are unset.
- The backend is the one the server selects on Apple Silicon (MLX).

Kev echoes the request's `model` string, so a Kev block's provenance comes from
`GET /v1/models`, never from the response.

### 5.3 Pins and provenance

`pin` runs before any item request and writes `runs/<run-id>/models.lock.json`. It
never rewrites the file.

- **Ollama:** the version, plus for each tag:
  - the manifest digest and size;
  - the `/api/show` details (family, parameter size, quantization) and parameters
    (`num_ctx`, `temperature`).
- **Kev:**
  - the repository commit and the SHA-256 of its `uv.lock`;
  - the Hub revision sha of each `jaredpalmer/kev-*` repository, and the `base` and
    `base_revision` that the adapter names;
  - the byte size of every weight file of the adapter, its head and its base at
    those revisions;
  - the versions of `mlx`, `mlx-lm` and `torch` in the Kev environment.
- **Memory:** total physical memory (`hw.memsize`).

`pin` reads every size from registry or Hub metadata before any download. It applies
the static memory rule of §6.4 before downloading anything, and downloads only the
models that the rule admits.

**Server card.** At the start of each model block the harness records:

- **Ollama:** `/api/version`, `/api/show`, `/api/ps` (which gives the loaded context
  length), and the environment it passed to the server;
- **Kev:** `GET /v1/models` (run, base, LoRA, device, backend, dtype, temperature,
  prefix-cache settings).

Every raw record stores the server version and model digest of its block.

### 5.4 Canary

`canary` runs once per model, at the start of its first block:

- **Request.** It renders `prompts/canary.json`, which holds one `choice` and one
  `score` question, with `SystemOneRequest`, sends it, and checks:
  - every rule of §4.6;
  - that `calibration-decode` accepts the reply;
  - that the answer document keeps, for the choice question, the server's
    `probabilities` and `confidence`.
- **Confidence formulas.** It recomputes each server's `confidence` from the
  returned probabilities:
  - Ollama, `choice` and `score`: 1 − H(p)/ln K;
  - Kev, `choice`: (p_max − 1/K)/(1 − 1/K);
  - Kev, `score`: max(0, 1 − Σ pᵢ|i − mode| / D), with D the mean absolute deviation
    of the uniform distribution over the levels.

  The recomputed values must lie within 0.001 of the returned ones.
- **State-size probe.** It sends one request whose state is the canary state repeated
  until it holds more than 4 characters per token of the loaded context length
  (Ollama) or of the server's `max_state_tokens` (Kev). The server must answer in a
  STATE_TOO_LARGE form of §4.6, and an HTTP 200 answer fails the canary.
- **Failure.** A failure stops that model's block. A fix is a `protocol` entry.
- `analyze` runs the formula check on every answer and reports it.

## 6. Machine safety and pacing

The machine is one 24 GB laptop that is also its owner's daily workstation. The
rules below implement ADR 0005, "Machine safety", for this experiment. When a rule
stops or defers work, that is a `pause` or a `blocked` entry, never a reason to relax
the rule.

### 6.1 One guarded job

- **Model blocks.** A model block is one invocation of
  `script/mem-guard run -- <harness> run --model <id>`. The guard:
  - refuses to start below 40 % system-wide free memory, or when the pressure level
    is not normal (`MEM_GUARD_MIN_FREE_PCT` stays unset);
  - runs the block under `nice -n 10`;
  - stops the whole process group, server included, when the memory pressure leaves
    normal.
- **One model at a time.** The block starts its own server inside that process
  group, so a guard stop also unloads the model. Calls are serial, and exactly one
  model is resident while a block runs.
- **Builds and analysis.** Go builds (`go build -p 2`), the Shallnot build and
  `analyze` also run under `script/mem-guard run`, as a single process each.
- **Other heavy work.** A block starts only when no other heavy local work runs:
  - no process matches `ollama runner`, `kev.serve`, `llama-server` or `mlx_lm`
    other than the block's own;
  - the installed Ollama service, when it answers on `127.0.0.1:11434`, lists no
    model in `/api/ps`;
  - no other `script/mem-guard run` process exists, which covers the pilot's heavy
    verbs.

  The harness checks these conditions before the block and again every 25 requests.
  A failed check before the block makes it wait in 5-minute steps within the window.
  A failed check during the block stops it cleanly with a `pause` entry. This keeps
  the machine at no more than 2 heavy workers and never runs two local-model jobs at
  once.
- **Remote models.** Model blocks call no remote model. §17's calls are not heavy work
  and are not bound by §6.5.

### 6.2 Ollama server

- **Start.** The block starts `ollama serve` with:
  - `OLLAMA_HOST=127.0.0.1:11435`;
  - `OLLAMA_MAX_LOADED_MODELS=1`;
  - `OLLAMA_NUM_PARALLEL=1`;
  - `OLLAMA_KEEP_ALIVE=2m`.

  The server card records this environment. The request bodies carry no
  `keep_alive` member, as the product's bodies do not, so a paused or crashed block
  frees the model after at most 2 minutes.
- **Before the block:** `/api/ps` lists no model.
- **After the block, and after any stop:** the harness unloads the model with
  `POST /api/generate {"model": <tag>, "keep_alive": 0}`, checks that `/api/ps` is
  empty, stops the server and checks that port 11435 is free.

### 6.3 Kev server

A Kev block starts its server inside the guarded process group, and stopping the
server unloads the model. At the end of the block, and after any stop, the server is
stopped and port 8008 must be free. Kev blocks run after every Ollama block, and the
experiment's Ollama server is never running during a Kev block.

### 6.4 Memory fit

The footprint F of a model is:

- Ollama: the tag's manifest size plus 1 GiB;
- Kev: the bytes of the base, adapter and head weight files at the pinned revisions,
  plus 1 GiB.

T is the total physical memory.

- **Static rule (at `pin`).** A model with F > 0.5 T is BLOCKED with the reason
  `memory-fit`. It is never downloaded and never loaded. On a 24 GB machine this
  applies to Kev 9B at its stored precision whenever its base weights exceed
  11 GiB.
- **Dynamic rule (before each block).** The block loads its model only when the free
  memory, read as the guard's free percentage times T, is at least F + 0.15 T.
  Otherwise the block is deferred to the next session. A model whose block has been
  deferred in 5 sessions is BLOCKED (`memory-fit`).
- **Stops.** A second memory-pressure stop of the same model, or a failure to load
  it, makes that model BLOCKED (`memory-pressure`).
- **Rationale.** The 0.15 T reserve keeps memory free for the operating system and
  the owner's resident applications, so that the load itself cannot push the machine
  into swap before the guard's next poll.

Every check (guard reading, F, free bytes, decision) is appended to
`runs/<run-id>/safety.jsonl`.

### 6.5 Window and sessions

- **Window.** Model blocks run only inside a nightly window, 21:00 to 07:30 local
  time. No local model is loaded outside it.
  - A block starts only inside the window.
  - Before each request it checks the window.
  - At the close it finishes the request in flight, unloads, stops its server, logs
    `pause | none | run window closed` and exits with code 3.
  - A later session resumes it, and a request whose response file exists is not
    resent.
- **Rationale.** A resident model of up to 10 GB, during inference, competes for
  unified memory and memory bandwidth, which `nice` does not limit. The daytime cap is
  therefore zero wall time. The experiment has no deadline, and the window is the one
  the pilot already uses for model work.
- **Session.** `script/run-session` is one session:
  - it takes an exclusive lock on `runs/<run-id>/.session.lock`;
  - it runs the next unfinished model blocks in the order M3, M2, M1, M4, M5, M6,
    each block holding that model's canary (first block only), pass 1 and pass 2;
  - it stops at the window's close.
- `snapshot`, `build-items`, `pin`, `analyze`, `derive` and `report-lint` load no
  model and may run outside the window, under §6.1.

## 7. Confidence measures

### 7.1 Source (1): the product's reading

The primary measure is the quantity the product compares with its threshold. It is
computed from the answer document of §4.6:

| Kind | Prediction | Confidence c (primary) | `confidenceMapping` | Positive-event probability |
|---|---|---|---|---|
| K1 | the option the relay maps the `criterion` answer to | its `confidence`; an absent one counts as 0 | `system-one.choice.confidence` | none; the event is "the prediction is correct" |
| K2 | ADEQUATE when the level the relay maps the `coverage` score to is FULL or PARTIAL | the score's `confidence`; an absent one counts as 0 | `system-one.score.confidence` | c when ADEQUATE is predicted, 1 − c otherwise |
| K3 | ALIGNED when the K1 prediction is D(t) and the K2 prediction is ADEQUATE (`subjectRun.systemOne`) | min(K1 confidence, K2 confidence), an absent one counting as 0 (`relayedAnswer`) | `system-one.choice.confidence`, the mapping the alignment module records for a relayed answer (`confidenceSource`) | c when ALIGNED is predicted, 1 − c otherwise |
| K4 | the option the relay maps the `q<i>` answer to | P(VIOLATES) when VIOLATES is predicted, 1 − P(VIOLATES) otherwise | none: the screen reads probabilities, not `confidence` | P(VIOLATES) = `power.ViolationProbability` of the relayed answer: the probability it gave to VIOLATES, or, when it gave none, 1 for a VIOLATES answer and 0 for a CONSISTENT one |

For K1–K3 the top-label confidence is c. For K4 it is the probability of the
predicted option. `tauScreen` thresholds P(VIOLATES), and `tauRoute` thresholds the
K3 confidence. Each threshold is resolved for the answer's source by
`format.Threshold.For` (§12).

### 7.2 Source (2): agreement among host votes

**J-H (JH5, JH3; K1–K3).**

- **Votes.** A vote is a record of `alignment.jsonl` with judge J-H, arm NAME-MASKED
  and status `ok`. JH5 uses repetitions 1 to 5 (k = 5), and JH3 uses repetitions
  1 to 3 (k = 3).
- **Aggregation.** The alignment module's `aggregate.Aggregate`, which is also the
  pilot's §7.4:
  - the modal criterion has the highest count, with ties going to the higher mean
    stated confidence and then to the lexicographically smaller reference;
  - the modal share is the modal criterion's count over k, so a missing vote counts
    against agreement;
  - the modal coverage is the commonest among the votes on the modal criterion, with
    ties going to the lexicographically smaller level.
- **Confidence.** The modal share s (`host-votes.modal-share`) for K1, K2 and K3.
- **Predictions:**
  - K1: the modal criterion;
  - K2: ADEQUATE when the modal coverage is FULL or PARTIAL;
  - K3: ALIGNED when the modal criterion is D(t) and the modal coverage is FULL or
    PARTIAL.
- **No cut.** The `modalMin` cut is not applied, so every share enters calibration.
  The cut defines source (3) (§7.3).
- **Positive-event probability** (K2, K3): s when the positive class is predicted,
  1 − s otherwise.
- A test with no vote has no decision and is counted.

**V-H (VH2; K4).**

- **Votes.** A vote is a record of `validity.jsonl` with judge V-H1 or V-H2 and
  status `ok`. Its per-criterion verdict for c maps:
  - VIOLATED → VIOLATES;
  - NOT_VIOLATED → CONSISTENT;
  - UNDETERMINED → an abstention.
- **Modal answer.** The modal answer of (m, c) is whichever of VIOLATES and
  CONSISTENT has more votes. A one-to-one split between them, or no vote for either,
  leaves no modal answer.
- **Agreement share.** s = the number of votes on the modal answer divided by 2
  (`host-votes.modal-share`). s takes the values 0.5 and 1.
- **Prediction.** The modal answer. A pair without one has no prediction, enters
  source (3), and is excluded from the top-label metrics.
- **Positive-event probability.** P_votes(VIOLATES) is the screen's rule for host
  votes. `power.ViolationProbability` counts each vote by its choice: 1 for
  VIOLATES, 0 for CONSISTENT, whatever probability the vote states. It takes the mean
  over the votes that answered, so an abstention or a missing vote is left out, and
  a pair with no answering vote gets 0.
- **Secondary mapping** (`host-votes.weakest-stated`, the mapping the product
  applies to validity votes). When both votes give the same answer, the confidence is
  the smaller stated confidence, and the positive-event probability is that
  confidence or its complement.
- **Population.** The K4 pairs of mutants with at least one V-H record. Pairs with
  none are counted.

### 7.3 Source (3): UNDETERMINED

For a source and kind with threshold t, UNDETERMINED(t) is the share of decisions
whose confidence is below t, or that have no prediction. It equals 1 − coverage(t)
on the risk–coverage curve of §8.3, which reports it over every threshold.
Fall-throughs (§4.6) are counted apart and are never UNDETERMINED.

Operating points:

| Source | Kinds | Threshold | Product rule |
|---|---|---|---|
| S1 | K3 | 0.90 | alignment `tauRoute`, default |
| S1 | K1 | 0.90 | reported for comparison |
| JH5 | K1–K3 | 0.80 | alignment `modalMin`, default, resolved for `host-votes(5)` |
| JH3 | K1–K3 | 0.90 | alignment `tauRoute` for `host-votes(3)`, the closed questions' host votes at `decideK` = 3; this is unanimity |
| VH2 | K4 | both votes on the same answer with stated confidence ≥ 0.70 | discpower `tauJudge`, default, resolved for `host-votes(2)` |

The screen has no UNDETERMINED outcome: below `tauScreen` a survivor is screened
out. For K4 under S1, the screen-out rate at 0.20 and its miss rate take the place
of source (3).

Per operating point, the report gives:

- the UNDETERMINED rate;
- the error rate among the decided decisions;
- the truth composition of the UNDETERMINED decisions.

### 7.4 Secondary measures and checks

**Returned-probability measures** (K1–K3). These come from the `probabilities`
member of the server's reply:

- K1: the probability of the predicted option;
- K2: P(FULL) + P(PARTIAL);
- K3: q = min(P_criterion(D(t)), P(FULL) + P(PARTIAL)).

Every returned-probability figure names its server.

**Confidence checks** (S1, K1–K3, per model):

- Spearman's ρ between the product confidence and the returned probability of the
  predicted option;
- the number of answers without a `confidence` member, which count as 0 there.

**Screen regression check** (S1, K4, per model). For every accepted answer:

- the product's P(VIOLATES) must equal the server's returned probability of
  VIOLATES, within 10⁻⁹;
- P(VIOLATES) must be non-decreasing in that returned probability over all of the
  model's answers;
- answers whose document carries no probability of VIOLATES are counted.

A failure is listed per answer and logged as a `protocol` entry with K4 and that
model as its scope.

**Key check.** For each model, the report gives the exact `confidenceSource` string
of §3.1. Two models that share a string are reported as a key collision.

These checks feed no band and no rule, except through the `protocol` entry a
regression failure logs.

## 8. Metrics

Every metric is computed per (kind, source, model) on pass-1 answers. K1–K3 use the
PRIMARY truth set unless a table names CONSENSUS. For JH and VH cells, the model is
the pilot's host model. For HV-d it is d, and for HC it is the voter set. HC and HV
cells are keyed by their §3.1 Id.

### 8.1 Calibration

- **ECE, primary.** 10 equal-width bins on [0, 1], with the last bin closed. It is
  computed on the positive-event probability for K2–K4 and on the top-label
  confidence for K1:
  ECE = Σ_b (n_b/N)·|mean p_b − observed frequency_b|. Empty bins are skipped.
- **ECE, secondary.** Top-label ECE for K2–K4. For S1 only, ECE over equal-mass bins:
  10 for K4 and 5 for K1–K3.
- **Reliability table.** Per bin: the count, the mean confidence and the observed
  frequency with its Wilson interval. Stored in
  `results/reliability/<kind>__<source>__<model>.csv` and rendered in the report.
- **Q0.9.** The observed accuracy among decisions whose top-label confidence lies in
  [0.85, 0.95), with the decision interval of §8.5.
- **Brier score.** (p − y)² on the positive-event probability for K2–K4, and the
  top-label Brier score, (c − correct)², for K1. The Brier skill score is taken
  against the constant base-rate predictor of the same item set.

### 8.2 Discrimination and accuracy

- **AUROC:**
  - for K2–K4, of the positive-event probability against truth;
  - for K1, of the top-label confidence against correctness.

  Ties count one half. An AUROC with a single truth class present is undefined.
- **Accuracy** of the prediction.

### 8.3 Risk–coverage

- **Curve.** Decisions are sorted by top-label confidence, descending, and ties form
  one block. At each distinct threshold t:
  - coverage(t) is the share of decisions at or above t;
  - risk(t) is the error rate among them.
- **Reported:**
  - the curve and AURC;
  - C05, the largest coverage whose risk point estimate is ≤ 0.05;
  - C05u, the largest coverage whose risk decision-interval upper bound is ≤ 0.10.
- **K4 screen quantities,** over the grid τ ∈ {0.05, 0.10, 0.15, 0.20, 0.25, 0.30,
  0.40, 0.50} on the positive-event probability:
  - miss rate(τ): the share of VIOLATES-truth pairs below τ;
  - flag rate(τ): the share of all pairs at or above τ.

  Both are reported on all pairs and on the screen stratum (§3.4).
- **K3 gate quantity.** The false-ALIGNED rate(t): the share of NOT_ALIGNED among
  decisions predicted ALIGNED with confidence ≥ t. It is reported at t = 0.90 and
  over the curve.

### 8.4 Missing and fall-through data

A (kind, source, model) cell is INCONCLUSIVE for bands (§11) and rules (§12) when
more than 0.10 of its decisions are:

- for S1, fall-throughs;
- for source (2), units without any vote.

Its figures are still reported.

### 8.5 Intervals and clustering

- **Clusters:**
  - K1–K3 are clustered by the declared criterion D(t), giving 10 clusters.
  - K4 is clustered by mutant source, read in `analyze` only:
    - arm T: the generator call (`generator.call_id` of `mutants/<mutant_id>.json`);
    - arm P: the source criterion n of `PARK-n_*`, read from the snapshot's
      `prereg/arm_p/labels.yaml`.
  - Sensitivity: K4 clustered by criterion c.
- **Cluster bootstrap:** percentile interval, B = 10,000, seed 20261007. Clusters are
  resampled with every decision inside them.
- **Wilson 95 %** for single proportions. n is the number of units: tests for K1–K3,
  (m, c) pairs for K4.
- **Decision interval.** For a proportion, the wider of Wilson and the bootstrap.
  ECE, Brier, AUROC, AURC, C05 and differences use the bootstrap only.
- An estimate over zero units, or an undefined statistic, is INCONCLUSIVE.
- Intervals are unadjusted for multiplicity and labelled so.

### 8.6 Determinism (S1)

Pass 2 is compared with pass 1, per (kind, model):

- the share of answers with identical `confidence` and probabilities at the server's
  reported precision;
- the maximum and 99th percentile of |Δc|;
- the prediction flip rate, with its Wilson interval.

Kev's `prefix_cache_hit`, and Ollama's `prompt_eval_cached_count` where the server
returns it, are recorded with each answer. A (kind, model) cell with a flip rate above
0.01 is annotated UNSTABLE.

## 9. Comparisons (descriptive)

- **Trivial baselines,** per kind:
  - the majority-class constant predictor, with its accuracy and its Brier score at
    the base rate;
  - for K1, also "always D(t)". It uses information that the product withholds from
    every answerer;
  - for K4, also "flag everything" and "flag nothing".
- **Source (1) against source (2) on shared items:**
  - K1–K3: each S1 model against JH5 on the shared tests;
  - K4: each S1 model against VH2 on the shared pairs.

  Reported: accuracy, AUROC, ECE and Brier score on the shared items; the paired
  cluster-bootstrap difference of each; the both-wrong count; and P(S1 wrong |
  source (2) wrong). The inputs differ (§3.2), and the tables say so.
- **Two-signal ALIGNED simulation** (ADR 0005 claim-power row ALIGNED), per S1 model
  and for JH5:
  - The pilot's matrix signature is recomputed with the snapshot's
    `pilot.domain.matrix` from run-02's `mutant_classes.json` and `kills.jsonl`.
    PINS_OFFSPEC is `none(t)` > 0.20.
  - A test is two-signal ALIGNED when three conditions hold:
    - the K3 prediction is ALIGNED with confidence ≥ 0.90 (S1) or ≥ 0.80 (JH5);
    - the matrix is SPECIFIC or OVERBROAD;
    - the matrix is not PINS_OFFSPEC.
  - Reported:
    - false ALIGNED among two-signal ALIGNED, with its Wilson interval;
    - the 2×2 error table of K3 against the matrix binary verdict, and κ;
    - P(source wrong | matrix wrong);
    - the both-wrong count.
- **Model sizes.** Paired cluster-bootstrap differences in AUROC, Brier and ECE for
  these pairs:
  - M3–M2, M4–M5 and M5–M6, within a family;
  - M1–M6 and M1–M2, across families;
  - every model against M1.

  A pair with a BLOCKED member is reported as BLOCKED.

## 10. Admission test (ADR 0003, keyed as ADR 0005)

The test is run per (kind, `confidenceSource`, model, template digest), on pass-1
answers and the primary confidence. For source (2), the template digest is the
SHA-256 of the pilot prompt and schema the votes answered.

1. The k clusters are ordered by `sha256("20261007|" + cluster_id)`. The first
   ⌈k/2⌉ form the fit half, and the rest are held out.
2. An isotonic map (pool-adjacent-violators, non-decreasing) is fitted on the fit
   half, from top-label confidence to correctness. A held-out decision takes the
   fitted value of the largest fit-half confidence at or below its own, or the lowest
   fitted value when there is none.
3. The threshold is a calibrated value of 0.95.
4. The test is met when, among held-out decisions with a calibrated value ≥ 0.95:
   - the error point estimate is ≤ 0.05;
   - the Wilson upper bound of the error is ≤ 0.10;
   - coverage (their share of held-out decisions) is ≥ 0.30.

**Preconditions.** The fit half holds at least 200 labelled decisions in at least
10 clusters. Otherwise the outcome is:

- UNREACHABLE, if the item set cannot supply them;
- INCONCLUSIVE, if missing or fall-through data removed them.

**Also reported for K4,** beside the ADR rule:

- the same test with the cluster-bootstrap upper bound in place of Wilson;
- the test restricted to decisions predicting VIOLATES (the direction that sends a
  survivor to the pool) and to decisions predicting CONSISTENT (the screen-out
  direction);
- the error of the majority-class rule at the same coverage.

**Unreachable outcomes, stated in advance:**

- **K1–K3, every source.** There are at most 38 tests (K2 at most 32) in 10
  clusters, so the fit half cannot hold 200 decisions. A held-out half of at most
  19 tests cannot reach a Wilson upper bound of 0.10 even with no error: 0/19 gives
  0.168.
- **VH2.** Its confidence takes two values, so the isotonic map has at most two
  levels. The test is reachable only if the unanimous level alone carries the held-out
  error and coverage conditions.

Outcomes are MET, NOT_MET, UNREACHABLE or INCONCLUSIVE. A MET outcome holds for one
author-built fixture and one domain, and is reported as a candidate for its key. It
grants no power: ADR 0005's claim-power table governs what a System One answer or a
host vote may do, whatever this test says. This experiment never edits a decision
policy or its digest.

## 11. Reading bands

Bands are read per (kind, source, model). The inputs are the primary measure, pass 1
and the PRIMARY truth set. Rules are evaluated in this order, and the first that
applies gives the band.

| Band | Rule | Meaning for the product |
|---|---|---|
| BLOCKED | the model is blocked (§6), or its kind's items are missing (§2.2) | none |
| INCONCLUSIVE (data) | §8.4 applies | none |
| USELESS | AUROC decision upper bound < 0.60; for K1, also when the accuracy upper bound is below the majority-class accuracy | this source's confidence carries no usable signal for this kind; §12 proposes removing the use where a rule covers it |
| THRESHOLD_READY | AUROC lower bound ≥ 0.75; ECE point ≤ 0.05 and ECE upper bound ≤ 0.10; for K4, also a miss-rate upper bound of ≤ 0.10 and a flag-rate point of ≤ 0.50, both at τ = 0.20 on the screen stratum | a threshold on this source's confidence can be read as an error rate from these tables, within the uses ADR 0005 allows |
| ROUTE_ONLY | AUROC lower bound > 0.55 | the confidence orders the work, but its values are not error rates; thresholds come only from the §12 derivations |
| INCONCLUSIVE | otherwise | none |

Annotations: UNSTABLE (§8.6) and the admission outcome (§10). No band grants an
answer the power to block, credit or exclude beyond ADR 0005's claim-power table.

With at most 38 tests in 10 clusters, the ECE interval for K1–K3 is wide.
INCONCLUSIVE and ROUTE_ONLY are the expected bands there, even for a well-calibrated
source.

Conclusion templates, one per (kind, source, model):

- THRESHOLD_READY: "<source> (<model>) on <kind>: AUROC <a> [lo, hi], ECE <e> [lo, hi], Q0.9 <q> [lo, hi], n = <n> decisions in <k> clusters. A threshold on this confidence reads as an error rate on this fixture; this does not establish calibration on other repositories."
- ROUTE_ONLY: "<source> (<model>) on <kind>: AUROC <a> [lo, hi], ECE <e> [lo, hi]. The confidence ranks decisions above chance but its values are not error rates; thresholds must be derived, not read."
- USELESS: "<source> (<model>) on <kind>: AUROC <a> [lo, hi]. No usable signal on this fixture."
- INCONCLUSIVE: "<source> (<model>) on <kind>: the data cannot place the result in a band (AUROC <a> [lo, hi], ECE <e> [lo, hi])."
- BLOCKED: "<source> (<model>) on <kind>: not measured (<reason>)."

## 12. Parameter derivation

`derive` reads `results/decisions.json` and writes `results/policy-inputs.json`.

**Threshold form.** Every module keeps its thresholds as `format.Threshold` values.
Each one is a default and optional entries keyed by a source kind (`system-one`,
`host-votes`) or by one exact source. `Threshold.For(source)` resolves a source in
this order: the exact source, then its kind, then the default. A module records the
value it applied in `extensions.decision.threshold`. In a configuration file, a
threshold is a `format.ThresholdOverride`: either a bare number, which sets the
default, or a mapping of `default` and of sources to numbers. A member the mapping
leaves out keeps its value.

**Entry format.** One entry per proposal:

`{module, configFile, configKey, policyKey, source, override, kind, model, templateDigest, rule, inputs}`

- `configFile` and `configKey`:
  - `.spec-proof/config.yaml` and `policy.tau_screen` for discpower;
  - `.spec-proof/alignment.yaml` and `policy.tau_route` for alignment.
- `policyKey`: `tauScreen` or `tauRoute`.
- `source`: the exact source the entry keys.
- `override`: the YAML mapping `{<source>: <value>}`. It sets that one entry and
  leaves the default and every other entry as they are.

**Endpoint digest.** An exact source-(1) key carries the endpoint digest (§3.1). The
digest of the experiment's endpoint is not the digest of a user's endpoint, so each
S1 entry is written twice:

- once for the experiment's endpoint, under which `analyze` replays the rule;
- once for the canonical default address of the model's server:
  - `http://localhost:11434/v1/systemone` for Ollama;
  - `http://127.0.0.1:8008/v1/systemone` for Kev.

Each digest is computed as the relay computes it.

**Check.** `derive` passes every override through the discpower builder's
`check-threshold` mode (§4.1). The check requires `For(source)` to return the
proposed value, and every other source the run measured to resolve to the module's
default. An entry that fails is not written, and the failure is a `protocol` entry.

An entry enters a module's configuration only by the user's or the project's
choice. This experiment writes no configuration or policy file.

**Common rules:**

- A rule whose inputs are INCONCLUSIVE, BLOCKED or capped by a deviation (§14.2)
  proposes nothing, and the module's default stays.
- **Volume gate.** A proposal that lets more decisions through than the default
  needs the admission precondition (§10) to hold for its key: at least 200 labelled
  decisions in at least 10 clusters in the fit half. This applies to a `tauScreen`
  entry above 0.20 and to a `tauRoute` entry below 0.90. A proposal in the other
  direction is not gated.
- **Exact sources only.** Every proposal keys one exact source. No rule proposes a
  kind entry or a new default, so a source the run did not measure keeps the
  module's default.
- **No new power.** No rule makes a System One answer or a host vote block, grant
  pass credit or exclude beyond the claim-power table of ADR 0005. A rule only moves
  the threshold of one source within a use the table allows.

| Rule | Input | Derivation | Proposal |
|---|---|---|---|
| CS1 `tauScreen` | each S1 model M; K4; screen stratum; pass 1; P(VIOLATES) | Over the grid of §8.3, keep each τ whose miss-rate decision upper bound is ≤ 0.10 and whose flag-rate point is ≤ 0.50. | The entry `system-one:<M>@<endpointDigest>: τ`, with τ the largest kept value; when the volume gate does not hold, the largest kept value at or below 0.20. If none is kept, the entry is 0.00: every survivor is flagged, and that source screens nothing out. With fewer than 10 VIOLATES-truth pairs in the stratum: no proposal. If the pilot's SW2a has fired, CS1 is reported and has no effect. |
| CS2 `tauRoute` | each S1 model M; K3; pass 1 | The false-ALIGNED rate at confidence ≥ 0.90. | If its Wilson lower bound is > 0.10: the entry `system-one:<M>@<endpointDigest>: 1.00`, the largest value a threshold admits. Then only an answer of confidence 1.00 from that source clears ALIGNED with the matrix, and every other test goes to the judge. Otherwise no proposal. No entry below 0.90 is proposed on this item set, because the volume gate cannot hold at 38 tests. The global switch `twoSignal` is not proposed; it belongs to the pilot's SW3. If SW3 has fired, CS2 is reported and has no effect. |
| CS3 endpoint model | K4; all pairs; pass 1; each S1 model M against M1 | Paired cluster-bootstrap differences. | Recommend M when four conditions hold: AUROC(M) − AUROC(M1) has a lower bound > 0; Brier(M) − Brier(M1) has an upper bound < 0; M is not USELESS on K3; and M meets §8.4 on K4. Among several, the highest AUROC point wins, and the smaller model breaks a tie. Otherwise recommend M1 when its K4 band is THRESHOLD_READY or ROUTE_ONLY, and nothing else. If M1 is BLOCKED: no recommendation. The recommendation names a model for the relay's `systemone.model` setting and is not a threshold. |

Sensitivities reported for CS1: all pairs in place of the screen stratum, and
clustering by criterion.

**Source (2) proposes nothing.** The thresholds its votes inform are owned by the
pilot's switch rules:

- discpower `tauJudge`, resolved for `host-votes(2)`, belongs to SW1 and SW5;
- alignment `modalMin`, resolved for `host-votes(5)`, and `alignK` belong to SW4.

The votes also answered the pilot's prompts, not the product's closed question
sets. Source-(2) figures are reported beside those switch rules and never override
them.

## 13. Sensitivity analyses

All of these are reported, and none feeds a band or a rule:

- CONSENSUS truth (K1–K3);
- K1 lenient truth;
- pass 2 in place of pass 1 (S1);
- returned probability in place of the product confidence, per server (S1, K1–K3,
  §7.4);
- the weakest-stated mapping in place of the agreement share (VH2);
- K4 clustered by criterion;
- K4 strata by arm, by F3 status and by target against other criteria;
- K4 restricted to mutants with a witness in the state (F3 DIFFERS).

## 14. Deviations and preconditions

### 14.1 Preconditions

- **PC1.** The tag `prereg-calibration` exists, its timestamp verifies (§16), and the
  run manifest records the harness commit and the builder digests. Otherwise, no run.
- **PC2.** The pilot inputs qualify (§2.2): the tag `prereg-limits-1-3-a1` exists,
  the pilot analysis commit exists, and the run-02 files of §1 exist. Otherwise, no
  file under `experiments/limits-1-3/runs/` is opened and the run does not start. When
  only arm records are missing, the kinds they feed are BLOCKED.
- **PC3.** The build checks hold:
  - (a) the bindings that the alignment builder reads from Shallnot equal run-02's
    `bindings.json`. Both sides are compared as pytest node ids:
    `<file relative to fixture/parkfee>::<Classname>::<name>`, with the class part
    left out when it is empty;
  - (b) every arm-T and arm-P `mutant_id` is reproduced from its normalized edits by
    the snapshot's `pilot.domain.ids`;
  - (c) every closed set passes `hostproto.ParseClosedSet`, and its module's response
    schema is built from it;
  - (d) every item's template digest equals the `decide.closed` entry of its module's
    `prompt.TemplateDigests()` at the frozen commit;
  - (e) the builder harness test of §16 step 3 passed at the harness commit.

  Otherwise, no run.
- **PC4.** Every canary passes (§5.4). Otherwise, that model does not run.
- **PC5.** The a1 tree holds `ClassifyPlan`, the source grammar and `classify-plan`.
  The a1 genTime precedes the pilot analysis commit. Otherwise HC and HV are BLOCKED.

The machine-safety rules of §6 are not preconditions. They wait, pause or block as §6
states.

### 14.2 Deviation log

`runs/<run-id>/DEVIATIONS.md` starts with the title `# Deviations`, followed by one
entry per line:

`- <kind> | scope: <scope> | <text>`

`<scope>` is `none`, `all`, or identifiers separated by commas, drawn from K1–K4,
M1–M6, JH5, JH3, VH2, CS1–CS3, HC-H, HC-S, HC-P, HV-S, HV-O and CL1. An output for
(kind k, source s, model m), or a rule, is in scope when its identifier, k, s or m is named, or when the scope is
`all`.

| Kind | Logged when | Writer | Effect |
|---|---|---|---|
| `provenance` | a recorded value that this document does not pin (OS build, Python patch, uv version) changes between blocks | pipeline | none |
| `pause` | a block stops (window close, memory guard, heavy-work check, server crash, sleep) and is resumed later | pipeline | none |
| `blocked` | a model cannot be served or held (§6), or a pilot input is missing | pipeline or operator | outputs in scope are BLOCKED |
| `protocol` | any departure from this document, including a harness behaviour this document does not fix. It includes any change from the lock or between blocks of: the Ollama version (pinned at 0.35.1), a model digest, the Kev commit, a Hub revision, the harness commit or a builder digest | pipeline or operator | outputs in scope: THRESHOLD_READY becomes INCONCLUSIVE, MET becomes INCONCLUSIVE, and the CS rules in scope propose nothing |

- Operator entries are written only with `log-deviation`, which accepts only
  `blocked` and `protocol`.
- Any other non-blank line counts as a `protocol` entry with scope `all`.
- The run is resumable: a request whose response file exists is not resent.

## 15. Records and reporting

### 15.1 Run directory

| File | Content |
|---|---|
| `manifest.json` | run id; the `prereg-calibration` commit; the harness commit; the builder source and binary digests; the pilot tag commit and the pilot analysis commit; the snapshot and product-export digests; the template digests per module; the source-(2) prompt digests (`prereg/prompts/alignment.md`, `prereg/prompts/validity.md` and their schemas); server versions; the `prereg-calibration-a1` commit; the plan and ticket digests; the voter ids; the CLI versions; the plugin versions |
| `pilot_inputs.json` | every run-02 file read, with its SHA-256 (§2.3) |
| `snapshot.json` | the digest of every snapshot file |
| `models.lock.json` | §5.3 |
| `items/<request_id-slug>.json` | `{request_id, kind_family, unit, closed, template_digest, bodies: {model: body}}`; alignment items also hold `nodeid`, `label` and `token_to_ref`; mutant items also hold `mutant_id`, `question_to_ref` and `witness_status` |
| `excluded.json` | masker rejections with reasons, `witness_unmapped` mutants, and mutants without an oracle record |
| `schedule.json` | pass-1 and pass-2 request ids, in order |
| `server/<model>/<block-start>.json` | the §5.3 server card |
| `canary/<model>.json` | the canary request, the response and its checks |
| `raw/<model>/<pass>/<request_id-slug>/` | `request.bin` (bytes sent), `response.bin` (bytes received) and `meta.json` `{http_status, attempt, started, finished, latency_ms, server_version, model_digest}` |
| `safety.jsonl` | every guard reading, heavy-work check, fit computation and window decision (§6) |
| `DEVIATIONS.md` | §14.2 |
| `timing.json` | per block: start, end, request count, and median and p95 latency |

### 15.2 Results

- `results/decisions.json`, per (kind, source, model):
  - counts: items, decisions, clusters, fall-throughs by type, units without votes;
  - every metric of §8 as `{point, lo, hi, method}`;
  - the source-(3) figures of §7.3;
  - the band, annotations, admission outcome and `capped_by`.
- `results/policy-inputs.json` (§12).
- `results/reliability/*.csv`, `results/risk_coverage/*.csv`,
  `results/comparisons/*.csv` and `results/checks.csv`.
- `results/REPORT.md`, in this order:
  1. provenance;
  2. the band table (every kind × source × model cell, BLOCKED included);
  3. the conclusion sentences;
  4. the source-(3) table;
  5. the admission table;
  6. the CS proposals;
  7. reliability and risk–coverage tables;
  8. determinism;
  9. comparisons;
  10. the confidence, screen-regression and key checks (§7.4);
  11. sensitivity analyses;
  12. the machine-safety summary (deferrals, pauses, blocks per model);
  13. deviation counts per kind, with a pointer to the file;
  14. the vendor-reported figures;
  15. limits;
  16. an exploratory discussion of at most 300 words, labelled exploratory.

Vendor-reported figures (Ollama library pages, Kev model cards) appear only in their
own table. Each row carries its source URL and the words "vendor-reported", and no
measured comparison uses them. A USELESS, NOT_MET, UNREACHABLE or INCONCLUSIVE result
is reported in the same tables and with the same prominence as any other.

`report-lint` fails in these cases:

- one of the words 'works', 'proves', 'proven', 'validated', 'confirm',
  'generalizes', 'well-calibrated' or 'human audit' appears, matched as a whole word
  in any case. Three places are exempt: conclusion templates, quoted run text, and
  the ADR 0003 phrase "confirm ALIGNED";
- a CONSENSUS figure appears without its PRIMARY figure;
- a returned-probability figure appears without its server;
- a table holding source-(2) figures lacks the fixed note "Source-(2) votes
  answered the pilot's prompts, not the product's closed question sets.";
- a figure from the vendor table appears outside it;
- a kind × source × model cell is missing from the band table;
- an HC figure does not name its voter set.

Limits paragraph, verbatim in the report:

- one author-built fixture, one language and one domain;
- 38 tests and 10 criteria;
- alignment truth comes from author labels verified by execution against a
  hand-written violation set;
- mutant truth is relative to model-written oracles, whose author model is not
  recorded;
- the mutant items are model-written and planted mutants, while the product screens
  operator mutants;
- the screen state shows all ten statements without the specification's precedence
  paragraph, and its witness comes from the pilot's differential;
- the source-(2) votes answered the pilot's prompts on the pilot's inputs, not the
  product's closed question sets, and one host model cast all of them;
- the alignment items are too few for the admission test;
- decisions within a mutant are correlated;
- one laptop, and server numerics depend on hardware and kernels;
- intervals are unadjusted.

## 16. Run order and freeze

1. **Freeze.** The tag `prereg-calibration` is set on the commit that adds this
   document and `prompts/`:
   - `freeze/prereg-commit.txt` holds that commit's hash;
   - `freeze/prereg-commit.tsr` is an RFC 3161 timestamp of it from
     `http://timestamp.digicert.com`, with its query `freeze/prereg-commit.tsq`.

   The pilot's freeze uses the same format. The tagged commit's tree is the product
   source of §2.1.
2. **Blindness.** No file under `experiments/limits-1-3/runs/` is opened for this
   experiment before the tag exists and PC2 holds (§2.3).
3. **Harness.** The harness and the builders implement this document, and their
   commit is recorded in every run manifest. Before the first item request, their
   tests pass on synthetic run directories. Those tests include a builder test. It
   drives the alignment binary of the frozen commit on the project
   `s1relay/testdata/alignment/project` to its first `awaiting_host` wave, by the
   procedure of `s1relay/internal/e2e/e2e_test.go`, and reads
   `spec-proof-alignment systemone export`. The alignment builder must rebuild every
   exported closed set from the same inputs, byte for byte. A harness behaviour that
   this document does not fix is logged as a `protocol` entry naming its scope.
4. **Preparation:** `snapshot`, then `build-items` (PC3), then `pin` (the static rule
   of §6.4).
5. **Sessions.** `run-session`, night after night inside the window, runs the blocks
   in the order M3, M2, M1, M4, M5, M6. Each block holds the model's `canary` (PC4,
   first block only), then pass 1, then pass 2.
6. **Analysis.** Once every model is complete or BLOCKED: `analyze`, `derive`,
   `report-lint`.

Any change to this document or to `prompts/` after the tag is a new
pre-registration, and the first complete run under the old one is still reported. A
rerun of a model block is allowed only after an infrastructure failure of the whole
block (server or machine) and before any `analyze` output exists. It is logged as a
`protocol` entry with that model as its scope.

`prereg-calibration-a1` is frozen as in step 1, in `freeze/a1-commit.{txt,tsq,tsr}`.
The first complete run under `prereg-calibration` is still reported.

## 17. Source (4): host classify, and the decider comparator

### 17.1 Arms

- HC-H: `host-classify(5,rot1):claude-haiku-4-5-20251001*5`
- HC-S: `host-classify(5,rot1):claude-sonnet-5-5*5`
- HC-P: `host-classify(5,rot1):claude-sonnet-5-5*3+claude-haiku-4-5-20251001*2`, made
  of calls S0–S2 and H0–H1.

The template digest of an HC cell is `sha256(decide.closed digest + "|rot1|" + CLI
version)`.

### 17.2 Calls

- **Archive.** `build-items` archives each question's `ClassifyPlan`, with i = 0–4.
- **Execution.**
  - Each planned call runs once under Haiku and once under Sonnet, after the archive
    is complete.
  - `claude -p --plugin-dir harness/classify-mod` loads a mod that reads the plan
    with `$.fs`, calls `$.model.classify`, and writes one raw record per call:
    `{request_id, question, i, model, order, label|null, attempt, started, finished}`.
    The harness records `claude --version` per session.
  - At most 4 calls are in flight, and the run is resumable.
- **Canary.** A canary call per model runs first. A model the allowlist refuses makes
  its arms BLOCKED.
- **Failures.** An `undefined` label or a rejection is retried once with the same
  order. A second `undefined` is a missing vote, and a second rejection is MISSING. A
  question with no prediction is a fall-through (§4.6).
- **Recording.** The product records a host-classify answer on the channel
  `host-classify` at isolation `restricted-unverified`. The experiment measures
  correctness, not isolation, and §12's claim-power statement is unchanged.

### 17.3 Comparator HV-d

The decider model d is `claude-sonnet-5-5` for HV-S and `claude-opus-5-5` for HV-O.

- **Tickets.** A builder mode `tickets` writes each archived closed set as a pending
  `decide.closed` request, through the module's own store package. The store is a
  fresh project at `runs/<run-id>/hv/<d>/<request>/`.
- **Votes.** The module's fulfil skill runs there under `claude -p --model d`, with
  `decide_k` = 3 and attestation on.
- **Readings.** K4 uses the modal answer and `power.ViolationProbability`, and K1–K3
  use `aggregate.Aggregate`.

### 17.4 Confidence and chain

- **Share and ties.** s and ties are as in §7.2.
- **Maps.** A PAV map from s to correctness is fitted on the fit half (§10 steps 1–2),
  giving c.
- **K1–K3.** K2 is ADEQUATE when the modal coverage is FULL or PARTIAL. K3 is ALIGNED
  when K1 is D(t) and K2 is ADEQUATE, with confidence min(c_K1, c_K2).
- **Answered request.** HC-a answers a request when every one of its questions has
  c ≥ 0.95.
- **Chain(a, d).** HC-a answers the requests it answers, and HV-d answers the rest.
- **Attestation.** HV-d votes run with attestation on (§17.3). The product's record of
  a host-classify answer is as §17.2 states.

### 17.5 Arm selection (fit half, per kind)

- **Choice.** The arm with the largest share of decisions in answered requests is
  selected.
- **Ties.** Arms within 0.02 of it are ranked HC-H, then HC-P, then HC-S.
- **No arm.** When no arm answers any request, CL1 is NOT_MET (`no-arm`).

### 17.6 Rule CL1 (held-out, paired cluster bootstrap as in §8.5)

CL1 is MET for (K4, the selected arm a, d) when all of these hold:

1. §10 gives MET, with the decisions of answered requests as the decisions at or above
   the threshold.
2. Among those decisions, the ones predicting CONSISTENT have an error point ≤ 0.05
   and a Wilson upper bound ≤ 0.10.
3. On answered requests, error(HC-a) − error(HV-d) has an upper bound ≤ +0.02.
4. On the screen stratum at τ = 0.20, the chain against HV-d has a miss-rate
   difference whose upper bound is ≤ +0.02, and a flag-rate difference whose point is
   ≤ +0.05. With fewer than 10 VIOLATES-truth pairs, this condition is INCONCLUSIVE.
5. Brier(chain) − Brier(HV-d) has an upper bound ≤ +0.01.
6. §8.4 holds for HC-a and for HV-d.
7. No `protocol` entry is in scope.

**Admission file.** `derive` writes `results/classify-admission.json`, one entry per
(kind, source, d): the outcome, the tested fit-half PAV table with its digest, and the
CLI version.

A MET entry grants no power (§12).

### 17.7 Stated in advance

- **K1–K3:** UNREACHABLE (§10). They are reported, and no alignment entry is written.
- **K4:** UNREACHABLE when the fit half holds fewer than 200 decisions or fewer than
  10 clusters.
- **A MET covers** one fixture, model-written and planted mutants; arm O is
  unmeasured.

### 17.8 Reported

- every arm's held-out figures (descriptive when unselected);
- the request-level skip rate;
- the distribution of s, and the share of calls that pick the first-listed label;
- P(Haiku wrong given Sonnet wrong), and both-wrong counts;
- HC against VH2 and JH5, under §15.2's note.
