# loop-1 study tooling: the verbs as one runner

How the verbs of the loop-1 study (`experiments/loop-1/PREREG.md`) are wired
together, and the contracts between the packages that write a record and the
packages that read it.

## The run phase

- **SP-2640~1**: WHEN `oracle`, `detect` or `analyze` waits for the end of the run
  phase THE SYSTEM SHALL judge it with the rule of PREREG §6.6 over the order of the
  inputs freeze, refuse with the open runs while it has not ended, and hand `oracle`
  and `detect` the counted attempt of every run of the order, in that order.

## Acceptance

- **SP-2641~1**: WHEN `accept` equips the control arm for check A2 THE SYSTEM SHALL
  equip a project made from the fixture's files beside them, as a run of the control
  arm equips it, every step a command of the study's box with its run directory alone
  mounted, and run the traceability gate in that project.

## The canary

- **SP-2649~1**: WHEN the detectors of canary C3 run THE SYSTEM SHALL first measure
  the tests of C3's attempt at its equipped commit and at its end against its task's
  held-out violations, writing the attempt's `heldout.json`, and then run every
  detector of PREREG §9.8 on the attempt with the hosts C1 observed.

## The front door

- **SP-2642~1**: WHEN `experiments/loop-1/script/<verb>` runs a verb THE SYSTEM SHALL
  run the runner's subcommand of the same name, every such script naming a verb of
  the runner and every verb of the runner having its script, and every verb SHALL
  take `--repo`, `--config` and `--work`.

## Contracts between the packages

- **SP-2643~1**: WHEN construct names the oracle file of a criterion THE SYSTEM SHALL
  name it so that the oracle maps the file back to the same criterion.
- **SP-2644~1**: WHEN the reference or a violation of a task is applied to its
  fixture THE SYSTEM SHALL apply its files at the directory of the task's source
  file, in `accept` and in `oracle` alike.
- **SP-2645~1**: WHEN a run writes `hashes-before.json` and `hashes-after.json` THE
  SYSTEM SHALL write each as one flat object of the project's slash-separated paths to
  the hex SHA-256 of their contents, the shape `detect` reads.
- **SP-2646~1**: WHEN `analyze` reads the inputs freeze THE SYSTEM SHALL read it as
  `resolve` writes it, refuse a member the freeze does not define, and take the
  dropped domains from the construction log the freeze records.
- **SP-2647~1**: WHEN a session or a probe runs the Claude Code CLI THE SYSTEM SHALL
  run the image's `/usr/local/bin/claude`, never a copy under the product.
- **SP-2648~1**: WHEN `analyze` recognises a give-up of the end-of-turn hook THE
  SYSTEM SHALL recognise the exit code and the message prefix of the pinned
  Shallnot.
