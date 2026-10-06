Below are the specification of a small Python library, the original source of the functions that were changed, the changed functions, and a unified diff.

Decide, for every requirement of the specification, whether the changed library violates it:
- VIOLATED: some input covered by the requirement now gives a result the requirement forbids, taking the specification's precedence order into account;
- NOT_VIOLATED: no such input exists;
- UNDETERMINED: you cannot tell.

Give a confidence between 0 and 1 for each verdict.

For every VIOLATED verdict, give a witness: one concrete call on which the original library satisfies the requirement and the changed library does not. Fill `function` and the parameters that function takes; set every other parameter to null.
- `quote` takes `entry`, `exit` (ISO 8601 local times, "YYYY-MM-DDTHH:MM"), `lost_ticket`, `validated` (booleans);
- `format_receipt` takes `amount` (a decimal string such as "12.50").
`expected` is what the original library produces on that call: kind RETURNS with the returned value as text (a decimal amount such as "12.50" for `quote`, the exact string for `format_receipt`), or kind RAISES with the exception class name. For a verdict other than VIOLATED, `witness` is null.

Also decide whether the change is behaviourally equivalent to the original: true if no input of the public functions gives a different result or exception type.

{differential}

=== Specification ===
{spec}

=== Original functions ===
{original}

=== Changed functions ===
{mutant}

=== Diff ===
{diff}
