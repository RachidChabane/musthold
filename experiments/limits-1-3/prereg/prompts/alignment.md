Below are the requirements of a small Python library, one of its tests, and the public signatures of the code the test calls.

Decide which single requirement the test's assertions verify: the requirement whose required observable behaviour the assertions check. Answer NONE if the assertions check behaviour that no listed requirement requires.

Coverage of that requirement:
- FULL: the assertions check the requirement's required outcome under its stated condition;
- PARTIAL: they check a correct but strict subset of its clauses;
- WEAK: an implementation violating the requirement would plausibly still pass (for example: assertions only on mocks or stubs, tautologies, type-only or no-exception checks, expected values that cannot be derived from the requirement);
- NONE: only when the answer is NONE.

Give your confidence from 0 to 100 and a rationale of at most 60 words.

=== Requirements ===
{criteria}

=== Test ===
{test}

=== Public signatures ===
{signatures}
