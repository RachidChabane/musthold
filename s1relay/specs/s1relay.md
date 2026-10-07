# s1relay requirements

The requirements of the `s1relay` module, the only module that reaches a System
One endpoint, in Shallnot's Markdown spec format. `shallnot gate` traces them
to the Go tests of the module in CI.

## Binary

- **SP-105~1**: WHEN the `spec-proof-s1` binary is run with the `version`
  argument THE SYSTEM SHALL print its version.

## Drain

- **SP-600~2**: WHEN `drain --module M` runs THE SYSTEM SHALL run
  `spec-proof-M systemone export` from the project root, post each exported
  closed request to the endpoint, and run `spec-proof-M systemone import` with the
  reply of the endpoint for each usable answer, with the ticket, its nonce and the source
  `system-one:<model>@<endpointDigest>`, the confidence source a proof records.
- **SP-601~1**: WHEN `drain` runs THE SYSTEM SHALL take the endpoint and the
  model from `--endpoint` and `--model`, else from `systemone.endpoint` and
  `systemone.model` of `.spec-proof/config.yaml`, and WHEN no usable endpoint is
  given THE SYSTEM SHALL exit 2 without running a module or opening a
  connection.
- **SP-602~1**: WHEN a module is named THE SYSTEM SHALL run the executable
  `spec-proof-` followed by the module name, accept only a name of lower-case
  letters, digits and hyphens, and fail with exit 2 when the executable cannot
  be run.
- **SP-603~1**: WHEN an endpoint is configured THE SYSTEM SHALL post to its
  address with the route `/v1/systemone` added when the address has no path, name
  it in a source by the SHA-256 of its scheme, host and path, so that
  credentials and query never appear in a proof, and give two different
  endpoints two different digests.
- **SP-614~1**: WHEN the module refuses the answers of a request THE SYSTEM SHALL
  report the request as unanswered with the kind `import` and the reason the
  module gave, and go on with the next request.
- **SP-616~1**: WHEN an exported closed request cannot be read, holds a question
  of a type no endpoint answers, or holds more questions than the route takes
  THE SYSTEM SHALL post nothing for it and report it as unanswered.

## Wire mapping

- **SP-604~1**: WHEN a question set is posted THE SYSTEM SHALL send its state, each
  question under its id in the order of the set with its text as the
  instructions, a choice with its options as an object from name to
  description, a score with its levels as an array of descriptions, an option
  without description described by its name, and the model only when one is
  named, and SHALL read the question set of a request through the host protocol's
  closed question type, whose options are objects with a name and a description,
  and leave a noul question to the host votes.
- **SP-605~1**: WHEN an endpoint replies THE SYSTEM SHALL map a choice to the
  option it names, a score to the option nearest to the weighted level, keep the
  confidence only when the endpoint stated one, ignore fields it does not know,
  and name the replying model, for the replies of Ollama and of a Kev server
  alike.
- **SP-606~1**: WHEN a reply is not JSON, omits a question, answers with another
  type than the one asked, names a choice that is not an option, gives a score
  outside the levels or a confidence outside [0,1] THE SYSTEM SHALL import
  nothing for the request and report it as unanswered with the kind
  `malformed`.

## Failures and limits

- **SP-607~1**: WHEN the endpoint times out, cannot be reached, answers with a
  4xx status, which includes a state larger than the model accepts, or answers
  with a 5xx status THE SYSTEM SHALL leave the request unanswered, report its
  kind (`timeout`, `transport`, `rejected` or `server`) with the status and the
  start of the body, and go on with the next request.
- **SP-608~1**: WHEN three requests in a row fail because of the endpoint itself
  THE SYSTEM SHALL post no further request and report the remaining ones as
  skipped.
- **SP-609~1**: WHEN a batch runs THE SYSTEM SHALL post one request at a time and
  open the next only after the previous reply or failure.
- **SP-610~1**: WHEN `--max-requests N` is given with N above zero THE SYSTEM
  SHALL post at most N requests and report how many exported requests it left
  untouched.
- **SP-611~1**: WHEN a batch has posted a request and the endpoint answers the
  version route of Ollama THE SYSTEM SHALL unload the model with a generate
  request whose keep_alive is zero, unless `--no-unload` is given, and SHALL
  leave any other server untouched and report an unload failure without failing
  the batch.
- **SP-612~1**: WHEN `--mem-guard` is given THE SYSTEM SHALL refuse the batch with
  exit 4 when fewer than 40 percent of memory is free on darwin, read from
  `memory_pressure` with the threshold of `MEM_GUARD_MIN_FREE_PCT` when set,
  stop posting when the rule refuses between two requests, fail closed when the
  percentage cannot be read, and skip the rule with a logged reason on every
  other operating system.
- **SP-613~1**: WHEN a drain ends THE SYSTEM SHALL print one JSON report with the
  endpoint digest, the counts and the outcome of each request, and exit 0 when
  no request is left unanswered, 1 when some are, 2 on a tool failure and 4 when
  the memory rule refuses.

## End to end

- **SP-615~1**: WHEN the closed requests of the real alignment binary are drained
  into a stub endpoint THE SYSTEM SHALL leave none of them waiting for a first
  vote and record their answers under the relay source, so that the module
  carries on with them.

## Plugin

- **SP-830~1**: WHEN the Claude Code plugin of the relay is generated THE SYSTEM
  SHALL produce its files from one definition, the same bytes on every run, and
  the committed tree under `plugin/claude` SHALL equal what the generator produces.
- **SP-831~1**: WHEN the plugin files are read against the formats Claude Code
  documents THE SYSTEM SHALL give a manifest that names the plugin `spec-proof-s1`
  with a version and a description, and one skill that runs `spec-proof-s1 drain
  --module` for each proof module, pre-approves nothing else, and carries no text
  about its own authoring.
