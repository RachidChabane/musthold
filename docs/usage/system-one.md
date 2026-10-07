# System One and the relay (`spec-proof-s1`)

System One is an optional accelerator the environment may provide: a service that
answers closed questions (a choice among named options, a score on ordered levels,
a true/false question) with a confidence. spec-proof never requires it. Without
it, the host votes: `decide_k` blind `decider` executors answer each closed
question and the modal share is the confidence.

`spec-proof-s1` is the only program of the project that opens a connection to a
System One endpoint. The proof modules open no connection and call no model; the
relay is the environment's client of your service.

## Resolution chain

For every closed request (a `decide.closed` ticket):

1. If `systemone.endpoint` is configured and `spec-proof-s1` is on `PATH`, the
   module's fulfil skill runs `spec-proof-s1 drain --module <module>`. The relay
   answers what it can. A relay failure leaves the requests for step 2.
2. Otherwise `decide_k` fresh `decider` executors vote, the confidence is the modal
   share, and the confidence source is `host-votes(k)`.
3. A confidence below the threshold of its source leaves the answer UNDETERMINED.

The source of a confidence is recorded in the proof as `execution`,
`host-votes(<k>)` or `system-one:<model>@<endpoint digest>`, where the digest is the
SHA-256 of the endpoint's scheme, host and path, so credentials and query strings
never appear in a proof. Thresholds are keyed by source; see
[configuration](configuration.md#thresholds-per-confidence-source).

## Configure

```yaml
# .spec-proof/config.yaml
systemone:
  endpoint: http://localhost:11434
  model: example-model
```

`--endpoint` and `--model` override the file. An address without a path gets the
route `/v1/systemone`.

## drain

```
spec-proof-s1 version
spec-proof-s1 drain --module MODULE [--endpoint URL] [--model NAME] [--timeout DURATION] [--max-requests N] [--root DIR] [--mem-guard] [--no-unload]
```

`spec-proof-s1 -h`, `--help` and `help` print the commands, and `spec-proof-s1 drain -h`
prints the flags below with their defaults; both exit 0.

| Flag | Default | Meaning |
|---|---|---|
| `--module` | required | The proof module whose closed requests are relayed: `dp`, `alignment` or `completeness`. The relay runs the executable `spec-proof-<module>`; a module name holds lower-case letters, digits and hyphens. |
| `--endpoint` | `systemone.endpoint` | The endpoint. |
| `--model` | `systemone.model` | Model named in each request. |
| `--timeout` | `2m` | Time limit of one request. Must be positive. |
| `--max-requests` | `0` | At most N requests are posted; 0 posts them all. The report says how many were left. |
| `--root` | working directory | Project root. |
| `--mem-guard` | off | Refuse to start, and stop between requests, when free memory is below the rule: fewer than 40 percent free on macOS (`MEM_GUARD_MIN_FREE_PCT` changes the threshold), fail closed when it cannot be read, skipped with a logged reason on other systems. |
| `--no-unload` | off | Keep the model loaded after the batch. |

`drain` runs `spec-proof-<module> systemone export` from the project root, posts
each exported closed request to the endpoint, one at a time, and runs
`spec-proof-<module> systemone import` with the reply of the endpoint for each
usable answer, under the source `system-one:<model>@<endpoint digest>`. Run it by
hand or let the fulfil skill do it.

When the endpoint answers the version route of Ollama, `drain` unloads the model
after the batch with a generate request whose `keep_alive` is zero, so a batch
leaves no resident model. Another server is left untouched, and an unload failure
is reported without failing the batch.

### Wire format

Each question goes under its id, in order, with its text as the instructions: a
choice sends its options as an object from name to description, a score sends its
levels as an array of descriptions, and the model is sent only when one is named.
A true/false question is left to the host votes. In the reply a choice names an
option, a score maps to the option nearest the weighted level, and the confidence is
kept only when the endpoint states one. Unknown fields are ignored.

### Failures

A request is left unanswered, with a kind in the report, when:

- the reply is not JSON, omits a question, answers with another type, names an
  option that does not exist or gives a score or confidence out of range
  (`malformed`);
- the exported request cannot be read, holds a question type no endpoint answers or
  more questions than the route takes (nothing is posted for it);
- the module refuses the answers (`import`);
- the endpoint times out, cannot be reached, answers 4xx or 5xx (`timeout`,
  `transport`, `rejected`, `server`).

After three requests in a row fail because of the endpoint itself, no further
request is posted and the rest are reported as skipped.

### Report and exit codes

`drain` prints one JSON report: the module, the endpoint digest, the model, the
counts `answered`, `unanswered` and `remaining`, one item per request and the
unload outcome. Progress lines go to standard error.

| Code | Meaning |
|---|---|
| 0 | no request is left unanswered |
| 1 | some requests are unanswered |
| 2 | tool failure: no usable endpoint, a bad flag, a module that cannot be run |
| 4 | the memory rule refused the batch |
