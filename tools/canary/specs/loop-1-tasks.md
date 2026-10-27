# loop-1 task construction and acceptance requirements

The `construct` and `accept` verbs of the loop-1 study (`experiments/loop-1/PREREG.md`
§3.3 and §3.4), in Shallnot's Markdown spec format. `construct` asks a model to write
each task and its tests, and `accept` checks the result. These requirements trace to
the Go tests of this module, which replace the model by a scripted one and the
container by a simulated one and make no model call.

## Construction

- **SP-2540~2**: WHEN construction runs THE SYSTEM SHALL build the fixed task, then
  the domains in the order the protocol lists them, count a domain whose directory is
  complete and which the run configuration lists as passed without building it again,
  count a domain the construction records show as dropped as dropped without building
  it again, stop once five domains have passed, and end with an error naming how many
  passed when the domains run out first.
- **SP-2541~1**: WHEN the model is called THE SYSTEM SHALL run `codex exec` for
  gpt-5.6-terra at high reasoning effort, in a read-only sandbox, without the user's
  configuration or a kept session or a git check, in an empty working directory,
  with an output schema file, an output file for the final message and the prompt as
  its last argument, with standard input closed; SHALL read the output of
  `codex --version` once per call; and SHALL return the files of the final message,
  or an error that still carries the call's output, error output, version, times and
  exit status.
- **SP-2542~1**: WHEN the author is called for a whole task THE SYSTEM SHALL give it
  the domain line, the shape rules with both required properties, the prompt
  sentence to fill and the files of the boxoffice fixture as an example of shape; and
  SHALL word every prompt, schema and finding it sends with no reference to the
  study, a gate, mutants, test quality or what is measured.
- **SP-2543~1**: WHEN the author answers THE SYSTEM SHALL accept only `task.json`
  and files under `fixture/`, `reference/`, `violations/` and `refactors/` by clean
  relative paths, none written twice; SHALL set the name, fixture and truth paths of
  the task description itself; SHALL replace the author's previous files with the
  new set; SHALL check that the prompt follows the template with the task's
  criterion, spec and source; and SHALL report any other answer to the author as
  a failure of its output.
- **SP-2544~1**: WHEN the test writer is called, for a first answer, a repair or a
  rewrite after the spec changed, THE SYSTEM SHALL give it the spec and the
  interface, that is the module path, the operation and its signature, and nothing
  else: never the source, the tests, the reference, the violations or the refactors.
- **SP-2545~1**: WHEN the test writer answers THE SYSTEM SHALL accept only a
  `pytest.ini` and files named `test_` followed by the criterion's ID in lower case
  with its hyphen written as an underscore, one per criterion of the spec, in one
  flat directory, and write them as the task's oracle.
- **SP-2546~1**: WHEN the fixed task is built THE SYSTEM SHALL take its prompt and
  operations from the run configuration, ask the author for violations and
  refactors alone, from the spec and the reference source, never write its fixture
  or its reference, and stop with an error naming the check when a check can only
  be satisfied by changing either.

## Repair and records

- **SP-2547~2**: WHEN an acceptance check fails THE SYSTEM SHALL send each finding to
  the role it concerns; SHALL tell the test writer test names and criteria without
  the code or the messages of the project; SHALL make at most two repair calls per
  role and domain, a call that produced no usable files included and a call that
  failed on the harness side (SP-2650) excluded; and SHALL write the tests again from
  the new spec, without using a repair call, when a repair of the author changed the
  spec.
- **SP-2548~1**: WHEN a domain still fails after its repairs THE SYSTEM SHALL drop it,
  log the failing check with its reason, remove its files and repair nothing by
  hand; and WHEN the fixed task is dropped THE SYSTEM SHALL stop with an error
  naming the failing check.
- **SP-2549~2**: WHEN construction or acceptance runs THE SYSTEM SHALL append to the
  construction records every call with its role, purpose, domain, attempt, prompt
  digest, schema, raw output, Codex version, start and end times in UTC and exit
  status, the output of every check of every round, and every dropped domain with
  its reason, marking a call that failed on the harness side with its kind of
  failure; and SHALL read the records back whole, refusing a line that holds no
  record.

## Acceptance

- **SP-2550~1**: WHEN a check executes a command THE SYSTEM SHALL run it three times
  in the offline box with only the work directory mounted, and fail the check when
  the three runs differ in exit status or in the outcome of any test.
