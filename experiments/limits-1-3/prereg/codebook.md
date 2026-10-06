# Codebook: test-to-criterion classes

Each test declares one criterion it verifies (its `verifies` link, written
`ID~REV`). A rater assigns each test one of six classes and lists the criteria
the test actually checks. Judge the test by what its assertions can detect when
the code under test changes, not by its name and not by the link it declares.

## Terms

- **Criterion**: one `ID~REV` requirement from the spec. It has a condition
  (the WHEN clause) and a required outcome (the SHALL clause). A precedence
  order in the spec decides which criterion governs an input that matches
  several conditions.
- **Declared criterion (A)**: the criterion named by the test's link.
- **Checks**: the criteria whose required outcome the test's assertions
  observe. A test checks criterion X when some implementation that violates X,
  and keeps every other criterion satisfied, makes the test fail.
- **Uncovered behaviour**: anything the spec leaves open. Typical cases are
  wording, representation and layout that no SHALL clause fixes, and
  behaviour on inputs that meet no criterion's condition.
- **Exact outcome**: an assertion that admits only the output A requires for
  the inputs used. An equality with A's required value is exact; so is a
  comparison with another output that A requires to be equal. An inequality,
  a range, a "not equal to", a substring or a case-insensitive match is not
  exact when A requires one specific output.

## Classes

### GOOD(A)

The test exercises inputs where A decides the outcome and asserts A's exact
outcome there. It does not depend on any other criterion's outcome, apart
from what A's condition itself requires.

- `checks: [A]`
- Typical shape: exact expected values at and around A's own boundaries.
  Where A's boundary meets another criterion's, the test uses a comparison
  that holds whatever the neighbouring criterion prescribes. For example, a
  value at the boundary is compared with a value well inside A, instead of
  with a number that the neighbouring criterion computes.
- `shape: n/a`

### GOOD_BROAD(A, B)

The test is GOOD for A and also depends on a second criterion B: an
implementation that violates only B can also make it fail. Typical causes are
asserting exact values that B computes at a boundary shared with A, or feeding
the output of B's behaviour into the behaviour A governs.

- `checks: [A, B]`, with A first.
- `shape: n/a`

### WEAK(A)

The test exercises inputs where A decides the outcome, but its assertion is
not exact: it admits outputs that A forbids, so some implementations that
violate A still pass. Shape:

- `weaker_property`: an inequality, a range, a "not equal to X", a
  case-insensitive or substring match, or a comparison with an output that A
  does not require to be equal.
- `checks: [A]`

### VACUOUS

No behaviour of the code under test can make the test fail. Shapes:

- `mock_assertion`: the function or helper that decides the outcome is
  replaced by a mock, and the assertion observes only the mock's configured
  return value or call.
- `tautology`: the assertion compares a value with itself, or with an
  expression that is equal by construction, such as a second identical call
  or a conversion that cannot change the value.
- `swallowed_exception`: the expected exception is caught and ignored, or
  asserted only inside the handler, so the test also passes when nothing is
  raised.
- `checks: NONE`

### MIS_OTHER(A→B)

The test declares A but checks a different criterion B. It passes on every
implementation that violates only A, and fails on some implementation that
violates B. Shape:

- `neighbour_criterion`, in one of two forms:
  - the inputs fall outside A's condition, or a precedence rule hands them to
    an earlier criterion B, for example a flag set on the call that lets B
    decide;
  - A's condition holds but A's outcome is not binding at the inputs used,
    so the asserted value is the one B produces and no violation of A alone
    can change it.

  The name often still describes A.
- `checks: [B]`

### OFFSPEC(A)

The test declares A but its assertions pin behaviour that no criterion
specifies. An implementation can satisfy every criterion and still fail it.
It passes on every implementation that violates only A, unless that
violation also changes the pinned uncovered behaviour. Shapes:

- `frozen_actual`: the expected value is a snapshot of whatever the code
  returned, and it includes parts no criterion specifies.
- `uncovered_behaviour`: a targeted assertion on one uncovered behaviour, as
  defined under Terms.
- `checks: UNCOVERED`

## Decision procedure

Answer in order and stop at the first class that fits.

1. With any mocks the test installs left in place, can a change to the real
   logic of the code under test make the test fail? If not: **VACUOUS**.
2. Does every assertion that can fail concern uncovered behaviour, so that an
   implementation satisfying every criterion could still fail the test? If so:
   **OFFSPEC**.
3. Look at what the assertions observe, once the precedence order is
   applied. If no implementation that violates only A can make the test
   fail, but one that violates only another criterion B can:
   **MIS_OTHER(A→B)**.
4. Does the assertion admit an output that A forbids at the inputs used, so
   that a plausible violation of A passes? If so: **WEAK(A)**.
5. Does the test also fail under a plausible violation of another criterion B
   only? If so: **GOOD_BROAD(A, B)**. Otherwise: **GOOD(A)**.

A plausible violation is a realistic developer mistake that breaks one
criterion and keeps the others: a shifted boundary, a wrong constant, a
missing or misordered branch.

## Alignment

`alignment_truth` is `ALIGNED` for GOOD and GOOD_BROAD and `NOT_ALIGNED` for
the other four classes.

## Record per test

```yaml
- nodeid: "tests/<file>.py::<test name>"
  cites: "<declared ID~REV>"
  label: GOOD | GOOD_BROAD | WEAK | VACUOUS | MIS_OTHER | OFFSPEC
  shape: n/a | weaker_property | mock_assertion | tautology | swallowed_exception | neighbour_criterion | frozen_actual | uncovered_behaviour
  checks: ["<ID~REV>", ...] | UNCOVERED | NONE
  alignment_truth: ALIGNED | NOT_ALIGNED
```
