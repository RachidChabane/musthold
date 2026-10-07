# format requirements

The requirements of the `format` module, the proof format shared by every
module, in Shallnot's Markdown spec format. `shallnot gate` traces them to the
Go tests of the module in CI.

## Statement envelope

- **SP-1~1**: WHEN a statement is decoded THE SYSTEM SHALL read the in-toto
  Statement v1 envelope of the three predicate types of the format, report a
  `statement_type` or `predicate_type` violation for a wrong type URI, and
  refuse a predicate type outside the format.
- **SP-2~1**: WHEN a statement carries a member the format does not define THE
  SYSTEM SHALL ignore it, so that minor versions stay additive.
- **SP-3~1**: WHEN a claim is built THE SYSTEM SHALL give a judged claim a
  confidence in [0,1] and an executed claim none.
- **SP-4~1**: WHEN a decoded claim is judged without a confidence, executed with
  a confidence, or carries a confidence outside [0,1] THE SYSTEM SHALL report
  `judged_without_confidence`, `executed_with_confidence` or `confidence_range`.
- **SP-5~1**: WHEN a claim has a class other than the one its predicate field
  allows THE SYSTEM SHALL report `wrong_class`.
- **SP-6~1**: WHEN a value is serialized THE SYSTEM SHALL produce RFC 8785
  canonical bytes, and canonicalizing canonical bytes SHALL leave them
  unchanged.
- **SP-7~1**: WHEN the members of a JSON object are written in another order THE
  SYSTEM SHALL produce the same canonical bytes.

## Digests and keys

- **SP-8~1**: WHEN a file set is digested THE SYSTEM SHALL hash `path NUL
  content NUL` over the files sorted by path, regardless of the order given, and
  refuse a path that is not relative POSIX or that occurs twice.
- **SP-9~1**: WHEN a cache key is computed THE SYSTEM SHALL hash the canonical
  form of the subject and the inputs, regardless of the order of the inputs, and
  give another key when any input changes.
- **SP-10~1**: WHEN the statement is an alignment or a discriminating-power
  statement THE SYSTEM SHALL require exactly one subject named `ID~REV` after
  the requirement of the predicate, with the statement digest.
- **SP-11~1**: WHEN the statement is a completeness statement THE SYSTEM SHALL
  require exactly one subject named by a relative POSIX spec path, with the hash
  of the file.
- **SP-12~1**: WHEN an array of a predicate is not strictly ascending in
  byte-wise order of its sort key THE SYSTEM SHALL report `unsorted`.
- **SP-13~1**: WHEN an input role, a verdict, a validity, an outcome, a producer
  or a decision basis is outside its closed set, or a digest is not a SHA-256
  hex string, THE SYSTEM SHALL report `unknown_value` or `bad_digest`.
- **SP-14~1**: WHEN the recorded `cacheKey` differs from the digest of the
  subject and the inputs THE SYSTEM SHALL report `cache_key_mismatch`.
- **SP-21~1**: WHEN a `junit_result` digest is computed THE SYSTEM SHALL hash
  the canonical map from test identifier to outcome and never the raw XML.

## Predicates

- **SP-15~1**: WHEN an alignment predicate is checked THE SYSTEM SHALL require a
  judged verdict and a blind judge.
- **SP-16~1**: WHEN a discriminating-power predicate is checked THE SYSTEM SHALL
  require an executed `correctCodePasses`, a judged validity per mutant and an
  executed outcome per test result.
- **SP-17~1**: WHEN a mutant is a sentinel THE SYSTEM SHALL require its validity
  to be EQUIVALENT.
- **SP-18~1**: WHEN a discriminating-power predicate lists example results THE
  SYSTEM SHALL require each to be an executed `passed` or `failed`.
- **SP-19~1**: WHEN a completeness predicate is checked THE SYSTEM SHALL require
  `advisory` to be true and SHALL refuse a verdict field in its schema.
- **SP-20~1**: WHEN a predicate carries `extensions.decision` or
  `extensions.operatorScreen` THE SYSTEM SHALL keep them through decoding and
  encoding and check the decision's basis and confidences.

## Pending criteria and ledger layout