- **SP-2551~1**: WHEN check A1 runs THE SYSTEM SHALL require the fixture shape of the
  protocol: a task description and a prompt that follow their templates, a package
  of one module with one public operation callable by keyword, `pytest.ini` and
  `.gitignore` with their entries, one spec file that parses with Shallnot, holds 4
  to 6 criteria, each one WHEN … THE SYSTEM SHALL … statement with inline examples that
  all call the task's operation by keyword, tests that cite every criterion but the
  task criterion, at least two violations per criterion and one refactor, each an
  edit of an existing source file, and an operations binding that imports as a
  callable in the box; and WHEN A1 fails THE SYSTEM SHALL not run the other checks.
- **SP-2552~1**: WHEN check A2 runs THE SYSTEM SHALL require that the fixture's tests
  pass and that the traceability gate of the control arm, run on the equipped
  fixture, blocks on the task criterion's missing test and on nothing else.
- **SP-2553~1**: WHEN checks A3 and A4 run THE SYSTEM SHALL require that the reference
  tree passes the fixture's tests and every oracle test, and that the fixture fails
  an oracle test of the task criterion and passes every oracle test of the other
  criteria.
- **SP-2554~1**: WHEN checks A5 and A6 run THE SYSTEM SHALL require that each
  violation fails an oracle test of its own criterion and that every oracle test of
  the other criteria passes on it, and that each refactor passes every oracle test.
- **SP-2555~1**: WHEN check A7 runs THE SYSTEM SHALL require a violation of the task
  criterion that passes a test made of the task criterion's inline examples alone,
  and a violation of an implemented criterion that passes the fixture's tests.
- **SP-2556~2**: WHEN check A8 runs THE SYSTEM SHALL require that no fixture file
  holds a significant line of an oracle file, a line a violation or refactor adds to
  the reference, one of the truth paths, or one of the words oracle, mutant,
  planted, weak and deliberate; a line is significant from twenty characters, and
  an import, a line that restates an inline example, or an oracle line that a test
  file of the fixture holds too is not.
- **SP-2557~1**: WHEN check A9 runs THE SYSTEM SHALL require that `pytest
  --collect-only` collects at least two tests for each criterion and that the
  oracle skips none on the reference tree.
- **SP-2558~1**: WHEN every check passes THE SYSTEM SHALL append the task to the
  `tasks` of the run configuration, in the layout of its other entries and with
  every other byte unchanged, unless a task of that name is listed; and WHEN a
  check fails THE SYSTEM SHALL leave the configuration as it is.

## Verbs

- **SP-2559~2**: WHEN `accept` or `construct` is run THE SYSTEM SHALL read `--repo`,
  `--config` and `--work`, print the report of each check with its findings, exit 0
  when everything passed, exit 1 when a check failed, a step could not run or no
  container runner is wired in, exit 2 for a usage error, and, for `construct`, exit 3
  when a call failed on the harness side.

## Harness failures

- **SP-2650~1**: WHEN a call to the model ends THE SYSTEM SHALL class it as a harness
  failure when the process exceeded its time limit of thirty minutes, when `codex`
  could not be started or reported no version, when it exited non-zero and left no
  final message or an empty one, or when it produced no usable final message and its
  standard error or output shows a sign-in error (not logged in, 401, unauthorized),
  a usage or rate limit (usage limit, rate limit, 429, try again) or a network
  failure; SHALL class a final message that is not the expected JSON document, and
  any other failed call, as a failure of the model; and SHALL never class a call that
  exited cleanly with a final message as a harness failure, whatever its output text
  says.
- **SP-2651~1**: WHEN a call fails on the harness side THE SYSTEM SHALL stop
  construction at once with an error that names the domain, the role and the attempt,
  use no repair call, drop no domain, append the call to the construction records
  with its kind of harness failure, and make `construct` exit with its own status and
  a message that says the run can be resumed.
- **SP-2652~1**: WHEN construction runs again after it stopped THE SYSTEM SHALL read
  the construction records and resume at the call where it stopped, with the same
  domain, role, purpose and attempt number: the model answers no call that the records
  already hold, the checks of a failed round the records hold are not run again, the
  calls that failed on the harness side use no attempt number, and records that do
  not match the calls to repeat refuse the resume.
