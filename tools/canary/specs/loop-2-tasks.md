# loop-2 study tooling: task construction and acceptance

The `draw-domains`, `construct` and `accept` verbs of the loop-2 study
(`docs/adr/0017-the-loop-2-decision.md`, "Tasks and oracle"). `draw-domains` deals
the business-rule domains to the study's phases; `construct` asks models to write
each task and its truth files; `accept` checks them without any model call. These
requirements trace to the Go tests of this module, which replace every model by a
scripted one and the container by a simulated one, and make no model call.

The rules that loop-2 keeps from loop-1 are loop-1's code and requirements: the
three agreeing runs of an executed check in the offline box (SP-2550), the oracle's
test files and its command (SP-2545, SP-2560), the leak rules (SP-2556), the Codex
invocation and its harness failures (SP-2541, SP-2650), the construction records
and their resume (SP-2549, SP-2651, SP-2652), and the admission of a task into the
run configuration (SP-2558).

## A task

A task lies in `experiments/loop-2/tasks/<name>/`:

- `task.json`, the task description;
- `fixture/`, the project the session starts from;
- `reference/`, the files of the fixture's package with both task criteria
  implemented, each at its path relative to the package directory;
- `violations/<criterion>/<k>/` and `refactors/<k>/`, edits of the reference in the
  same layout;
- `oracle/` and `oracle-2/`, one test file `test_<criterion>.py` per criterion and a
  `pytest.ini` each;
- `levers.json`, `citations.json` and `provenance.json`;
- `heldout.json`, written by `accept`.

```json
{
  "name": "parcel-rates",
  "tier": "max",
  "fixture": "experiments/loop-2/tasks/parcel-rates/fixture",
  "spec": "specs/rates.md",
  "package": "parcels",
  "criteria": [{"id": "RATE-4", "kind": "revised"}, {"id": "RATE-12", "kind": "new"}],
  "prompt": "...",
  "operations": {"quote": {"python": "parcels.rates:quote"}},
  "interface": [{"module": "parcels.rates", "operation": "quote", "signature": "quote(*, weight, zone)"}],
  "truth": {"reference": "...", "violations": "...", "refactors": "...", "oracle": "...", "second_oracle": "..."}
}
```

```json
{
  "interacting": [{"criterion": "RATE-2", "task_criterion": "RATE-12"}],
  "boundaries": [{"criterion": "RATE-12", "value": "30", "inside": "30", "outside": "31", "violation": "RATE-12/3"}],
  "shared_helper": {"function": "parcels.common:_band"},
  "revision": {"criterion": "RATE-4"},
  "size": true
}
```

- **SP-2900~1**: WHEN a loop-2 task description is read THE SYSTEM SHALL refuse
  unknown members and a second document, and name each member at fault: a name of
  lowercase letters, digits and hyphens; a tier of `hard` or `max`; clean relative
  fixture, spec, package and truth paths; exactly two task criteria, of the kinds
  `new` and `new` or `revised` and `new`, with distinct IDs; a prompt; and 3 to 6
  interface entries, each a dotted module, an operation name and a signature that
  starts with the operation, each operation bound alone in `operations` as
  `<module>:<operation>`.
- **SP-2901~1**: WHEN a loop-2 prompt is checked THE SYSTEM SHALL require, for two
  new criteria, "Implement requirements <ID1> and <ID2> of <spec path>, <statement
  in a few words>, in <package>/, and add tests that verify them. Tell me when the
  task is done." and, for a revised and a new criterion, "Implement the revised
  requirement <revised ID> and the new requirement <new ID> of <spec path>,
  <statement in a few words>, in <package>/, and update and add the tests that
  verify them. Tell me when the task is done.", with a statement of at most 12
  words on one line.

## Acceptance

`accept` runs the checks in this order: A1 to A12, then the lever checks L1 to L5.
Every check that executes a command runs it three times in the offline box, by
loop-1's rule.

- **SP-2902~1**: WHEN check A1 runs THE SYSTEM SHALL require a task description
  that parses and a prompt that follows its template; the fixture files of
  loop-1's shape (`pytest.ini`, `.gitignore`, one spec under `specs/`, tests under
  `tests/`); a spec that parses with Shallnot and holds 9 to 14 criteria, each one
  WHEN … THE SYSTEM SHALL … statement with inline examples that call an operation of
  the interface by keyword with arguments it takes; both task criteria in the
  spec, a revised one at revision 2 or more; each operation importable in the box
  as a callable whose parameters all pass by keyword and match its signature; the
  fixture's size of the tier; at least two violations per criterion, each under a
  criterion of the spec, and one refactor, each editing files the package holds;
  and a valid `levers.json`; and WHEN A1 fails THE SYSTEM SHALL not run the other
  checks.
