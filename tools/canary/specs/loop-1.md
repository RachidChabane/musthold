# loop-1 study tooling: shared records

The shapes the verbs of the loop-1 study (`experiments/loop-1/PREREG.md`) exchange.
Each verb's own requirements live in a spec file of its own beside this one.

## Task descriptions

- **SP-2500~1**: WHEN a task description is read THE SYSTEM SHALL take from it the
  task's name, fixture, spec, source file, task criterion, prompt, operation
  bindings, the interface the oracle writer is told (module, operation and
  signature) and the paths of its reference, violations, refactors and oracle;
  and SHALL refuse a document with an unknown member or more than one document, a
  path that is absolute or leaves its root, a criterion that is not an ID, an
  empty prompt or operation binding, and a signature that does not start with the
  operation, naming every member at fault.

## Outcomes

- **SP-2501~1**: WHEN an attempt's outcome is judged THE SYSTEM SHALL count the
  completed and capped outcomes, rerun the quota stops and harness errors, and
  turn a run whose last allowed attempt is void into quota-blocked or
  harness-blocked after the cause of that attempt.

## Deviations

- **SP-2502~1**: WHEN a deviation is written THE SYSTEM SHALL write it as one line
  of time, kind, scope, section, what the protocol says and what happened, refuse
  a kind outside the closed set, a missing field, and a field that holds the
  field separator or a line break; and WHEN the log is read THE SYSTEM SHALL skip
  its title and blank lines, read back every entry it wrote, and read any other
  line as a protocol entry with scope all.

## Records

- **SP-2503~1**: WHEN an oracle test or a kill matrix is judged THE SYSTEM SHALL
  call a test defective when it fails in at least two of its three runs and flaky
  when it fails in exactly one, and give a test directory's kill share as its
  killed violations over all the task's violations.

## Inputs freeze

- **SP-2504~1**: WHEN the inputs freeze is read THE SYSTEM SHALL take from it the
  commits, binaries, pins, images, toolchains, egress hosts, quota patterns,
  policy, tasks with their base commit, equipped trees and instruction digests per
  arm, the construction records and the run order; SHALL refuse an unknown member
  or a second document; and SHALL find a task's entry by its name.
