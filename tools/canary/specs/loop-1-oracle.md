# loop-1 study tooling: the oracle verb

The `oracle` verb of the loop-1 study (`experiments/loop-1/PREREG.md`, §4).

## Oracle run

- **SP-2560~1**: WHEN the oracle judges an attempt THE SYSTEM SHALL build a
  temporary tree from the project the session left, without its `tests/`,
  `.musthold/`, `test-results/` and `.git` entries and without any `oracle`
  directory of its own, add the task's `oracle/` to it, and run the command of
  PREREG §4.1 in it three times through the box runner, each run under the
  300 s limit; and SHALL fail with an error, and write no verdict, when the box
  cannot run the command or kills the container for memory.

- **SP-2561~1**: WHEN an oracle test's result is read from a pytest run THE
  SYSTEM SHALL count the test as failed when its JUnit case has a failure or an
  error child, count a run as a collection or import failure, which fails every
  oracle test, when its exit code is neither 0 nor 1 or its JUnit report is
  missing, unreadable or without a case, and count a test the run did not list as
  failed.

- **SP-2562~1**: WHEN an oracle run reaches its limit THE SYSTEM SHALL count
  every oracle test as failing in that run, whatever report the run wrote, and
  record the run as timed out.

- **SP-2563~1**: WHEN the oracle tests of an attempt are judged THE SYSTEM SHALL
  map each test to the criterion named by its file, `test_<criterion>.py` with
  the criterion in lower case and underscores for hyphens, call a test defective
  when it fails in at least two of the three runs, log a test failing in exactly
  one run as flaky and count it clean, mark the change defective when a test is
  defective, and list the criteria with a defective test as violated.

## Held-out violations

- **SP-2564~1**: WHEN a test directory is measured THE SYSTEM SHALL build the
  reference tree, the fixture with the task's reference applied, with the
  directory in place of `tests/`, the `pytest.ini` of the equipped commit and the
  directory's own `conftest.py` files, run the directory three times on it, take
  as valid the tests that pass in all three runs, report the others as
  contradicting the spec, and name each violation `<criterion>/<k>` after its
  directory under the task's violations.

- **SP-2565~1**: WHEN a test directory is run on a violation THE SYSTEM SHALL
  apply the violation's files over the reference tree, run the directory three
  times, and mark the violation killed when a valid test fails on it in at least
  two of the three runs and not killed when it fails in one.

- **SP-2566~1**: WHEN the held-out violations of an attempt are measured THE
  SYSTEM SHALL measure the tests at the equipped commit, the tests the session
  left and the tests at each stop snapshot, labelled `equipped`, `final` and
  `stop-<n>`, read the equipped commit and the stop trees with `git archive` run
  in the box against the run's repositories and the separate stop object store,
  treat a snapshot without a `tests/` path as holding no tests, and measure a
  test directory with the same content only once.

## Preconditions and records

- **SP-2567~1**: WHEN the oracle verb is started before the run phase has ended
  THE SYSTEM SHALL refuse to run, name the rule, run no command and write no
  record, and Main SHALL exit with 1 on a refusal and 2 on a usage error.

- **SP-2568~1**: WHEN the oracle verb runs THE SYSTEM SHALL write into a run
  directory nothing but `records/oracle.json` and `records/heldout.json`, write
  nothing into the task's truth directories or fixture, run every git command on
  the run's repositories in the box with the run directory mounted read-only, and
  remove its temporary trees.

- **SP-2569~1**: WHEN the oracle verb runs over a work directory THE SYSTEM SHALL
  judge every counted attempt of the order and no other, continue with the other
  attempts when one fails and report every failure, and reuse an output already
  written when the digest of its inputs is unchanged, so that the verb can
  resume.