- **SP-2903~1**: WHEN the fixture's size is measured THE SYSTEM SHALL count the
  non-blank lines of the fixture's Python files and of its spec, and require 250 to
  600 of them, and at least 400 for the tier `max`.
- **SP-2904~1**: WHEN check A1 reads the tests' citations THE SYSTEM SHALL require
  that the fixture's tests cite every implemented criterion at its revision, that
  no test cites a new task criterion, and that a revised task criterion is cited,
  by at least one test, only at an earlier revision.
- **SP-2905~1**: WHEN check A1 looks for a shared helper THE SYSTEM SHALL run the
  inline examples of every implemented criterion in the box under a trace of the
  package's function calls, and require a function that is not an operation and
  that the examples of at least two implemented criteria reach.
- **SP-2906~1**: WHEN check A2 runs THE SYSTEM SHALL require that the fixture's
  tests pass and that Shallnot's traceability check alone, `shallnot check` on the
  fixture's spec, tests and results and never a gate, reports error findings on
  nothing but the task criteria: `uncovered_requirement` on a new one,
  `uncovered_requirement` or `revision_mismatch` on the revised one, and at least
  one on each.
- **SP-2907~1**: WHEN check A3 runs THE SYSTEM SHALL require that the reference
  tree passes every oracle test and every fixture test that cites no revised task
  criterion.
- **SP-2908~1**: WHEN check A4 runs THE SYSTEM SHALL require that the fixture fails
  an oracle test of each task criterion and passes every oracle test of the
  implemented criteria.
- **SP-2909~1**: WHEN check A5 runs THE SYSTEM SHALL require that each violation
  fails an oracle test of its own criterion and that every oracle test of the other
  criteria passes on it, except the oracle tests of a task criterion that
  `levers.json` pairs with the violation's criterion as interacting.
- **SP-2910~1**: WHEN check A6 runs THE SYSTEM SHALL require that each refactor
  passes every oracle test.
- **SP-2911~1**: WHEN check A7 runs THE SYSTEM SHALL require, for each task
  criterion, a violation of it that passes a test made of its inline examples
  alone, and a violation of an implemented criterion that passes the fixture's
  tests by the rule of SP-2925.
- **SP-2912~1**: WHEN check A8 runs THE SYSTEM SHALL apply loop-1's leak rules with
  both oracles among the truth, each oracle line reported to the role that wrote
  it, and the reference, violations, refactors, both oracles, `levers.json`,
  `citations.json` and `provenance.json` among the truth paths.
- **SP-2913~1**: WHEN check A9 runs THE SYSTEM SHALL apply loop-1's collection rule
  to the oracle, and require that `provenance.json` records the oracle as written
  from the spec of the fixture.
- **SP-2914~1**: WHEN check A10 runs THE SYSTEM SHALL require that the second
  oracle holds one test file per criterion, passes every one of its tests on the
  reference tree, and was written, as `provenance.json` records, from the spec of
  the fixture.
- **SP-2915~1**: WHEN the assertions of an oracle are listed THE SYSTEM SHALL list,
  in each test file in name order, every line whose text starts with an `assert`
  statement or a `pytest.raises` call, each named `<file>:<line>`.
- **SP-2916~1**: WHEN check A11 runs THE SYSTEM SHALL require that `citations.json`
  was written from the spec and the oracle the task holds, and that it cites for
  every assertion of the oracle a sentence that the spec holds once spaces are
  collapsed, reporting each uncited assertion to the oracle's writer by its name.
- **SP-2917~1**: WHEN check A12 runs THE SYSTEM SHALL require, for each boundary
  `levers.json` declares, its violation among the violations of the boundary's
  criterion, failing an oracle test of that criterion and passing a test made of
  that criterion's inline examples alone.

## Levers

- **SP-2918~1**: WHEN `levers.json` is read THE SYSTEM SHALL refuse unknown members,
  count as declared each of L1 to L5 whose member is present (`interacting`,
  `boundaries`, `shared_helper`, `revision`, `size` true), and require at least 3
  declared levers for the tier `hard` and all 5 for the tier `max`.
