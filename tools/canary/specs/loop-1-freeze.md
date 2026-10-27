# loop-1 study tooling: freeze, timestamps and deviations

The verbs that close the study's inputs (`resolve`, `freeze-check`), the verb that
logs a departure from the protocol (`log-deviation`), and the RFC 3161 timestamps
they share (`experiments/loop-1/PREREG.md` §12, §13). The shared shapes are in
`loop-1.md`.

## RFC 3161 timestamps

- **SP-2610~1**: WHEN the text of an `openssl ts -reply -text` reply is read THE SYSTEM
  SHALL take from it the message imprint, read by the fixed columns of the hexadecimal
  dump for an imprint of any length, and the generation time in UTC, with or without
  fractional seconds, SHALL refuse a reply whose status is not granted, whose hash
  algorithm is not SHA-256, or that lacks the message data or the generation time, and
  SHALL read each committed token so that its imprint is the SHA-256 of its commit file.
- **SP-2611~1**: WHEN a file is timestamped THE SYSTEM SHALL build the query with
  `openssl ts -query -data <file> -sha256 -cert`, post it to the authority, verify the
  reply with `openssl ts -reply -text`, keep the query and the reply under the file's
  name with its extension replaced by `.tsq` and `.tsr` only when the reply is a
  granted token whose imprint is the SHA-256 of the file, and WHEN the tool, the
  authority or the check fails SHALL leave neither file behind.
- **SP-2612~1**: WHEN a query is posted THE SYSTEM SHALL send it with the method POST
  and the content type `application/timestamp-query` to `http://timestamp.digicert.com`,
  return the body of a reply with status 200, and refuse any other status.

## Deviation log

- **SP-2613~1**: WHEN an entry is appended THE SYSTEM SHALL create the log with its
  title when it is missing, write the entry as one line dated with the entry's time or
  the clock's, lose no entry and write the title once when entries are appended
  concurrently, and write nothing for an entry that cannot be written.
- **SP-2614~1**: WHEN a `protocol` or `token-exposed` entry is appended THE SYSTEM SHALL
  store its line in `deviation-timestamps/<UTC date>-<slug>.txt`, a name that never
  replaces an earlier file, timestamp that file, and WHEN the timestamp cannot be taken
  SHALL keep the entry in the log and report that its token is missing.
- **SP-2615~1**: WHEN `log-deviation` runs THE SYSTEM SHALL append the entry that
  `--kind`, `--scope`, `--section`, `--says` and `--happened` describe, accept every
  kind of the closed set, and exit 2 on a usage error or an entry the log cannot hold,
  and 1 when the timestamp is missing.

## resolve

- **SP-2616~1**: WHEN `resolve` runs THE SYSTEM SHALL refuse a tracked file that differs
  from HEAD, and record the commit of the pre-registration tag, HEAD as the inputs
  commit, and the commits after the pre-registration's, HEAD included, that touch the
  product directories, oldest first, reading the repository with git on the host.
- **SP-2617~1**: WHEN `resolve` records the product THE SYSTEM SHALL refuse a product
  built from a modified tree, from another commit than HEAD or from another Shallnot
  than `script/shallnot.pin`, and record the SHA-256 of each binary, the tree digest of
  each plugin tree, the digests of the images, and the toolchains: the versions of
  CPython, pytest, uv, Node.js and git read in the image, Go from the build image, the
  Docker engine and colima versions, the settings of the colima profile and the host's
  OS, version and architecture.
- **SP-2618~1**: WHEN `resolve` records Shallnot THE SYSTEM SHALL write the pin, the
  output of `shallnot --version` run in the image, and the lowest public release tag
  by version whose history holds the pin, or `none`.
- **SP-2619~1**: WHEN `resolve` records Claude Code THE SYSTEM SHALL refuse a version
  other than 2.1.295 in the image or in `script/claude-code.pin`, and record the version
  and the SHA-256 that `sha256sum` prints for the binary in the image.
- **SP-2620~3**: WHEN `resolve` records the construction and the canary THE SYSTEM SHALL
  copy the construction records construct appended, as one log of their calls, checks
  and drops, with the work directory, the repository and the home directory written as
  `<work>`, `<repo>` and `~`, record the Codex CLI version of every call, refuse a records file with a
  line it cannot read, and copy the auxiliary models, observed hosts, allowlist, quota
  patterns and records of the canary's last output.
- **SP-2621~1**: WHEN `resolve` records the policy THE SYSTEM SHALL read, from the source
  at the inputs commit, the default of each member of the table of PREREG §2 in the
  `Default` function of the module's policy package, resolving constants, threshold
  wrappers and the isolation level of the format package, compare numbers by value, and
  refuse a module that lacks a member.
- **SP-2622~1**: WHEN `resolve` records a task THE SYSTEM SHALL write the task's name,
  the tree digest of its fixture, its criterion, the SHA-256 of its prompt, its start
  commit, its equipped tree and instruction digests for each arm, and the tree digests
  of its oracle, reference, violations and refactors, and refuse a task description
  that differs from the configuration or arms that start from different commits.
- **SP-2623~1**: WHEN `resolve` writes the freeze THE SYSTEM SHALL record the run names
  in the seeded order of the configuration, write `inputs.json`, and write
  `inputs-SHA256SUMS` in `shasum` format, sorted, listing every tracked file of every
  task directory and fixture, of the configuration and of the image directory, and
  `inputs.json`, and SHALL exit 2 on a usage error and 1 on a failure.

## freeze-check

- **SP-2624~1**: WHEN `freeze-check` runs THE SYSTEM SHALL require the tags of the
  pre-registration, of its amendment and of the inputs, require each commit file to
  equal its peeled tag, require each token's imprint to be the SHA-256 of its commit
  file, and require the inputs tag to sit on a commit that changes the inputs and their
  sums alone, on top of the commit `commits.inputs` names, descending from the
  pre-registration's.
- **SP-2625~1**: WHEN `freeze-check` orders the generation times THE SYSTEM SHALL
  require the pre-registration and its amendment to precede every construction call and
  every canary session the inputs record, the inputs to follow them and to precede the
  start of every run, and refuse an inputs freeze that records no call, no session, or
  a call without a start.
- **SP-2626~1**: WHEN `freeze-check` reads the sums THE SYSTEM SHALL require each
  listed digest to be the SHA-256 of the file at the tag, `SHA256SUMS` to list exactly
  the files of the pre-registration's roots except the freeze directory, and
  `inputs-SHA256SUMS` to list exactly what `resolve` lists for the configuration at the
  inputs tag.
- **SP-2627~1**: WHEN `freeze-check` reads the pins THE SYSTEM SHALL require the Shallnot
  pin of the amendment in the inputs and in `script/shallnot.pin`, Claude Code 2.1.295
  in the inputs and in its pin file, the Go, CPython and uv versions and the virtual
  machine settings of PREREG §2, the pytest version of the lock file, the policy of the
  inputs and the defaults of the source at the inputs commit to equal the table of
  PREREG §2, and the arms' instructions to be equal, file for file, except
  `shallnot.yaml`.
- **SP-2628~1**: WHEN `freeze-check` reads the product THE SYSTEM SHALL recompute the
  digests of the binaries and plugin trees of the product directory and the images of its
  manifest, require them to equal the inputs, and require the engine to hold the built
  image under its recorded id.
- **SP-2629~1**: WHEN `freeze-check` finds a condition that does not hold THE SYSTEM
  SHALL continue, report every failure with the name of its condition on the error
  stream, print nothing on the output stream, and exit 1, exit 2 on a usage error, and
  exit 0 when every condition holds.