- **SP-147~1**: WHEN a module's pending file is decoded THE SYSTEM SHALL require
  a module name that is one path segment and entries that each name a criterion
  `ID~REV` with a revision from 1 on and a reason of the closed set
  `awaiting_host`, `awaiting_proof`, `awaiting_host_judgment` and
  `host_unresponsive`, strictly ascending by criterion and then reason, ignore
  unknown members, and report `missing_field`, `requirement`, `unknown_value` or
  `unsorted` for a violation.
- **SP-148~1**: WHEN the path of a proof is built or read THE SYSTEM SHALL give
  `proofs/<module>/<subject key>/<cacheKey>.dsse.json` for a signed proof and
  `<module>/<subject key>/<cacheKey>.json` for a bare statement, accept a path
  only when its module and subject key are one path segment and its cache key is
  a SHA-256 hex digest, and derive the subject key from the subject name with its
  `%`, `/` and `\` percent-encoded, so that distinct subjects keep distinct keys.

- **SP-149~1**: WHEN statement bytes are admitted for signing or for verified
  storage THE SYSTEM SHALL decode and validate them, refuse bytes that are not
  their own RFC 8785 canonical form, refuse a predicate type outside the format,
  and place the statement at its module, the subject key of its subject and its
  cache key, so that a statement stored anywhere else is refused.

## Executors and corroboration

- **SP-147~1**: WHEN a predicate carries `extensions.hostExclusions` THE SYSTEM
  SHALL keep it through decoding and encoding.
- **SP-111~1**: WHEN a predicate or a judged claim carries an executor THE
  SYSTEM SHALL require a channel, an isolation level and a model source of their
  closed sets, a named harness, a named model unless its source is `unknown`,
  and a SHA-256 prompt digest; and, for a `restricted-attested` executor, an
  agent identifier, a cap digest and a transcript digest; and SHALL report
  `unknown_value`, `missing_field` or `bad_digest`, and `executed_with_executor`
  for an executed claim that carries one.
- **SP-112~1**: WHEN a judged mutant validity is VALID THE SYSTEM SHALL require
  its `corroboratedBy` list, whose entries are of a closed set of kinds, carry a
  SHA-256 evidence digest and are strictly ascending, and SHALL report
  `uncorroborated_claim` for an empty list and `executed_with_corroboration` for
  an executed claim that carries one.
- **SP-113~1**: WHEN a mutant is unattributed or off-target THE SYSTEM SHALL
  require its validity to be NOT_VIOLATING and SHALL refuse a mutant that is
  both, reporting `inconsistent_attribution`.
- **SP-114~1**: WHEN an input names a host response or a consumed proof THE
  SYSTEM SHALL accept the roles `host_response` and `proof`.
- **SP-115~1**: WHEN a predicate schema is listed THE SYSTEM SHALL state its
  minor version, equal to the minor version of its kind.
- **SP-116~1**: WHEN isolation levels are compared THE SYSTEM SHALL order
  `restricted-attested`, `restricted`, `restricted-unverified`, `unrestricted`
  and `inline` from the strongest to the weakest, and rank an unknown level
  below every floor.

## Host protocol: requests and tickets

- **SP-117~1**: WHEN a request is validated THE SYSTEM SHALL require the request
  type URI of its major version, a named module, a kind of the closed set, a role
  `plugin:agent` whose agent serves the kind, a shape that is closed exactly for
  `decide.closed` and carries its question set exactly then, a round and a seed
  exactly for generation kinds, a user prompt and a template digest, a response
  schema whose root is an object, an output limit within the ceiling, strictly
  ascending inputs of known roles with SHA-256 digests, and a cache key; and the
  view an executor receives SHALL carry the ticket, its nonce, the prompt, the
  response schema and the question set only.
- **SP-118~1**: WHEN a request id is computed THE SYSTEM SHALL hash the RFC 8785
  bytes of the request document with `module.version` removed, so that the id is
  the same for any member order, unchanged by a module version bump, and changed
  by any other difference; and a document without a module object SHALL have no
  id.
- **SP-119~1**: WHEN a ticket is issued THE SYSTEM SHALL name it
  `<requestId>.<vote>.<emission>`, draw its 128-bit nonce from the entropy
  source it is given and keep the nonce out of the request id; WHEN a text is read
  as a ticket THE SYSTEM SHALL accept only that exact form, and SHALL find a
  ticket in a prompt only when the prompt holds exactly one; and a response SHALL
  carry a request id, a vote, a nonce, a JSON object output within the size limit and a
  valid executor profile.
- **SP-145~1**: WHEN the host-protocol schemas are listed THE SYSTEM SHALL give the
  request, the ticket and the response as JSON Schema 2020-12 documents whose
  `$id` is their URI, resolve each URI to its schema, and fix in the request
  schema the request type URI of its major version.

## Host protocol: lifecycle and binding

- **SP-120~1**: WHEN a ticket changes state THE SYSTEM SHALL allow only the moves
  of the lifecycle table (`emitted` to `fetched` or `superseded`; `fetched` to
  `accepted`, `expired` or `rejected`; `accepted` to `validated` or `rejected`;
  `rejected` and `expired` to `emitted` or `abandoned`; `validated` to
  `consumed`), refuse every other move without changing the entry, and treat
  `consumed`, `superseded` and `abandoned` as terminal.
- **SP-121~1**: WHEN a ticket is fetched, answered, expired or reissued THE SYSTEM
  SHALL bind a fetch to one capability, allow one schema correction before it
  rejects, expire a fetched ticket after 20 minutes, reissue a rejected or expired
  ticket with a new nonce while fewer than three emissions were made and abandon
  it as `host_unresponsive` after that, and make only a validated answer
  consumable.
- **SP-122~1**: WHEN a submission is checked THE SYSTEM SHALL require the ticket
  identifier, the nonce, a state that takes an answer, the schema digest recorded
  at emission and, when attestation is required, a capability of an agent
  started for the request's role, and SHALL bind one capability to one ticket: a
  ticket fetched with one capability refuses another, and a capability that
  already serves a ticket refuses every other. Any submission in which one of
  them differs SHALL be rejected.
- **SP-123~1**: WHEN a capability is made THE SYSTEM SHALL join the agent
  identifier and the lowercase hex HMAC-SHA256 of that identifier under the
  per-install secret, and WHEN it is verified THE SYSTEM SHALL compare in
  constant time and refuse any other secret, agent, MAC or spelling.
- **SP-124~1**: WHEN a second, different output arrives for an accepted ticket THE
  SYSTEM SHALL refuse it as `resubmission_conflict`, keep the first answer and
  journal the conflict, and SHALL treat the same output again as a replay.
- **SP-125~1**: WHEN a Store adapter is tested THE SYSTEM SHALL provide a contract
  suite and a reference store that satisfy it: content-addressed requests,
  entries created once per key and updated atomically, responses created once,
  a journal that stays a valid chain under concurrent appends, and a stable
  secret.
- **SP-126~1**: WHEN events are journaled THE SYSTEM SHALL chain each record to
  the digest of its predecessor from a genesis digest, SHALL refuse a chain in
  which a record was edited, removed, reordered or renumbered, and SHALL derive
  the roster of started executors from the journal.

- **SP-195~1**: WHEN a response is discarded THE SYSTEM SHALL remove it so that its
  key can be answered again, treat an absent response as discarded, and provide a
  contract suite and a reference store that satisfy it.

## Host protocol: hooks and channels

- **SP-127~1**: WHEN a Claude Code hook payload is decoded THE SYSTEM SHALL read
  PreToolUse on the Agent tool and on MCP tools, SubagentStart, SubagentStop and
  PostToolUse into a harness-neutral event with the agent, the tool, its input,
  the transcript path, the last message, the resolved model and the usage, and
  SHALL refuse a payload of another hook, without its required field, or
  malformed.
- **SP-128~1**: WHEN a Codex or a Copilot hook payload is decoded THE SYSTEM SHALL
  read the fields each documents into the same event, including Copilot's
  arguments given as a JSON string.
- **SP-129~1**: WHEN a hook answers THE SYSTEM SHALL rebuild an executor launch
  from its role and its single ticket with no model and no padding and in the
  foreground, refuse a launch of any other role or without a single ticket,
  inject the capability into an MCP tool input, and render the decision in the
  output format of each harness.
- **SP-130~1**: WHEN an executor transcript is checked THE SYSTEM SHALL read its
  tool calls in order and accept only `fetch_request(t)`, `submit_response(t)` and
  at most one corrective `submit_response(t)` for its own ticket.
- **SP-131~1**: WHEN a channel is selected THE SYSTEM SHALL choose from observed
  state only: the plugin subagent when its plugin and server are present, then
  sampling when declared and enabled, then the main agent; derive the isolation
  from what the harness enforces; and report the subagent channel as pending
  rather than degraded when attestation is required and its hooks are missing.
- **SP-132~1**: WHEN the host protocol vectors are replayed THE SYSTEM SHALL give
  the request ids, capabilities, ticket parses, lifecycle matrix, decoded hook
  events and rendered decisions the committed vectors state.
- **SP-133~1**: WHEN a decision function is checked for monotonicity THE SYSTEM
  SHALL report any step at which adding a response that points toward block or
  escalate lowers the severity of the decision.

## MCP stdio codec

- **SP-134~1**: WHEN MCP messages are framed THE SYSTEM SHALL read
  newline-delimited JSON-RPC 2.0 from an `io.Reader` and write it to an
  `io.Writer`, skip blank lines, accept CRLF, consume and report a frame above
  the byte limit without losing the following ones, classify each message as a
  request, a notification or a response, and refuse any frame that is not one,
  including an id that is neither a string nor a number.
- **SP-135~1**: WHEN a client initializes the session THE SYSTEM SHALL negotiate
  the newest supported protocol revision, announce the tools capability and the
  server identity, record whether the client declared sampling, refuse a
  second initialize and a request other than ping before it.
- **SP-136~1**: WHEN tools are listed THE SYSTEM SHALL give `fetch_request` and
  `submit_response` closed schemas whose members are all required, with the
  capability member described as set by the harness and to be left empty, and
  SHALL add `drain` only when the client declared sampling and the policy
  enables it.
- **SP-137~1**: WHEN a tool is called THE SYSTEM SHALL decode its arguments
  strictly against the closed schema, answer a missing, unknown or mistyped
  member as an invalid-params error, an unknown or unlisted tool likewise, and a
  failure of the executor service as a tool error; `fetch_request` SHALL return
  the executor view and `submit_response` the bare receipt or the schema errors.
- **SP-138~1**: WHEN ping, notifications, unknown methods and malformed frames
  arrive THE SYSTEM SHALL answer ping with an empty result, answer no
  notification, answer an unknown method as not found, answer a malformed frame
  with a parse or invalid-request error carrying a null id, answer each request
  exactly once, and stop at the end of input or of the context.
- **SP-139~1**: WHEN `drain` serves a ticket THE SYSTEM SHALL ask the client for
  `sampling/createMessage` with no included context and no tools, answer pings
  while it waits, queue other requests until the tool call ends, and report a
  refusal of the client as a tool error.

## Host protocol: test support

- **SP-140~1**: WHEN a fake host plays one of the personas honest, forger,
  omitter, stale, breaker, leaker, padder, model-swapper, hookless, unrestricted,
  selector, churner or slow against a host THE SYSTEM SHALL give the verdict
  asserted for that persona, and SHALL report an outcome that does not match it.
- **SP-141~1**: WHEN the golden JSON-RPC transcripts are replayed against the
  fake executor service THE SYSTEM SHALL write the server lines the transcripts
  commit, and the committed transcripts SHALL be those the scripts record.
- **SP-142~1**: WHEN a fake host drives a module through its command line THE
  SYSTEM SHALL list the pending tickets, show each ticket before it answers
  except for the forger, submit the output its persona gives, report each
  submission, reach nothing a hook is needed for, and fail on a module whose
  listing is unreadable.

## Schemas and golden vectors

- **SP-22~1**: WHEN the embedded schemas are listed THE SYSTEM SHALL give each
  as a JSON Schema 2020-12 document whose `$id` is its predicate type URI, and
  resolve each predicate type URI of the format to the schema it identifies.
- **SP-24~1**: WHEN a golden vector is decoded and validated THE SYSTEM SHALL
  give the outcome and the violation codes its manifest states.