- **SP-2919~1**: WHEN lever L1 is checked THE SYSTEM SHALL require at least one
  pair, each of an implemented criterion and a task criterion, and, for every
  pair, a violation of the implemented criterion that fails an oracle test of the
  task criterion.
- **SP-2920~1**: WHEN lever L2 is checked THE SYSTEM SHALL require at least two
  boundaries, each with its value written as a number in the statement of its
  criterion, with neither the value nor its two sides used as an argument of any
  inline example of the spec, and with both sides written in the oracle's test file
  of the criterion.
- **SP-2921~1**: WHEN lever L3 is checked THE SYSTEM SHALL require that the named
  function is defined in the fixture's package, that its definition in the
  reference differs from the fixture's, and that the inline examples of at least
  two implemented criteria reach it.
- **SP-2922~1**: WHEN lever L4 is checked THE SYSTEM SHALL require that the named
  criterion is the revised task criterion and that at least one fixture test fails
  on the reference tree.
- **SP-2923~1**: WHEN lever L5 is checked THE SYSTEM SHALL require at least 250
  lines by the count of SP-2903, and at least 400 for the tier `max`.
- **SP-2924~1**: WHEN a lever is not declared THE SYSTEM SHALL report its check as
  not run and pass the task on the other checks, and WHEN the tier is `max` THE
  SYSTEM SHALL require every lever, so that a `max` task revises one criterion and
  adds one.

## Held-out violations and admission

- **SP-2925~1**: WHEN the held-out violations of a task are recorded THE SYSTEM
  SHALL list every violation as `<criterion>/<k>` with its task and criterion, and
  mark that the fixture's tests pass on it when every fixture test that passes on
  the reference tree passes on the reference tree with the violation applied.
- **SP-2926~1**: WHEN every check passes THE SYSTEM SHALL write `heldout.json` in
  the task's directory and append the task to the `tasks` of `loop-2.json` by
  loop-1's admission rule; and WHEN a check fails THE SYSTEM SHALL leave both as
  they are.
- **SP-2927~1**: WHEN `loop-2.json` is read THE SYSTEM SHALL refuse unknown members,
  read its tasks as loop-1's run configuration does, require a draw seed, and
  validate the members it shares with loop-1's configuration by loop-1's rules once
  it lists a task.

## Domains and their draw

- **SP-2928~1**: WHEN the domain list is read THE SYSTEM SHALL require 60 domains,
  each a task name and a line, with distinct names, none of them a domain of
  loop-1.
- **SP-2929~1**: WHEN the domains are drawn THE SYSTEM SHALL order the 60 domains by
  a permutation seeded with the draw seed of `loop-2.json`, the same seed always
  giving the same order, and cut the order into 2 shakedown domains of the tier
  `max`, 8 domains of C1 of the tier `hard`, 8 of C2 of the tier `max`, 24
  confirmatory domains whose tier the calibration selects, and 18 in reserve.
- **SP-2930~1**: WHEN a domain is dropped THE SYSTEM SHALL give its place to the
  first reserve domain of the order not yet given, in the order the construction
  records hold the drops, and fail with an error naming the phase when the reserve
  is exhausted.
- **SP-2931~1**: WHEN `draw-domains` runs THE SYSTEM SHALL write
  `experiments/loop-2/draw.json` with the seed and the order when it is absent,
  refuse a committed draw that the seed does not reproduce, and print each phase's
  domains with every drop the construction records hold, its check and its reason.

## Construction

- **SP-2932~1**: WHEN `construct` runs for a phase THE SYSTEM SHALL build the
  phase's domains in the order of the draw at the phase's tier, `--tier` naming the
  confirmatory tier, count a domain whose task is complete and listed in
  `loop-2.json` as passed, replace each dropped domain by SP-2930, and stop once 2,
  8, 8 or 24 domains of the phase have passed.
- **SP-2933~1**: WHEN the author is called THE SYSTEM SHALL give it the domain line
  and the shape rules of the tier, accept only `task.json`, `levers.json` and files
  under `fixture/` and `reference/` by clean relative paths, set the name, tier,
  fixture and truth paths of the description itself, and replace its previous
  files with the new set.
