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
  form of the subject and of the inputs, each input by its role, name, digest and
  globs, regardless of the order of the inputs, and give another key when any of
  them changes; the criterion a `proof` input is about does not reach the key.
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

- **SP-196~1**: WHEN a module's pending file is decoded THE SYSTEM SHALL require
  a module name that is one path segment and entries that each name a criterion
  `ID~REV` with a revision from 1 on and a reason of the closed set
  `awaiting_host`, `awaiting_proof`, `awaiting_host_judgment` and
  `host_unresponsive`, strictly ascending by criterion and then reason, ignore
  unknown members, and report `missing_field`, `requirement`, `unknown_value` or
  `unsorted` for a violation.
- **SP-197~1**: WHEN the path of a proof is built or read THE SYSTEM SHALL give
  `proofs/<module>/<subject key>/<cacheKey>.dsse.json` for a signed proof and
  `<module>/<subject key>/<cacheKey>.json` for a bare statement, accept a path
  only when its module and subject key are one path segment and its cache key is
  a SHA-256 hex digest, and derive the subject key from the subject name with its
  `%`, `/` and `\` percent-encoded, so that distinct subjects keep distinct keys.

- **SP-198~1**: WHEN statement bytes are admitted for signing or for verified
  storage THE SYSTEM SHALL decode and validate them, refuse bytes that are not
  their own RFC 8785 canonical form, refuse a predicate type outside the format,
  and place the statement at its module, the subject key of its subject and its
  cache key, so that a statement stored anywhere else is refused.

- **SP-199~1**: WHEN a statement is wrapped for an Ed25519 identity THE SYSTEM
  SHALL carry the exact statement bytes as the base64 payload of a DSSE envelope
  of the in-toto payload type, sign their DSSE pre-authentication encoding,
  identify the key by the hex SHA-256 of its raw public key, serialize the
  envelope in RFC 8785 canonical form, and refuse an envelope with another
  payload type, an undecodable payload or signature, no signature or a signature
  without key identifier; and WHEN a signed proof file is read THE SYSTEM SHALL
  tell a bare envelope from a Sigstore bundle by its top-level members.

- **SP-225~1**: WHEN the TUF root of the public-good Sigstore instance is asked THE
  SYSTEM SHALL give the root embedded in the library as a parseable TUF root
  document, and a copy of it each time.

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

## Input descriptors and decisions

- **SP-400~2**: WHEN a proof about a criterion lists its inputs THE SYSTEM SHALL
  describe the statement as one `spec` input named `ID~REV` with the statement
  digest of the subject, each file the proof depends on as one `code_tree` or
  `test_source` input named by a path relative to the directory holding
  `shallnot.yaml` with `/` separators and carrying the SHA-256 of the file's
  exact bytes, and a consumed proof as one `proof` input named by its kind,
  carrying the criterion in `annotations.criterion` and the SHA-256 of the
  Statement file's exact bytes, and SHALL report `input_descriptor` for a
  descriptor of another shape.
- **SP-900~1**: WHEN the inputs of a predicate are ordered or checked THE SYSTEM
  SHALL order them strictly ascending by role, then name, then the
  `annotations.criterion` of a `proof` input, so that several `proof` inputs of
  one kind, one per consumed criterion, are valid, and SHALL report `unsorted`
  for a descending pair or for one input listed twice.
- **SP-401~1**: WHEN a predicate or a mutant records a decision THE SYSTEM SHALL
  require its `confidenceSource` to be `execution`, `host-votes(<k>)` or
  `system-one:<model>@<endpoint digest>`, accept the id of the mapping that
  produced the confidence in `confidenceMapping`, and report `unknown_value`
  for another source.
- **SP-901~1**: WHEN a threshold is read THE SYSTEM SHALL accept a bare number,
  which is the default for every confidence source, or a mapping of `default` and
  of confidence sources to numbers in [0,1], where a source is a kind
  (`execution`, `host-votes`, `system-one`) or one exact source, SHALL refuse a
  value outside [0,1] and a key that names no source, and WHEN a threshold is
  resolved for a source THE SYSTEM SHALL take the entry of the exact source, else
  the entry of its kind, else the default, a source without a form of the format
  taking the default; and a threshold without entries SHALL encode as a bare number.

- **SP-960~1**: WHEN the source a relay imports an answer under is read THE SYSTEM
  SHALL accept `system-one:<model>@<endpoint digest>`, name it unchanged as the
  confidence source of the answer, and refuse any other text, including a source
  without a model or without a digest.

## Host protocol: closed questions

- **SP-402~1**: WHEN the `closed` member of a request is validated THE SYSTEM
  SHALL require a state and 1 to 64 questions with distinct non-empty ids and
  instructions, each of the type `choice` (2 to 26 options), `score` (at least 2
  levels, lowest first) or `noul` (no options, or `true` then `false`), whose
  options have distinct non-empty names and a criteria text.
- **SP-403~2**: WHEN a response schema is built for a question set THE SYSTEM
  SHALL require one answer per question, by id, whose choice is an option of that
  question (`true` or `false` for a noul question) and whose confidence and
  probabilities of its options are optional, and WHEN answers are read or written THE SYSTEM SHALL keep them in the
  order of the questions.
- **SP-404~2**: WHEN a question set is rendered for a `/v1/systemone` endpoint THE
  SYSTEM SHALL name the questions in order with their type, instructions and
  criteria (option to description for a choice, ordered descriptions for a score,
  `true` and `false` for a noul), and WHEN the endpoint's response is read THE
  SYSTEM SHALL turn a choice into its option, its confidence and the probability of each
  option, a noul into `true` from probability one half with the confidence
  `max(p, 1-p)` and the probabilities `p` and `1-p`, and a score into the nearest
  level, keep the confidence apart from the probabilities, and SHALL refuse a response that leaves a question unanswered.
- **SP-405~1**: WHEN the closed requests of a listing are exported to a relay THE
  SYSTEM SHALL list the first vote of each with its ticket, nonce and question set.

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
  the transcript path, the last message, the resolved model, the usage and the
  reply text of the tool response, and
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

## Host protocol: harness, hooks and executor server of a module

- **SP-700~1**: WHEN a host harness is selected for a module THE SYSTEM SHALL
  name the executor tools of the module's plugin as that harness spells them,
  recognize them and no other tool in a hook payload, name the type of each
  executor agent the module ships as that harness reports it and map a reported
  type back to the role of its requests, take an agent the module does not ship
  for no executor, derive the isolation the harness's allowlist reaches
  (restricted on Claude Code, unrestricted on Codex, restricted-unverified on
  Copilot), and waive attestation on a harness that cannot issue a capability.
- **SP-701~1**: WHEN a response is read for a proof THE SYSTEM SHALL credit it
  with `restricted-attested` and the digest of its transcript only when the stop
  hook attested the agent that answered and no leak, exposure or conflict marks
  its ticket, and with the model the harness resolved when it reported one,
  leaving the stored profile as it was.
- **SP-702~1**: WHEN tickets are listed, shown or answered through the request
  service THE SYSTEM SHALL list the emitted tickets with their role and executor
  view, move a shown ticket to fetched, refuse a call without a capability while
  attestation is required, accept only the capability of a started agent bound
  to that one ticket, record an answer with the profile of its origin (executor,
  sampled, replayed, relayed, command line), journal an answer no capability
  backs as exposed outside an executor and as restricted inside one whose
  attestation was waived, allow one correction after a schema violation before
  the ticket is rejected, refuse a different second answer as a resubmission
  conflict and accept the same one as a replay, and refuse a nonce or an
  emission other than the issue's.
- **SP-703~1**: WHEN the executor server runs THE SYSTEM SHALL offer
  `fetch_request` and `submit_response`, and `drain` only when the client
  declared sampling and the module turned it on; return the schema violations of
  an answer as a result; and, for `drain`, put every pending ticket to the
  client's model through sampling with no context and no tools, submit the JSON
  object of each sampled answer as a sampled response with no capability, serve
  only the module it is named for and stop at the first refused sample or
  invalid answer without accepting it.
- **SP-704~1**: WHEN the PreToolUse hook sees the launch of an executor THE
  SYSTEM SHALL rebuild it to the bare ticket, in the foreground and without a
  model, deny a prompt without exactly one ticket and a ticket that is unknown or
  for another role, and leave every other launch and tool alone.
- **SP-705~1**: WHEN the PreToolUse hook sees a call of an executor tool THE
  SYSTEM SHALL add the capability of the calling agent when its type is the role
  of the ticket, deny otherwise and journal an exposure attempt with the agent,
  deny when the call cannot be checked, and leave the drain tool to the main
  agent.
- **SP-706~1**: WHEN the SubagentStart hook sees an executor THE SYSTEM SHALL
  journal its agent and role, and journal no other agent.
- **SP-707~1**: WHEN the SubagentStop hook sees an executor stop THE SYSTEM SHALL
  attest the run with the digest of its transcript when its calls are exactly the
  fetch and the submit of one ticket and its reply is the bare receipt, journal
  an exposure for any other call and a leak for any other reply, journal a
  restricted run when the transcript cannot be read, and never block the
  subagent.
- **SP-708~1**: WHEN the PostToolUse hook sees an executor launch finish THE
  SYSTEM SHALL record the resolved model and the usage against its ticket and
  journal a leak for a reply other than the bare receipt, and record nothing for
  another agent or a launch without a ticket.
- **SP-709~1**: WHEN a Codex or Copilot hook runs THE SYSTEM SHALL serve the hooks
  that harness has: capabilities, the start and the stop of executors on Codex,
  the denial of a call of the wrong role and the stop check on Copilot, resolving
  the role of an agent from the journal when the call does not carry its type,
  and conclude nothing from a call that carries no agent identity where the
  harness does not identify callers.
- **SP-710~1**: WHEN a hook is not about the module's executors THE SYSTEM SHALL
  leave the call alone without opening any state, deny a call of the module's own
  tools that it cannot check, and accept a receipt with whitespace around it.
- **SP-711~2**: WHEN the host files of a module are generated for a harness THE
  SYSTEM SHALL give the Claude Code plugin its manifest, its executor server, one
  agent per executor role of the module holding exactly the two executor tools
  with the inherited model, a blind context and four turns, its four hooks under
  matchers that select the executor alone, and a skill that pre-approves what its
  loop runs and, once the loop ended with a verdict, publishes the local proofs by
  running `spec-proof-verify --accept-local .spec-proof/cache --out
  .spec-proof/verified`; give Codex read-only agents carrying the executor server,
  hook entries and a skill that ends the same way; give Copilot agents naming
  server and tool pairs, a hooks file and a skill that ends the same way; and
  carry no text about their own authoring.
- **SP-712~1**: WHEN the host files are generated twice THE SYSTEM SHALL produce
  the same bytes, refuse a harness it has no files for, and ship an agent file,
  a matcher alternative and a role only for the executor agents the module has.
- **SP-713~1**: WHEN a host bundle is made or read THE SYSTEM SHALL list the
  responses in a manifest with the module, the head commit, the journal digest and
  the marks the journal holds, give the same bytes for the same responses in any
  order, refuse files that are not a consistent bundle, name the ref of a commit
  under `refs/spec-proof/host/<module>`, refuse a bundle of another version only
  when requests found no answer and name both versions, derive the CI seed from
  the commit and the first run only, and carry CI-issued requests in full so that
  their ids can be recomputed.
- **SP-714~1**: WHEN a proof asks for tickets through the bridge THE SYSTEM SHALL
  emit the missing tickets of a request, retire a fetched ticket past its time to
  live, reissue a rejected or expired one with a new nonce until its emissions run
  out, read the answers the lifecycle lets a proof consume with the profile they
  are credited with, move votes through validation, rejection and consumption,
  and supersede the tickets of requests no proof of the wave asked for.
- **SP-715~1**: WHEN the host answers are collected and imported THE SYSTEM SHALL
  put every answered ticket in the bundle with the profile it is credited with and
  the marks against it, refuse a store whose journal chain is broken, serve the
  tickets a run asks for from a pushed bundle through the same checks as any
  answer, leave the others pending, mark again what the pushing host marked, end
  the replay when the bundle answers nothing, and refuse an answer that breaks the
  schema.
- **SP-716~1**: WHEN the requests CI left for the host are exported and imported
  THE SYSTEM SHALL export the requests whose tickets wait, in full, with their
  votes, criteria and purpose, import each once as tickets on another machine,
  and refuse a request the manifest does not list.
- **SP-1602~1**: WHEN a module's CI run leaves the requests it issues THE SYSTEM
  SHALL name one directory per module below `.spec-proof/ci-issued`, so that no
  module overwrites the requests of another, and WHEN the artifact a CI job
  uploaded is read THE SYSTEM SHALL take the issued bundle from the root of the
  artifact or from the directory of the module below it.
- **SP-717~2**: WHEN a module is installed into a project THE SYSTEM SHALL write
  the module's configuration seed and ignore its local state (the statement cache,
  the verified set and the pending files), end a configuration it writes or
  extends with the commented example of the operations block, install the files
  of each selected harness (`auto`, `none` or a list) by merging into every file
  another tool shares and writing the files it owns, enable the plugin in the
  project's Claude Code settings without overriding the user's choice, plan no
  change on a second run, and leave Shallnot's configuration, its block of
  AGENTS.md and its hooks as they were.
- **SP-718~2**: WHEN a module is enabled or disabled THE SYSTEM SHALL turn the
  proof on or off in the module's configuration, add or remove its kind in
  `proofs.require` of `shallnot.yaml` and its block of AGENTS.md by line edits
  that keep every other byte, so that disabling removes what enabling added to
  `proofs.require` and to AGENTS.md and leaves `proofs.trust` and `proofs.code`,
  which the other modules rely on, refuse a project with no `shallnot.yaml`,
  refuse a layout it cannot extend without rewriting, say that the gate reads the
  proofs of this machine and that the CI configuration of the gate sets
  `proofs.trust` to `signed`, and for an advisory proof that Shallnot refuses in
  `proofs.require` write no require entry, mark its block advisory and say why.
- **SP-1603~1**: WHEN a required proof is enabled THE SYSTEM SHALL set
  `proofs.trust` of `shallnot.yaml` to `local` when it sets none, and list in
  `proofs.code` the source roots of the layout the project shows when it lists
  none, never change a value that is set, keep every comment and every other
  byte, apply the same edits once when several modules are enabled, and say so
  when no source root is found; and the source roots SHALL be `**/*.go` for a Go
  module, `src` for a Node project that has it, `src/main` for a Maven or Gradle
  project that has it, and for a Python project `src` or else each top-level
  directory that is a package and is not a test, documentation, build, hidden or
  private directory.
- **SP-1604~1**: WHEN a module is installed THE SYSTEM SHALL keep the statement
  cache, the verified set and the pending files out of version control and end
  the configuration it writes or extends with a commented example of the
  `operations` block, which binds each operation of the examples of a spec to a
  target under the language of its ecosystem, and SHALL add that example neither
  to a configuration that binds operations nor twice, so that a second run
  changes nothing.
- **SP-719~1**: WHEN the host settings of the shared configuration file are read
  THE SYSTEM SHALL keep attestation required and sampling off when the section is
  absent, read `sampling` and `require_attestation`, refuse an unknown or
  mistyped member, and tolerate the sections the other modules keep in the file.
- **SP-720~1**: WHEN recorded answers are replayed THE SYSTEM SHALL answer each
  pending ticket from the recording of its vote, falling back on the first vote's
  recording, keep the recorded executor and give the inline profile when there is
  none, fail on a ticket without a recording, refuse a pending ticket that is not
  an identifier and an answer that breaks the schema.

## Schemas and golden vectors

- **SP-22~1**: WHEN the embedded schemas are listed THE SYSTEM SHALL give each
  as a JSON Schema 2020-12 document whose `$id` is its predicate type URI, and
  resolve each predicate type URI of the format to the schema it identifies.
- **SP-24~1**: WHEN a golden vector is decoded and validated THE SYSTEM SHALL
  give the outcome and the violation codes its manifest states.

## Command-line conventions

- **SP-1600~1**: WHEN a command line asks for help with `-h`, `--help`, `-help` or,
  in the place of a command, `help` THE SYSTEM SHALL print the usage of what it
  was asked about on the output stream and succeed, giving the commands of a
  binary, the verbs of a command that takes one, and for a command its synopsis
  and exactly the flags that command defines with their values and defaults; and
  WHEN a command line does not parse THE SYSTEM SHALL print the failure and the
  usage of that command on the error stream and exit with the usage code, the
  same in every binary of the repository.
- **SP-1601~1**: WHEN the fields of an output line are joined THE SYSTEM SHALL
  separate the non-empty fields with one space and leave out the empty ones, so
  that a line never starts or ends with a space and never holds two in a row.