- **SP-2934~1**: WHEN the variants are called THE SYSTEM SHALL give the writer the
  spec, the reference package and the boundaries `levers.json` declares with the
  violation each names, and accept only files under `violations/` and `refactors/`.
- **SP-2935~1**: WHEN the oracle or the second oracle is called THE SYSTEM SHALL
  give it the spec and the interface and nothing else, never the other oracle, and
  accept only a `pytest.ini` and one test file per criterion of the spec.
- **SP-2936~1**: WHEN the citations are called THE SYSTEM SHALL ask
  `claude-opus-5-5`, without tools, for the spec sentence behind each listed
  assertion of the oracle, read the structured answer of the session, and write
  `citations.json` with the digests of the spec and the oracle it read.
- **SP-2937~1**: WHEN a prompt, a schema or a repair finding is sent to a model
  THE SYSTEM SHALL word it with no reference to the study, a gate, mutants, levers,
  test quality or what is measured, and render every set of files in path order.
- **SP-2938~1**: WHEN the spec changes THE SYSTEM SHALL write both oracles and the
  citations again, and WHEN the oracle changes the citations, without using a
  repair call, recording in `provenance.json` the digests each was written from.
- **SP-2939~1**: WHEN an acceptance check fails THE SYSTEM SHALL send each finding to
  the role it concerns, at most two repair calls per role and domain; give A10's
  findings to the author for one repair of the spec, then drop the domain when A10
  fails again; and give A11's findings to the oracle for one repair, then drop the
  domain when A11 fails again.
- **SP-2940~1**: WHEN a domain is dropped THE SYSTEM SHALL record the failing check
  and its reason, remove the task's files and repair nothing by hand.
- **SP-2941~1**: WHEN construction calls a model or runs a check THE SYSTEM SHALL
  append it to loop-1's construction records with its role (`author`, `variants`,
  `oracle`, `second-oracle`, `citer`), the version of the model's client, and its
  output, with every path of the work directory, the repository and the home
  directory written as its placeholder, and resume at the call where it stopped by
  loop-1's rule.
- **SP-2942~1**: WHEN construction or acceptance runs THE SYSTEM SHALL start no
  coding agent and run no gate: its only Claude session is the citation call, and
  its only Shallnot commands are `shallnot check`.

## The citation session

- **SP-2943~1**: WHEN the citation call runs THE SYSTEM SHALL run `claude -p` in a
  session container of the study's image under loop-1's live-session isolation: a
  scratch HOME on tmpfs and no host HOME, no SSH agent socket, no docker socket, a
  read-only root file system, the run's internal network whose only route out is
  the logging proxy, which forwards only to `api.anthropic.com:443`, and the token
  of the first credential source on the entry's standard input alone.
- **SP-2944~1**: WHEN the entry is asked to forward its standard input THE SYSTEM
  SHALL give its command everything after the token's line, and otherwise close the
  command's standard input; and WHEN the citation session starts THE SYSTEM SHALL
  send the prompt after the token's line, never on a command line.
- **SP-2945~1**: WHEN the citation call ends THE SYSTEM SHALL class it by loop-1's
  harness rules, read the session's `structured_output` as the answer only when the
  session exited cleanly without `is_error`, and remove its containers and network
  whatever happened.

## Verbs

- **SP-2946~1**: WHEN `draw-domains`, `construct` or `accept` is run THE SYSTEM
  SHALL read `--repo`, `--config` and `--work`, print each report or phase, exit 0
  when everything passed, 1 when a check failed or a step could not run, 2 for a
  usage error, and, for `construct`, 3 when a call failed on the harness side.
- **SP-2947~1**: WHEN the runner is called as `musthold-demo-loop loop-2 <verb>`
  THE SYSTEM SHALL run the loop-2 verb of that name, `draw-domains`, `construct`,
  `accept`, `analyze`, `calibrate` or `report-lint`, and leave every loop-1 verb at
  the top level as it is.
- **SP-2948~1**: WHEN a loop-2 front-door script runs THE SYSTEM SHALL run the
  loop-2 verb of its own name, every loop-2 verb has its script, and no flag a
  script passes is refused by its verb.
- **SP-2949~1**: WHEN a loop-2 check runs a command in a container THE SYSTEM SHALL
  run it through loop-2's docker client, whose engine mounts the loop-2 work
  directory and product, with the image `--image` names or else the product
  manifest's built image.
